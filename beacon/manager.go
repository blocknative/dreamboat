//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/beacon Datastore,ValidatorCache,BeaconClient,State
package beacon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	bcli "github.com/blocknative/dreamboat/beacon/client"
	"github.com/blocknative/dreamboat/datastore"
	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
)

const (
	Version = "0.3.6"
)

var (
	ErrUnkownFork = errors.New("beacon node fork is unknown")
)

type Datastore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type ValidatorCache interface {
	Get(types.PublicKey) (structs.ValidatorCacheEntry, bool)
}

type BeaconClient interface {
	SubscribeToHeadEvents(ctx context.Context, slotC chan bcli.HeadEvent)
	GetProposerDuties(structs.Epoch) (*bcli.RegisteredProposersResponse, error)
	SyncStatus() (*bcli.SyncStatusPayloadData, error)
	KnownValidators(structs.Slot) (bcli.AllValidatorsResponse, error)
	Genesis() (structs.GenesisInfo, error)
	GetWithdrawals(structs.Slot) (*bcli.GetWithdrawalsResponse, error)
	Randao(structs.Slot) (string, error)
	GetForkSchedule() (*bcli.GetForkScheduleResponse, error)
	SubscribeToPayloadAttributesEvents(payloadAttrC chan bcli.PayloadAttributesEvent)
}

type State interface {
	SetGenesis(structs.GenesisInfo)

	Duties() structs.DutiesState
	SetDuties(structs.DutiesState)

	KnownValidators() structs.ValidatorsState
	KnownValidatorsUpdateTime() time.Time
	SetKnownValidators(structs.ValidatorsState)

	HeadSlot() structs.Slot
	SetHeadSlot(structs.Slot)

	HeadSlotPayloadAttributes() uint64
	SetHeadSlotPayloadAttributesIfHigher(uint64) (uint64, bool)

	Withdrawals(uint64) structs.WithdrawalsState
	SetWithdrawals(structs.WithdrawalsState)

	SetRandao(structs.RandaoState)
	Randao(uint64) structs.RandaoState

	Fork() structs.ForkState
	SetFork(structs.ForkState)

	ParentBlockHash() string
	SetParentBlockHash(string)
}

type Config struct {
	AltairForkVersion    string
	BellatrixForkVersion string
	CapellaForkVersion   string

	RunPayloadAttributesSubscription bool
}

type Manager struct {
	Log    log.Logger
	Config Config

	mu sync.Mutex
}

func NewManager(l log.Logger, cfg Config) *Manager {
	return &Manager{
		Log: l.With(log.F{
			"subService":                       "beacon-manager",
			"runPayloadAttributesSubscription": cfg.RunPayloadAttributesSubscription,
			"numberOfSlotsInState":             structs.NumberOfSlotsInState,
		}),
		Config: cfg,
	}
}

func (s *Manager) Init(ctx context.Context, state State, client BeaconClient, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "Init")

	syncStatus, err := s.waitSynced(ctx, client)
	if err != nil {
		return err
	}

	genesis, err := client.Genesis()
	if err != nil {
		return fmt.Errorf("fail to get genesis from beacon: %w", err)
	}
	state.SetGenesis(genesis)
	logger.
		WithField("genesis-time", time.Unix(int64(genesis.GenesisTime), 0)).
		Info("genesis retrieved")

	if err := s.initForkEpoch(ctx, state, client); err != nil {
		return fmt.Errorf("failed to set fork state: %w", err)
	}

	fork := state.Fork()
	headSlot := structs.Slot(syncStatus.HeadSlot)
	if !fork.IsAltair(headSlot) && !fork.IsBellatrix(headSlot) && !fork.IsCapella(headSlot) {
		return ErrUnkownFork
	}

	events := make(chan bcli.HeadEvent, 1)

	ctx, cancel := context.WithCancel(ctx) // for stopping subscription after init is done
	defer cancel()

	client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-events:
			headSlot := structs.Slot(event.Slot)
			state.SetHeadSlot(headSlot)

			// update proposer duties and known validators
			if err := s.updateKnownValidators(ctx, state, client, headSlot); err != nil {
				logger.WithError(err).Error("failed to update known validators")
				continue
			}

			entries, err := s.getProposerDuties(ctx, client, headSlot)
			if err != nil {
				return err
			}
			s.storeProposerDuties(ctx, state, d, vCache, headSlot, entries)

			randao, err := client.Randao(headSlot)
			if err != nil {
				return fmt.Errorf("fail to update randao: %w", err)
			}
			state.SetRandao(structs.RandaoState{Slot: uint64(headSlot), Randao: randao})

			return nil
		}
	}
}

