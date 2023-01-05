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
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}
type Service struct {
	Log             log.Logger
	Config          Config
	Datastore       Datastore
	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	// state
	state        *AtomicState
	headslotSlot structs.Slot
	updateTime   atomic.Value
}

func NewService(l log.Logger, c Config, d Datastore, as *AtomicState) *Service {
	return &Service{
		Log:       l.WithField("relay-service", "Service"),
		Config:    c,
		Datastore: d,
		state:     as,
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

func (s *Service) RunBeacon(ctx context.Context, client BeaconClient) error {
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

	err = s.updateProposerDuties(ctx, client, structs.Slot(syncStatus.HeadSlot))
	if err != nil {
		return err
	}

	defer logger.Debug("beacon loop stopped")

	events := make(chan HeadEvent)

	client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			err := s.processNewSlot(ctx, client, ev)
			if err != nil {
				logger.
					With(ev).
					WithError(err).
					Warn("error processing slot")
				continue
			}

			validators := s.state.Beacon().KnownValidators()
			duties := s.state.Beacon().ProposerDutiesResponse

			logger.With(log.F{
				"epoch":                     s.headslotSlot.Epoch(),
				"slotHead":                  s.headslotSlot,
				"slotStartNextEpoch":        structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
				"numDuties":                 len(duties),
				"numKnownValidators":        len(validators),
				"knownValidatorsUpdateTime": s.knownValidatorsUpdateTime(),
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
		go func() {
			if err := s.updateKnownValidators(ctx, client, s.headslotSlot); err != nil {
				logger.WithError(err).Warn("failed to update known validators")
			} else {
				s.updateTime.Store(time.Now())
				s.setReady()
			}
		}()
	}

	if err := s.updateProposerDuties(ctx, client, s.headslotSlot); err != nil {
		return err
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

func (s *Service) updateProposerDuties(ctx context.Context, client BeaconClient, headSlot structs.Slot) error {
	epoch := headSlot.Epoch()

	logger := s.Log.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	state := structs.DutiesState{}

	// Query current epoch
	current, err := client.GetProposerDuties(epoch)
	if err != nil {
		return fmt.Errorf("current epoch: get proposer duties: %w", err)
	}

	entries := current.Data

	// Query next epoch
	next, err := client.GetProposerDuties(epoch + 1)
	if err != nil {
		return fmt.Errorf("next epoch: get proposer duties: %w", err)
	}
	entries = append(entries, next.Data...)

	state.ProposerDutiesResponse = make(structs.BuilderGetValidatorsResponseEntrySlice, 0, len(entries))
	state.CurrentSlot = headSlot

	for _, e := range entries {
		reg, err := s.Datastore.GetRegistration(ctx, e.PubKey)
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

	return nil
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
