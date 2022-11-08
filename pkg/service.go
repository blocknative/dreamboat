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
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger2"
	"github.com/lthibault/log"
	"github.com/sirupsen/logrus"
)

const (
	Version = "0.2.8"
)

type RelayService interface {
	// Proposer APIs (builder spec https://github.com/ethereum/builder-specs)
	RegisterValidator(context.Context, []SignedValidatorRegistration) error
	GetHeader(context.Context, HeaderRequest) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error)

	// Builder APIs (relay spec https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5)
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest) error
	GetValidators() BuilderGetValidatorsResponseEntrySlice

	// Data APIs
	GetPayloadDelivered(context.Context, TraceQuery) ([]BidTraceExtended, error)
	GetBlockReceived(context.Context, TraceQuery) ([]BidTraceWithTimestamp, error)
	Registration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type TraceQuery struct {
	Slot          Slot
	BlockHash     types.Hash
	BlockNum      uint64
	Pubkey        types.PublicKey
	Cursor, Limit uint64
}

func (q TraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q TraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q TraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q TraceQuery) HasPubkey() bool {
	return q.Pubkey != types.PublicKey{}
}

func (q TraceQuery) HasCursor() bool {
	return q.Cursor != 0
}

func (q TraceQuery) HasLimit() bool {
	return q.Limit != 0
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

		s.Datastore = &DefaultDatastore{TTLStorage: s.Storage}
	}
	s.Log.
		WithFields(logrus.Fields{
			"service":     "datastore",
			"startTimeMs": time.Since(timeDataStoreStart).Milliseconds(),
		}).Info("data store initialized")

	s.state.datastore.Store(s.Datastore)

	timeRelayStart := time.Now()
	if s.Relay == nil {
		s.Relay, err = NewRelay(s.Config, s.Datastore)
		if err != nil {
			return
		}
	}
	s.Log.WithFields(logrus.Fields{
		"service":     "relay",
		"startTimeMs": time.Since(timeRelayStart).Milliseconds(),
	}).Info("initialized")

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

	genesis, err := client.Genesis()
	if err != nil {
		return fmt.Errorf("fail to get genesis from beacon: %w", err)
	}
	s.state.genesis.Store(genesis)
	s.Log.
		WithField("genesis-time", time.Unix(int64(genesis.GenesisTime), 0)).
		Info("genesis retrieved")

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
	epoch := headSlot.Epoch()

	logger := s.Log.With(log.F{
		"method":    "UpdateProposerDuties",
		"slot":      headSlot,
		"epochFrom": epoch,
		"epochTo":   epoch + 1,
	})

	timeStart := time.Now()

	state := dutiesState{}

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
			logger.With(e.PubKey).
				Debug("new proposer duty")

			state.proposerDutiesResponse = append(state.proposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
				Slot:  e.Slot,
				Entry: &reg,
			})
		} else if err != nil && !errors.Is(err, ds.ErrNotFound) {
			logger.Warn(err)
		}
	}

	s.state.duties.Store(state)

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"receivedDuties":   len(entries),
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

func (s *DefaultService) RegisterValidator(ctx context.Context, payload []SignedValidatorRegistration) error {
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

func (s *DefaultService) GetPayloadDelivered(ctx context.Context, query TraceQuery) ([]BidTraceExtended, error) {
	var (
		event BidTraceWithTimestamp
		err   error
	)

	if query.HasSlot() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{Slot: query.Slot})
	} else if query.HasBlockHash() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{BlockNum: query.BlockNum})
	} else if query.HasPubkey() {
		event, err = s.state.Datastore().GetDelivered(ctx, Query{PubKey: query.Pubkey})
	} else {
		return s.getTailDelivered(ctx, query.Limit, query.Cursor)
	}

	if err == nil {
		return []BidTraceExtended{{BidTrace: event.BidTrace, BlockNumber: event.BlockNumber, NumTx: event.NumTx}}, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []BidTraceExtended{}, nil
	}
	return nil, err
}