func (m *Manager) initForkEpoch(ctx context.Context, state State, client BeaconClient) error {
	forkSchedule, err := client.GetForkSchedule()
	if err != nil {
		return fmt.Errorf("failed to get fork: %w", err)
	}

	forkState := structs.ForkState{}
	for _, fork := range forkSchedule.Data {
		switch fork.CurrentVersion {
		case m.Config.AltairForkVersion:
			forkState.AltairEpoch = structs.Epoch(fork.Epoch)
		case m.Config.BellatrixForkVersion:
			forkState.BellatrixEpoch = structs.Epoch(fork.Epoch)
		case m.Config.CapellaForkVersion:
			forkState.CapellaEpoch = structs.Epoch(fork.Epoch)
		}
	}

	state.SetFork(forkState)
	return nil
}

func (s *Manager) Run(ctx context.Context, state State, client BeaconClient, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "RunBeacon")

	defer logger.Debug("beacon loop stopped")

	events := make(chan bcli.HeadEvent, structs.NumberOfSlotsInState)

	if s.Config.RunPayloadAttributesSubscription {
		go s.RunPayloadAttributesSubscription(ctx, state, client, events)
	}

	client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			t := time.Now()

			err := s.processNewSlot(ctx, state, client, ev, d, vCache)
			if err != nil {
				logger.
					With(ev).
					WithError(err).
					Error("error processing slot")
				continue
			}

			headSlot := state.HeadSlot()
			validators := state.KnownValidators()
			duties := state.Duties()

			if s.Config.RunPayloadAttributesSubscription {
				logger.With(log.F{
					"epoch":                     headSlot.Epoch(),
					"slotHead":                  headSlot,
					"slotStartNextEpoch":        structs.Slot(headSlot.Epoch()+1) * structs.SlotsPerEpoch,
					"numDuties":                 len(duties.ProposerDutiesResponse),
					"numKnownValidators":        len(validators.KnownValidators),
					"knownValidatorsUpdateTime": state.KnownValidatorsUpdateTime(),
					"processingTimeMs":          time.Since(t).Milliseconds(),
					"fork":                      state.Fork().Version(headSlot).String(),
				}).Debug("processed new slot")
			} else {
				logger.With(log.F{
					"epoch":                     headSlot.Epoch(),
					"slotHead":                  headSlot,
					"slotHeadPayloadAttributes": state.HeadSlotPayloadAttributes(),
					"slotStartNextEpoch":        structs.Slot(headSlot.Epoch()+1) * structs.SlotsPerEpoch,
					"fork":                      state.Fork().Version(headSlot).String(),
					"numDuties":                 len(duties.ProposerDutiesResponse),
					"numKnownValidators":        len(validators.KnownValidators),
					"knownValidatorsUpdateTime": state.KnownValidatorsUpdateTime(),
					"randao":                    state.Randao(uint64(headSlot)).Randao,
					"randaoPrev":                state.Randao(uint64(headSlot) - 1).Randao,
					"withdrawalsRoot":           state.Withdrawals(uint64(headSlot)).Root.String(),
					"withdrawalsRootPrev":       state.Withdrawals(uint64(headSlot) - 1).Root.String(),
					"processingTimeMs":          time.Since(t).Milliseconds(),
				}).Debug("processed new slot")
			}
		}
	}
}

