//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg/beacon Datastore,ValidatorCache,BeaconClient,State
package beacon

import (
	"context"
	"errors"
	"fmt"
	"time"

	bcli "github.com/blocknative/dreamboat/pkg/beacon/client"
	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
)

const (
	Version = "0.3.6"
)

var (
	DurationPerSlot  = time.Second * 12
	DurationPerEpoch = DurationPerSlot * time.Duration(structs.SlotsPerEpoch)
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
	PublishBlock(block *types.SignedBeaconBlock) error
}

type State interface {
	SetReady()

	SetGenesis(structs.GenesisInfo)

	Duties() structs.DutiesState
	SetDuties(structs.DutiesState)

	KnownValidators() structs.ValidatorsState
	KnownValidatorsUpdateTime() time.Time
	SetKnownValidators(structs.ValidatorsState)

	HeadSlot() structs.Slot
	SetHeadSlot(structs.Slot)
}

type Manager struct {
	Log log.Logger
}

func NewManager(l log.Logger, as *AtomicState) *Manager {
	return &Manager{
		Log: l.WithField("relay-service", "Service"),
	}
}

func (s *Manager) Run(ctx context.Context, state State, client BeaconClient, d Datastore, vCache ValidatorCache) error {
	logger := s.Log.WithField("method", "RunBeacon")

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

	entries, err := s.getProposerDuties(ctx, client, structs.Slot(syncStatus.HeadSlot))
	if err != nil {
		return err
	}

	s.storeProposerDuties(ctx, state, d, vCache, structs.Slot(syncStatus.HeadSlot), entries)

	defer logger.Debug("beacon loop stopped")

	events := make(chan bcli.HeadEvent)

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
					Warn("error processing slot")
				continue
			}

			validators := state.KnownValidators()
			duties := state.Duties()
			headSlot := state.HeadSlot()

			logger.With(log.F{
				"epoch":                     headSlot.Epoch(),
				"slotHead":                  headSlot,
				"slotStartNextEpoch":        structs.Slot(headSlot.Epoch()+1) * structs.SlotsPerEpoch,
				"numDuties":                 len(duties.ProposerDutiesResponse),
				"numKnownValidators":        len(validators.KnownValidators),
				"knownValidatorsUpdateTime": state.KnownValidatorsUpdateTime(),
				"processingTimeMs":          time.Since(t).Milliseconds(),
			}).Debug("processed new slot")
		}
	}
}

func (s *Manager) waitSynced(ctx context.Context, client BeaconClient) (*bcli.SyncStatusPayloadData, error) {
	logger := s.Log.WithField("method", "WaitSynced")

	for {
		status, err := client.SyncStatus()
		if err != nil || !status.IsSyncing {
			return status, err
		}

		logger.Debug("beacon clients are syncing...")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
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
			s.Log.Warnf("missedSlot %d", slot)
		}
	}

	state.SetHeadSlot(received)
	headSlot = received

	// update proposer duties and known validators in the background
	if (DurationPerEpoch / 2) < time.Since(state.KnownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func(slot structs.Slot) {
			if err := s.updateKnownValidators(ctx, state, client, slot); err != nil {
				logger.WithError(err).Warn("failed to update known validators")
				return
			}
			state.SetReady()
		}(received)
	}

	entries, err := s.getProposerDuties(ctx, client, headSlot)
	if err != nil {
		return err
	}
	s.storeProposerDuties(ctx, state, d, vCache, headSlot, entries)

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