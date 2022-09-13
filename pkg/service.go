//go:generate mockgen -source=service.go -destination=../internal/mock/pkg/service.go -package=mock_relay
package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
)

const (
	Version = "0.1.0"
)

type RelayService interface {
	// Proposer APIs (builder spec https://github.com/ethereum/builder-specs)
	RegisterValidator(context.Context, []types.SignedValidatorRegistration) error
	GetHeader(context.Context, HeaderRequest) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error)

	// Builder APIs (relay spec https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5)
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest) error
	GetValidators() BuilderGetValidatorsResponseEntrySlice

	// Data APIs
	GetDelivered(context.Context, Slot) ([]types.BidTrace, error)
	GetDeliveredByHash(context.Context, types.Hash) ([]types.BidTrace, error)
	GetDeliveredByNum(context.Context, uint64) ([]types.BidTrace, error)
	GetDeliveredByPubKey(context.Context, types.PublicKey) ([]types.BidTrace, error)
	GetTailDelivered(context.Context, uint64) ([]types.BidTrace, error)
	GetTailDeliveredCursor(context.Context, uint64, uint64) ([]types.BidTrace, error)

	GetBlockReceived(context.Context, Slot) ([]BidTraceWithTimestamp, error)
	GetBlockReceivedByHash(context.Context, types.Hash) ([]BidTraceWithTimestamp, error)
	GetBlockReceivedByNum(context.Context, uint64) ([]BidTraceWithTimestamp, error)
	GetTailBlockReceived(context.Context, uint64) ([]BidTraceWithTimestamp, error)
	Registration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type DefaultService struct {
	Log             log.Logger
	Config          Config
	Relay           Relay
	Storage         TTLStorage
	Datastore       Datastore
	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	// state
	state        atomicState
	headslotSlot Slot
	updateTime   atomic.Value
}

// Run creates a relay, datastore and starts the beacon client event loop
func (s *DefaultService) Run(ctx context.Context) (err error) {
	if s.Log == nil {
		s.Log = log.New().WithField("service", "RelayService")
	}

	timeRelayStart := time.Now()
	if s.Relay == nil {
		s.Relay, err = NewRelay(s.Config)
		if err != nil {
			return
		}
	}
	s.Log.WithFields(logrus.Fields{
		"service":     "relay",
		"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
	}).Info("initialized")

	timeDataStoreStart := time.Now()
	if s.Datastore == nil {
		if s.Storage == nil {
			storage, err := badger.NewDatastore(s.Config.Datadir, &badger.DefaultOptions)
			if err != nil {
				s.Log.WithError(err).Fatal("failed to initialize datastore")
				return err
			}
			s.Storage = &TTLDatastoreBatcher{storage}
		}

		s.Datastore = &DefaultDatastore{Storage: s.Storage}
	}
	s.Log.
		WithFields(logrus.Fields{
			"service":     "datastore",
			"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
		}).Info("data store initialized")

	s.state.datastore.Store(s.Datastore)

	if s.NewBeaconClient == nil {
		s.NewBeaconClient = func() (BeaconClient, error) {
			clients := make([]BeaconClient, 0, len(s.Config.BeaconEndpoints))
			for _, endpoint := range s.Config.BeaconEndpoints {
				client, err := NewBeaconClient(endpoint, s.Config)
				if err != nil {
					return nil, err
				}
				clients = append(clients, client)
			}
			return NewMultiBeaconClient(s.Config.Log.WithField("service", "multi-beacon client"), clients), nil
		}
	}

	client, err := s.NewBeaconClient()
	if err != nil {
		s.Log.WithError(err).Warn("failed beacon client registration")
		return err
	}

	s.Log.Info("beacon client initialized")

	return s.beaconEventLoop(ctx, client)
}

func (s *DefaultService) Ready() <-chan struct{} {
	s.once.Do(func() {
		s.ready = make(chan struct{})
	})
	return s.ready
}

func (s *DefaultService) setReady() {
	select {
	case <-s.Ready():
	default:
		close(s.ready)
	}
}

