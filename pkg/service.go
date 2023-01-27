//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg Datastore,BeaconClient
package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/lthibault/log"
)

const (
	Version = "0.3.6"
)

var (
	ErrBeaconNodeSyncing = errors.New("beacon node is syncing")
)

type Datastore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type Service struct {
	Log             log.Logger
	Config          Config
	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	// state
	state        *AtomicState
	headslotSlot structs.Slot
	updateTime   atomic.Value
}

func NewService(l log.Logger, c Config, as *AtomicState) *Service {
	return &Service{
		Log:    l.WithField("relay-service", "Service"),
		Config: c,
		state:  as,
	}
}

func (s *Service) Ready() <-chan struct{} {
	s.once.Do(func() {
		s.ready = make(chan struct{})
	})
	return s.ready
}

func (s *Service) setReady() {
	select {
	case <-s.Ready():
	default:
		close(s.ready)
	}
}

func (s *Service) RunBeacon(ctx context.Context, client BeaconClient, d Datastore) error {
	logger := s.Log.WithField("method", "RunBeacon")

	syncStatus, err := s.waitSynced(ctx, client)
	if err != nil {
		return err
	}

	genesis, err := client.Genesis()
	if err != nil {
		return fmt.Errorf("fail to get genesis from beacon: %w", err)
	}
	s.state.genesis.Store(genesis)
	logger.
		WithField("genesis-time", time.Unix(int64(genesis.GenesisTime), 0)).
		Info("genesis retrieved")

	entries, err := s.getProposerDuties(ctx, client, structs.Slot(syncStatus.HeadSlot))
	if err != nil {
		return err
	}
	s.storeProposerDuties(ctx, d, s.headslotSlot, entries)

	defer logger.Debug("beacon loop stopped")

	events := make(chan HeadEvent)

	client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			t := time.Now()

			err := s.processNewSlot(ctx, client, ev)
			if err != nil {
				logger.
					With(ev).
					WithError(err).
					Warn("error processing slot")
				continue
			}
			entries, err := s.getProposerDuties(ctx, client, s.headslotSlot)
			if err != nil {
				logger.
					With(ev).
					WithError(err).
					Warn("error processing slot (proposer duties)")
				continue
			}
			s.storeProposerDuties(ctx, d, s.headslotSlot, entries)

			validators := s.state.Beacon().KnownValidators()
			duties := s.state.Beacon().ProposerDutiesResponse

			logger.With(log.F{
				"epoch":                     s.headslotSlot.Epoch(),
				"slotHead":                  s.headslotSlot,
				"slotStartNextEpoch":        structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
				"numDuties":                 len(duties),
				"numKnownValidators":        len(validators),
				"knownValidatorsUpdateTime": s.knownValidatorsUpdateTime(),
				"processingTimeMs":          time.Since(t).Milliseconds(),
			}).Debug("processed new slot")
		}
	}
}

func (s *Service) waitSynced(ctx context.Context, client BeaconClient) (*SyncStatusPayloadData, error) {
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

func (s *Service) processNewSlot(ctx context.Context, client BeaconClient, event HeadEvent) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")

	received := structs.Slot(event.Slot)
	if received <= s.headslotSlot {
		return nil
	}

	if s.headslotSlot > 0 {
		for slot := s.headslotSlot + 1; slot < received; slot++ {
			s.Log.Warnf("missedSlot %d", slot)
		}
	}

	s.headslotSlot = received

	// update proposer duties and known validators in the background
	if (DurationPerEpoch / 2) < time.Since(s.knownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func(slot structs.Slot) {
			if err := s.updateKnownValidators(ctx, client, slot); err != nil {
				logger.WithError(err).Warn("failed to update known validators")
				return
			}

			s.updateTime.Store(time.Now())
			s.setReady()
		}(received)
	}

	return nil
}

func (s *Service) knownValidatorsUpdateTime() time.Time {
	updateTime, ok := s.updateTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return updateTime
}

func (s *Service) getProposerDuties(ctx context.Context, client BeaconClient, headSlot structs.Slot) (entries []RegisteredProposersResponseData, err error) {
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

func (s *Service) storeProposerDuties(ctx context.Context, d Datastore, headSlot structs.Slot, entries []RegisteredProposersResponseData) {
	epoch := headSlot.Epoch()
	logger := s.Log.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	state := structs.DutiesState{
		CurrentSlot:            headSlot,
		ProposerDutiesResponse: make(structs.BuilderGetValidatorsResponseEntrySlice, 0, len(entries)),
	}

	for _, e := range entries {
		reg, err := d.GetRegistration(ctx, e.PubKey.PublicKey)
		if err == nil {
			state.ProposerDutiesResponse = append(state.ProposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
				Slot:  e.Slot,
				Entry: &reg,
			})
		} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
			logger.Warn(err)
		}
	}
	s.state.duties.Store(state)
}

func (s *Service) updateKnownValidators(ctx context.Context, client BeaconClient, current structs.Slot) error {
	state := structs.ValidatorsState{}
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

	state.KnownValidators = knownValidators
	state.KnownValidatorsByIndex = knownValidatorsByIndex

	s.state.validators.Store(state)

	return nil
}