func (s *Manager) RunPayloadAttributesSubscription(ctx context.Context, state State, client BeaconClient, events chan bcli.HeadEvent) {
	logger := s.Log.WithField("method", "RunPayloadAttributesSubscription")

	c := make(chan bcli.PayloadAttributesEvent)
	client.SubscribeToPayloadAttributesEvents(c)

	for payloadAttributes := range c {
		slot := payloadAttributes.Data.ProposalSlot - 1
		logger = logger.WithField("slotPayloadAttributes", slot)

		paHeadSlot, ok := state.SetHeadSlotPayloadAttributesIfHigher(slot)
		if !ok {
			logger.WithField("slotHeadPayloadAttributes", paHeadSlot).Debug("received old payload attributes")
			continue
		}

		headSlot := state.HeadSlot()

		if slot <= uint64(headSlot)-structs.NumberOfSlotsInState {
			continue
		}

		select {
		case events <- bcli.HeadEvent{Slot: slot}:
		default:
		}

		// discard repetitive payload attributes (we receive them once from each beacon node)
		latestParentBlockHash := state.ParentBlockHash()
		if latestParentBlockHash == payloadAttributes.Data.ParentBlockHash {
			continue
		}

		if fork := state.Fork().Version(structs.Slot(slot)); fork == structs.ForkAltair || fork == structs.ForkBellatrix {
			continue
		}

		hW := structs.HashWithdrawals{Withdrawals: payloadAttributes.Data.PayloadAttributes.Withdrawals}
		root, err := hW.HashTreeRoot()
		if err != nil {
			logger.WithError(err).Warn("failed to compute withdrawals root")
			continue
		}
		randao := payloadAttributes.Data.PayloadAttributes.PrevRandao

		state.SetWithdrawals(structs.WithdrawalsState{Slot: structs.Slot(slot), Root: root})
		state.SetRandao(structs.RandaoState{Slot: uint64(slot), Randao: randao})

		logger.With(log.F{
			"epoch":                     headSlot.Epoch(),
			"slot":                      slot,
			"slotHead":                  headSlot,
			"slotHeadPayloadAttributes": paHeadSlot,
			"slotStartNextEpoch":        structs.Slot(headSlot.Epoch()+1) * structs.SlotsPerEpoch,
			"fork":                      state.Fork().Version(headSlot).String(),
			"randao":                    randao,
			"randaoPrev":                state.Randao(slot - 1).Randao,
			"withdrawalsRoot":           types.Hash(root).String(),
			"withdrawalsRootPrev":       state.Withdrawals(slot - 1).Root.String(),
		}).Debug("processed payload attributes")
	}
}