func (s *DefaultService) beaconEventLoop(ctx context.Context, client BeaconClient) error {
	syncStatus, err := client.SyncStatus()
	if err != nil {
		return err
	}
	if syncStatus.IsSyncing {
		return ErrBeaconNodeSyncing
	}

	err = s.updateProposerDuties(ctx, client, Slot(syncStatus.HeadSlot))
	if err != nil {
		return err
	}

	defer s.Log.Debug("beacon loop stopped")

	events := make(chan HeadEvent)

	client.SubscribeToHeadEvents(ctx, events)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if err := s.processNewSlot(ctx, client, ev); err != nil {
				s.Log.
					With(ev).
					WithError(err).
					Warn("error processing slot")
				continue
			}
		}
	}
}

func (s *DefaultService) processNewSlot(ctx context.Context, client BeaconClient, event HeadEvent) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")
	timeStart := time.Now()

	received := Slot(event.Slot)
	if received <= s.headslotSlot {
		return nil
	}

	if s.headslotSlot > 0 {
		for slot := s.headslotSlot + 1; slot < received; slot++ {
			s.Log.Warnf("missedSlot %d", slot)
		}
	}

	s.headslotSlot = received

	logger.With(log.F{
		"epoch":              s.headslotSlot.Epoch(),
		"slotHead":           s.headslotSlot,
		"slotStartNextEpoch": Slot(s.headslotSlot.Epoch()+1) * SlotsPerEpoch,
	},
	).Debugf("updated headSlot to %d", received)

	// update proposer duties and known validators in the background
	if (DurationPerEpoch / 2) < time.Since(s.knownValidatorsUpdateTime()) { // only update every half DurationPerEpoch
		go func() {
			if err := s.updateKnownValidators(ctx, client, s.headslotSlot); err != nil {
				s.Log.WithError(err).Warn("failed to update known validators")
			} else {
				s.updateTime.Store(time.Now())
				s.setReady()
			}
		}()
	}

	if err := s.updateProposerDuties(ctx, client, s.headslotSlot); err != nil {
		return err
	}

	logger.With(log.F{
		"epoch":              s.headslotSlot.Epoch(),
		"slotHead":           s.headslotSlot,
		"slotStartNextEpoch": Slot(s.headslotSlot.Epoch()+1) * SlotsPerEpoch,
		"slot":               uint64(s.headslotSlot),
		"processingTimeMs":   time.Since(timeStart).Milliseconds(),
	}).Info("updated head slot")

	return nil
}

func (s *DefaultService) knownValidatorsUpdateTime() time.Time {
	updateTime, ok := s.updateTime.Load().(time.Time)
	if !ok {
		return time.Time{}
	}
	return updateTime
}