func (s *DefaultService) getTailDelivered(ctx context.Context, limit, cursor uint64) ([]BidTraceExtended, error) {
	headSlot := s.state.Beacon().HeadSlot()
	start := headSlot
	if cursor != 0 {
		start = min(headSlot, Slot(cursor))
	}

	stop := start - Slot(s.Config.TTL/DurationPerSlot)

	batch := make([]BidTraceWithTimestamp, 0, limit)
	queries := make([]Query, 0, limit)

	s.Log.WithField("limit", limit).
		WithField("start", start).
		WithField("stop", stop).
		Debug("querying delivered payload traces")

	for highSlot := start; len(batch) < int(limit) && stop <= highSlot; highSlot -= Slot(limit) {
		queries = queries[:0]
		for s := highSlot; highSlot-Slot(limit) < s && stop <= s; s-- {
			queries = append(queries, Query{Slot: s})
		}

		nextBatch, err := s.state.Datastore().GetDeliveredBatch(ctx, queries)
		if err != nil {
			s.Log.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]BidTraceExtended, 0, len(batch))
	for _, event := range batch {
		events = append(events, event.BidTraceExtended)
	}
	return events, nil
}

func (s *DefaultService) GetBlockReceived(ctx context.Context, query TraceQuery) ([]BidTraceWithTimestamp, error) {
	var (
		events []HeaderAndTrace
		err    error
	)

	if query.HasSlot() {
		events, err = s.state.Datastore().GetHeaders(ctx, Query{Slot: query.Slot})
	} else if query.HasBlockHash() {
		events, err = s.state.Datastore().GetHeaders(ctx, Query{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		events, err = s.state.Datastore().GetHeaders(ctx, Query{BlockNum: query.BlockNum})
	} else {
		return s.getTailBlockReceived(ctx, query.Limit)
	}

	if err == nil {
		traces := make([]BidTraceWithTimestamp, 0, len(events))
		for _, event := range events {
			traces = append(traces, *event.Trace)
		}
		return traces, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []BidTraceWithTimestamp{}, nil
	}
	return nil, err
}

func (s *DefaultService) getTailBlockReceived(ctx context.Context, limit uint64) ([]BidTraceWithTimestamp, error) {
	batch := make([]HeaderAndTrace, 0, limit)
	stop := s.state.Beacon().HeadSlot() - Slot(s.Config.TTL/DurationPerSlot)
	queries := make([]Query, 0)

	s.Log.WithField("limit", limit).
		WithField("start", s.state.Beacon().HeadSlot()).
		WithField("stop", stop).
		Debug("querying received block traces")

	for highSlot := s.state.Beacon().HeadSlot(); len(batch) < int(limit) && stop <= highSlot; highSlot -= Slot(limit) {
		queries = queries[:0]
		for s := highSlot; highSlot-Slot(limit) < s && stop <= s; s-- {
			queries = append(queries, Query{Slot: s})
		}

		nextBatch, err := s.state.Datastore().GetHeaderBatch(ctx, queries)
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
	genesis    atomic.Value
}

func (as *atomicState) Datastore() Datastore { return as.datastore.Load().(Datastore) }

func (as *atomicState) Beacon() BeaconState {
	duties := as.duties.Load().(dutiesState)
	validators := as.validators.Load().(validatorsState)
	genesis := as.genesis.Load().(GenesisInfo)
	return beaconState{dutiesState: duties, validatorsState: validators, GenesisInfo: genesis}
}

type beaconState struct {
	dutiesState
	validatorsState
	GenesisInfo
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

func (s beaconState) Genesis() GenesisInfo {
	return s.GenesisInfo
}

type dutiesState struct {
	currentSlot            Slot
	proposerDutiesResponse BuilderGetValidatorsResponseEntrySlice
}

type validatorsState struct {
	knownValidatorsByIndex map[uint64]types.PubkeyHex
	knownValidators        map[types.PubkeyHex]struct{}
}