func (s *Manager) waitSynced(ctx context.Context, client BeaconClient) (*bcli.SyncStatusPayloadData, error) {
	logger := s.Log.WithField("method", "WaitSynced")

	for {
		status, err := client.SyncStatus()
		if err != nil {
			if !errors.Is(err, bcli.ErrBeaconNodeSyncing) {
				return nil, err
			}
		} else if !status.IsSyncing {
			return status, nil
		}

		logger.Debug("beacon clients are syncing...")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func (s *Manager) processNewSlot(ctx context.Context, state State, client BeaconClient, event bcli.HeadEvent, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")

	received := structs.Slot(event.Slot)
	headSlot := state.HeadSlot()
	if received <= headSlot {
		return nil
	}

	if headSlot > 0 {
		for slot := headSlot + 1; slot < received; slot++ {
			logger.With(log.F{"slot": slot, "event": "missed_slot"}).Warn("missed slot")
		}
	}

	state.SetHeadSlot(received)
	headSlot = received
	logger = logger.WithField("slotHead", headSlot)

	// update proposer duties and known validators in the background
	if (structs.DurationPerEpoch / 2) < time.Since(state.KnownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func(slot structs.Slot) {
			if err := s.updateKnownValidators(ctx, state, client, slot); err != nil {
				logger.WithError(err).Error("failed to update known validators")
				return
			}
		}(received)
	}

	// payload_attributes event was not received
	if !s.Config.RunPayloadAttributesSubscription {
		// update randao
		randao, err := client.Randao(headSlot)
		if err != nil {
			return fmt.Errorf("fail to update randao: %w", err)
		}

		if curRandao := state.Randao(uint64(headSlot)); curRandao.Randao == "" {
			state.SetRandao(structs.RandaoState{Slot: uint64(headSlot), Randao: randao})
			// query expected withdrawals root
			go s.updateExpectedWithdrawals(headSlot, state, client)
		} else if curRandao.Randao != randao {
			logger.With(log.F{"current": curRandao.Randao, "received": randao}).Warn("blocked randao replace")
		}
	}

	// update proposer duties
	entries, err := s.getProposerDuties(ctx, client, headSlot)
	if err != nil {
		return err
	}
	s.storeProposerDuties(ctx, state, d, vCache, headSlot, entries)

	return nil
}

func (m *Manager) updateExpectedWithdrawals(slot structs.Slot, state State, client BeaconClient) {
	logger := m.Log.WithField("method", "UpdatedExpectedWithdrawals").WithField("slot", slot)
	current := state.Withdrawals(uint64(slot))
	latestKnownSlot := current.Slot
	if slot < latestKnownSlot || !state.Fork().IsCapella(slot) {
		return
	}

	// get withdrawals from BN
	withdrawals, err := client.GetWithdrawals(slot)
	if err != nil {
		if errors.Is(err, bcli.ErrWithdrawalsUnsupported) {
			logger.WithError(err).Debug("attempted to fetch withdrawals before capella")
		} else {
			logger.WithError(err).Error("failed to get withdrawals from beacon node")
		}
		return
	}
	hW := structs.HashWithdrawals{Withdrawals: withdrawals.Data.Withdrawals}
	root, err := hW.HashTreeRoot()
	if err != nil {
		logger.WithError(err).Warn("failed to compute withdrawals root")
		return
	}

	state.SetWithdrawals(structs.WithdrawalsState{Slot: slot, Root: root})
}

func (s *Manager) getProposerDuties(ctx context.Context, client BeaconClient, headSlot structs.Slot) (entries []bcli.RegisteredProposersResponseData, err error) {
	epoch := headSlot.Epoch()

	// Query current epoch
	current, err := client.GetProposerDuties(epoch)
	if err != nil {
		return nil, fmt.Errorf("current epoch: get proposer duties: %w", err)
	}

	entries = current.Data

	// Query next epoch
	next, err := client.GetProposerDuties(epoch + 1)
	if err != nil {
		return nil, fmt.Errorf("next epoch: get proposer duties: %w", err)
	}

	return append(entries, next.Data...), nil
}

func (s *Manager) storeProposerDuties(ctx context.Context, state State, d Datastore, vCache ValidatorCache, headSlot structs.Slot, entries []bcli.RegisteredProposersResponseData) {
	epoch := headSlot.Epoch()
	logger := s.Log.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	newState := structs.DutiesState{
		CurrentSlot:            headSlot,
		ProposerDutiesResponse: make(structs.BuilderGetValidatorsResponseEntrySlice, 0, len(entries)),
	}

	var err error
	for _, e := range entries {
		reg, ok := vCache.Get(e.PubKey.PublicKey)
		if !ok {
			reg.Entry, err = d.GetRegistration(ctx, e.PubKey.PublicKey)
			if err != nil {
				if !errors.Is(err, datastore.ErrNotFound) {
					logger.Warn(fmt.Errorf("fail retrieve validator %s: %w", e.PubKey.PublicKey.String(), err))
				}
				continue
			}
		}
		newState.ProposerDutiesResponse = append(newState.ProposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
			Slot:  e.Slot,
			Entry: &reg.Entry,
		})
	}
	state.SetDuties(newState)
}

func (s *Manager) updateKnownValidators(ctx context.Context, state State, client BeaconClient, current structs.Slot) error {
	newState := structs.ValidatorsState{}
	validators, err := client.KnownValidators(current)
	if err != nil {
		return err
	}

	knownValidators := make(map[types.PubkeyHex]struct{})
	knownValidatorsByIndex := make(map[uint64]types.PubkeyHex)
	for _, vs := range validators.Data {
		knownValidators[types.NewPubkeyHex(vs.Validator.Pubkey)] = struct{}{}
		knownValidatorsByIndex[vs.Index] = types.NewPubkeyHex(vs.Validator.Pubkey)
	}

	newState.KnownValidators = knownValidators
	newState.KnownValidatorsByIndex = knownValidatorsByIndex

	state.SetKnownValidators(newState)

	return nil
}