func (s *DefaultService) updateProposerDuties(ctx context.Context, client BeaconClient, headSlot Slot) error {
	logger := s.Log.WithField("method", "UpdateProposerDuties")
	timeStart := time.Now()

	state := dutiesState{}

	epoch := headSlot.Epoch()

	l := s.Log.With(log.F{
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

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

	state.proposerDutiesResponse = make(BuilderGetValidatorsResponseEntrySlice, 0, len(entries))
	state.currentSlot = headSlot

	for _, e := range entries {
		reg, err := s.Datastore.GetRegistration(ctx, e.PubKey)
		if err == nil {
			l.With(log.F{
				"method": "UpdateProposerDuties",
				"slot":   e.Slot},
			).With(e.PubKey).
				Debug("updating Proposer duties")

			state.proposerDutiesResponse = append(state.proposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
				Slot:  e.Slot,
				Entry: &reg,
			})
		}
	}

	s.state.duties.Store(state)

	logger.With(log.F{
		"epochFrom":        epoch,
		"epochTo":          epoch + 1,
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).With(state.proposerDutiesResponse).Debug("proposer duties updated")

	return nil
}

func (s *DefaultService) updateKnownValidators(ctx context.Context, client BeaconClient, current Slot) error {
	logger := s.Log.WithField("method", "UpdateKnownValidators")
	timeStart := time.Now()

	state := validatorsState{}
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

	state.knownValidators = knownValidators
	state.knownValidatorsByIndex = knownValidatorsByIndex

	s.state.validators.Store(state)

	logger.With(log.F{
		"slotHead":         uint64(current),
		"numValidators":    len(knownValidators),
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Debug("updated known validators")

	return nil
}

func (s *DefaultService) RegisterValidator(ctx context.Context, payload []types.SignedValidatorRegistration) error {
	return s.Relay.RegisterValidator(ctx, payload, &s.state)
}

func (s *DefaultService) GetHeader(ctx context.Context, request HeaderRequest) (*types.GetHeaderResponse, error) {
	return s.Relay.GetHeader(ctx, request, &s.state)
}

func (s *DefaultService) GetPayload(ctx context.Context, payloadRequest *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) {
	return s.Relay.GetPayload(ctx, payloadRequest, &s.state)
}

func (s *DefaultService) SubmitBlock(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	return s.Relay.SubmitBlock(ctx, submitBlockRequest, &s.state)
}

func (s *DefaultService) GetValidators() BuilderGetValidatorsResponseEntrySlice {
	return s.Relay.GetValidators(&s.state)
}

func (s *DefaultService) GetDelivered(ctx context.Context, slot Slot) ([]types.BidTrace, error) {
	event, err := s.state.Datastore().GetHeader(ctx, slot, true)
	if err == nil {
		return []types.BidTrace{event.Trace.BidTrace}, err
	}
	return nil, err
}

func (s *DefaultService) GetDeliveredByHash(ctx context.Context, bh types.Hash) ([]types.BidTrace, error) {
	event, err := s.state.Datastore().GetHeaderByBlockHash(ctx, bh, true)
	if err == nil {
		return []types.BidTrace{event.Trace.BidTrace}, err
	}
	return nil, err
}

func (s *DefaultService) GetDeliveredByNum(ctx context.Context, bn uint64) ([]types.BidTrace, error) {
	event, err := s.state.Datastore().GetHeaderByBlockNum(ctx, bn, true)
	if err == nil {
		return []types.BidTrace{event.Trace.BidTrace}, err
	}
	return nil, err
}

func (s *DefaultService) GetDeliveredByPubKey(ctx context.Context, pk types.PublicKey) ([]types.BidTrace, error) {
	event, err := s.state.Datastore().GetHeaderByPubkey(ctx, pk, true)
	if err == nil {
		return []types.BidTrace{event.Trace.BidTrace}, err
	}
	return nil, err
}

func (s *DefaultService) GetTailDelivered(ctx context.Context, limit uint64) ([]types.BidTrace, error) {
	stop := s.state.Beacon().HeadSlot() - Slot(s.Config.TTL/DurationPerSlot)
	return s.getTailDelivered(ctx, limit, stop)
}
func (s *DefaultService) GetTailDeliveredCursor(ctx context.Context, limit, cursor uint64) ([]types.BidTrace, error) {
	headSlot := s.state.Beacon().HeadSlot()
	stop := Slot(cursor)
	if headSlot <= stop {
		return nil, errors.New("invalid cursor, is higher than headslot range")
	}
	if maxStop := s.state.Beacon().HeadSlot() - Slot(s.Config.TTL/DurationPerSlot); stop < maxStop {
		stop = maxStop
	}
	return s.getTailDelivered(ctx, limit, stop)
}

func (s *DefaultService) getTailDelivered(ctx context.Context, limit uint64, stop Slot) ([]types.BidTrace, error) {
	batch := make([]HeaderAndTrace, 0, limit)
	slots := make([]Slot, 0, limit)

	s.Log.WithField("limit", limit).
		WithField("start", s.state.Beacon().HeadSlot()).
		WithField("stop", stop).
		Debug("getting delivered traces")

	for highSlot := s.state.Beacon().HeadSlot(); len(batch) < int(limit) && stop <= highSlot; highSlot -= Slot(limit) {
		slots = slots[:0]
		for s := highSlot; highSlot-Slot(limit) < s; s-- {
			slots = append(slots, s)
		}

		nextBatch, err := s.state.Datastore().GetHeaderBatch(ctx, slots, true)
		if err != nil {
			s.Log.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]types.BidTrace, 0, len(batch))
	for _, event := range batch {
		events = append(events, event.Trace.BidTrace)
	}
	return events, nil
}

func (s *DefaultService) GetBlockReceived(ctx context.Context, slot Slot) ([]BidTraceWithTimestamp, error) {
	event, err := s.state.Datastore().GetHeader(ctx, slot, false)
	if err == nil {
		return []BidTraceWithTimestamp{*event.Trace}, err
	}
	return nil, err
}

func (s *DefaultService) GetBlockReceivedByHash(ctx context.Context, bh types.Hash) ([]BidTraceWithTimestamp, error) {
	event, err := s.state.Datastore().GetHeaderByBlockHash(ctx, bh, false)
	if err == nil {
		return []BidTraceWithTimestamp{*event.Trace}, err
	}
	return nil, err
}

func (s *DefaultService) GetBlockReceivedByNum(ctx context.Context, bn uint64) ([]BidTraceWithTimestamp, error) {
	event, err := s.state.Datastore().GetHeaderByBlockNum(ctx, bn, false)
	if err == nil {
		return []BidTraceWithTimestamp{*event.Trace}, err
	}
	return nil, err
}

func (s *DefaultService) GetTailBlockReceived(ctx context.Context, limit uint64) ([]BidTraceWithTimestamp, error) {
	batch := make([]HeaderAndTrace, 0, limit)
	stop := s.state.Beacon().HeadSlot() - Slot(s.Config.TTL/DurationPerSlot)
	slots := make([]Slot, 0)

	s.Log.WithField("limit", limit).
		WithField("start", s.state.Beacon().HeadSlot()).
		WithField("stop", stop).
		Debug("getting received traces")

	for highSlot := s.state.Beacon().HeadSlot(); len(batch) < int(limit) && stop <= highSlot; highSlot -= Slot(limit) {
		slots = slots[:0]
		for s := highSlot; highSlot-Slot(limit) < s; s-- {
			slots = append(slots, s)
		}

		nextBatch, err := s.state.Datastore().GetHeaderBatch(ctx, slots, false)
		if err != nil {
			s.Log.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]BidTraceWithTimestamp, 0, len(batch))
	for _, event := range batch {
		events = append(events, *event.Trace)
	}
	return events, nil
}

func (s *DefaultService) Registration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	return s.Datastore.GetRegistration(ctx, PubKey{pk})
}

type atomicState struct {
	datastore  atomic.Value
	duties     atomic.Value
	validators atomic.Value
}

func (as *atomicState) Datastore() Datastore { return as.datastore.Load().(Datastore) }

func (as *atomicState) Beacon() BeaconState {
	duties := as.duties.Load().(dutiesState)
	validators := as.validators.Load().(validatorsState)
	return beaconState{dutiesState: duties, validatorsState: validators}
}

type beaconState struct {
	dutiesState
	validatorsState
}

func (s beaconState) KnownValidatorByIndex(index uint64) (types.PubkeyHex, error) {
	pk, ok := s.knownValidatorsByIndex[index]
	if !ok {
		return "", ErrUnknownValue
	}
	return pk, nil
}

func (s beaconState) IsKnownValidator(pk types.PubkeyHex) (bool, error) {
	_, ok := s.knownValidators[pk]
	return ok, nil
}

func (s beaconState) KnownValidators() map[types.PubkeyHex]struct{} {
	return s.knownValidators
}

func (s beaconState) HeadSlot() Slot {
	return s.currentSlot
}

func (s beaconState) ValidatorsMap() BuilderGetValidatorsResponseEntrySlice {
	return s.proposerDutiesResponse
}

type dutiesState struct {
	currentSlot            Slot
	proposerDutiesResponse BuilderGetValidatorsResponseEntrySlice
}

type validatorsState struct {
	knownValidatorsByIndex map[uint64]types.PubkeyHex
	knownValidators        map[types.PubkeyHex]struct{}
}
