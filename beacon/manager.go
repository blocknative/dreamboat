//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/beacon Datastore,ValidatorCache,BeaconClient,State
package beacon

import (
	"context"
	"errors"
	"fmt"
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
	SetHeadSlotIfHigher(structs.Slot) (structs.Slot, bool)

	Withdrawals(uint64, types.Hash) structs.WithdrawalsState
	SetWithdrawals(structs.WithdrawalsState)

	SetRandao(structs.RandaoState)
	Randao(uint64, types.Hash) structs.RandaoState

	Fork() structs.ForkState
	SetFork(structs.ForkState)

	ParentBlockHash() types.Hash
	SetParentBlockHash(types.Hash)
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

	ctx, cancel := context.WithCancel(ctx) // for stopping subscription after init is done
	defer cancel()

	c := make(chan bcli.PayloadAttributesEvent)
	client.SubscribeToPayloadAttributesEvents(c)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-c:
			headSlot := structs.Slot(event.Data.ProposalSlot - 1)
			state.SetHeadSlotIfHigher(headSlot)

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

			if err := s.updateWithdrawalsAndRandao(ctx, logger, state, event); err != nil {
				return err
			}
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

	c := make(chan bcli.PayloadAttributesEvent)
	client.SubscribeToPayloadAttributesEvents(c)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-c:
			t := time.Now()

			err := s.processNewSlot(ctx, state, client, ev, d, vCache)
			if err != nil {
				logger.
					WithField("slot", ev.Data.ProposalSlot-1).
					WithError(err).
					Error("error processing slot")
				continue
			}

			headSlot := state.HeadSlot()
			validators := state.KnownValidators()
			duties := state.Duties()
			parentHash := state.ParentBlockHash()

			logger.With(log.F{
				"epoch":                     headSlot.Epoch(),
				"slotHead":                  headSlot,
				"slotStartNextEpoch":        structs.Slot(headSlot.Epoch()+1) * structs.SlotsPerEpoch,
				"fork":                      state.Fork().Version(headSlot).String(),
				"numDuties":                 len(duties.ProposerDutiesResponse),
				"numKnownValidators":        len(validators.KnownValidators),
				"knownValidatorsUpdateTime": state.KnownValidatorsUpdateTime(),
				"randao":                    state.Randao(uint64(headSlot), parentHash).Randao,
				"randaoPrev":                state.Randao(uint64(headSlot)-1, parentHash).Randao,
				"withdrawalsRoot":           state.Withdrawals(uint64(headSlot), parentHash).Root.String(),
				"withdrawalsRootPrev":       state.Withdrawals(uint64(headSlot)-1, parentHash).Root.String(),
				"processingTimeMs":          time.Since(t).Milliseconds(),
			}).Debug("processed new slot")
		}
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

func (s *Manager) processNewSlot(ctx context.Context, state State, client BeaconClient, event bcli.PayloadAttributesEvent, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")

	headSlot := state.HeadSlot()
	received := structs.Slot(event.Data.ProposalSlot - 1)
	paHeadSlot, ok := state.SetHeadSlotIfHigher(received)
	if !ok {
		logger.WithField("slotHead", paHeadSlot).Debug("received old payload attributes")
		return nil
	}

	if headSlot > 0 {
		for slot := headSlot + 1; slot < received; slot++ {
			logger.With(log.F{"slot": slot, "event": "missed_slot"}).Warn("missed slot")
		}
	}

	headSlot = received
	logger = logger.WithField("slotHead", headSlot)

	if fork := state.Fork().Version(received); fork == structs.ForkAltair || fork == structs.ForkBellatrix {
		return fmt.Errorf("unknown fork: %d", fork)
	}

	// update proposer duties and known validators in the background
	if (structs.DurationPerEpoch / 2) < time.Since(state.KnownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func(slot structs.Slot) {
			if err := s.updateKnownValidators(ctx, state, client, slot); err != nil {
				logger.WithError(err).Error("failed to update known validators")
				return
			}
		}(received)
	}

	var latestParentBlockHash types.Hash
	if err := latestParentBlockHash.UnmarshalText([]byte(event.Data.ParentBlockHash)); err != nil {
		return fmt.Errorf("failed to unmarshal parentBlockHash: %w", err)
	}
	state.SetParentBlockHash(latestParentBlockHash)

	if err := s.updateWithdrawalsAndRandao(ctx, logger, state, event); err != nil {
		return err
	}

	// update proposer duties
	entries, err := s.getProposerDuties(ctx, client, structs.Slot(headSlot))
	if err != nil {
		return err
	}
	s.storeProposerDuties(ctx, state, d, vCache, structs.Slot(headSlot), entries)

	return nil
}

func (m *Manager) updateWithdrawalsAndRandao(ctx context.Context, logger log.Logger, state State, event bcli.PayloadAttributesEvent) error {
	slot := event.Data.ProposalSlot - 1

	hW := structs.HashWithdrawals{Withdrawals: event.Data.PayloadAttributes.Withdrawals}
	root, err := hW.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("failed to compute withdrawals root: %w", err)
	}
	randao := event.Data.PayloadAttributes.PrevRandao

	state.SetWithdrawals(structs.WithdrawalsState{Slot: structs.Slot(slot), Root: root})
	state.SetRandao(structs.RandaoState{Slot: uint64(slot), Randao: randao})

	return nil
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
