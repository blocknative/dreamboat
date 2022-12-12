//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg Relay,Datastore,BeaconClient
package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/lthibault/log"
)

const (
	Version = "0.2.8"
)

var (
	ErrBeaconNodeSyncing = errors.New("beacon node is syncing")
)

type Relay interface {
	// Proposer API
	RegisterValidator(context.Context, []structs.SignedValidatorRegistration) error
	GetHeader(context.Context, structs.HeaderRequest) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error)

	// Builder APIs
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest) error
	GetValidators() structs.BuilderGetValidatorsResponseEntrySlice

	AttachMetrics(m *metrics.Metrics)
}

type Datastore interface {
	GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error)
	GetHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.HeaderAndTrace, error)
	GetHeadersByBlockNum(ctx context.Context, num uint64) ([]structs.HeaderAndTrace, error)
	GetLatestHeaders(ctx context.Context, limit uint64, stopLag uint64) ([]structs.HeaderAndTrace, error)

	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, structs.PayloadQuery) (structs.BidTraceWithTimestamp, error)
	GetDeliveredBatch(context.Context, []structs.PayloadQuery) ([]structs.BidTraceWithTimestamp, error)
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error)
	PutRegistrationRaw(context.Context, structs.PubKey, []byte, time.Duration) error
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}

type Service struct {
	Log             log.Logger
	Config          Config
	Relay           Relay
	Datastore       Datastore
	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	// state
	state        *AtomicState
	headslotSlot structs.Slot
	updateTime   atomic.Value
}

func NewService(l log.Logger, c Config, d Datastore, r Relay, as *AtomicState) *Service {
	return &Service{
		Log:       l.WithField("service", "RelayService"),
		Config:    c,
		Datastore: d,
		Relay:     r,
		state:     as,
	}
}

// Run creates a relay, datastore and starts the beacon client event loop
func (s *Service) RunBeacon(ctx context.Context) (err error) {
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

func (s *Service) beaconEventLoop(ctx context.Context, client BeaconClient) error {
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

	err = s.updateProposerDuties(ctx, client, structs.Slot(syncStatus.HeadSlot))
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

func (s *Service) processNewSlot(ctx context.Context, client BeaconClient, event HeadEvent) error {
	logger := s.Log.WithField("method", "ProcessNewSlot")
	timeStart := time.Now()

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

	logger.With(log.F{
		"epoch":              s.headslotSlot.Epoch(),
		"slotHead":           s.headslotSlot,
		"slotStartNextEpoch": structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
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
		"slotStartNextEpoch": structs.Slot(s.headslotSlot.Epoch()+1) * structs.SlotsPerEpoch,
		"slot":               uint64(s.headslotSlot),
		"processingTimeMs":   time.Since(timeStart).Milliseconds(),
	}).Info("updated head slot")

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

	timeStart := time.Now()

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
			logger.With(e.PubKey).
				Debug("new proposer duty")

			state.ProposerDutiesResponse = append(state.ProposerDutiesResponse, types.BuilderGetValidatorsResponseEntry{
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
	}).With(state.ProposerDutiesResponse).Debug("proposer duties updated")

	return nil
}

func (s *Service) updateKnownValidators(ctx context.Context, client BeaconClient, current structs.Slot) error {
	logger := s.Log.WithField("method", "UpdateKnownValidators")
	timeStart := time.Now()

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

	logger.With(log.F{
		"slotHead":         uint64(current),
		"numValidators":    len(knownValidators),
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Debug("updated known validators")

	return nil
}

func (s *Service) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) error {
	return s.Relay.RegisterValidator(ctx, payload)
}

func (s *Service) GetHeader(ctx context.Context, request structs.HeaderRequest) (*types.GetHeaderResponse, error) {
	return s.Relay.GetHeader(ctx, request)
}

func (s *Service) GetPayload(ctx context.Context, payloadRequest *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) {
	return s.Relay.GetPayload(ctx, payloadRequest)
}

func (s *Service) SubmitBlock(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	return s.Relay.SubmitBlock(ctx, submitBlockRequest)
}

func (s *Service) GetValidators() structs.BuilderGetValidatorsResponseEntrySlice {
	return s.Relay.GetValidators()
}

func (s *Service) Registration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	return s.Datastore.GetRegistration(ctx, structs.PubKey{PublicKey: pk})
}

func (s *Service) GetPayloadDelivered(ctx context.Context, query structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error) {
	var (
		event structs.BidTraceWithTimestamp
		err   error
	)

	if query.HasSlot() {
		event, err = s.Datastore.GetDelivered(ctx, structs.PayloadQuery{Slot: query.Slot})
	} else if query.HasBlockHash() {
		event, err = s.Datastore.GetDelivered(ctx, structs.PayloadQuery{BlockHash: query.BlockHash})
	} else if query.HasBlockNum() {
		event, err = s.Datastore.GetDelivered(ctx, structs.PayloadQuery{BlockNum: query.BlockNum})
	} else if query.HasPubkey() {
		event, err = s.Datastore.GetDelivered(ctx, structs.PayloadQuery{PubKey: query.Pubkey})
	} else {
		return s.getTailDelivered(ctx, query.Limit, query.Cursor)
	}

	if err == nil {
		return []structs.BidTraceExtended{{BidTrace: event.BidTrace, BlockNumber: event.BlockNumber, NumTx: event.NumTx}}, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceExtended{}, nil
	}
	return nil, err
}

func (s *Service) getTailDelivered(ctx context.Context, limit, cursor uint64) ([]structs.BidTraceExtended, error) {
	headSlot := s.state.Beacon().HeadSlot()
	start := headSlot
	if cursor != 0 {
		start = min(headSlot, structs.Slot(cursor))
	}

	stop := start - structs.Slot(s.Config.TTL/DurationPerSlot)

	batch := make([]structs.BidTraceWithTimestamp, 0, limit)
	queries := make([]structs.PayloadQuery, 0, limit)

	s.Log.WithField("limit", limit).
		WithField("start", start).
		WithField("stop", stop).
		Debug("querying delivered payload traces")

	for highSlot := start; len(batch) < int(limit) && stop <= highSlot; highSlot -= structs.Slot(limit) {
		queries = queries[:0]
		for s := highSlot; highSlot-structs.Slot(limit) < s && stop <= s; s-- {
			queries = append(queries, structs.PayloadQuery{Slot: s})
		}

		nextBatch, err := s.Datastore.GetDeliveredBatch(ctx, queries)
		if err != nil {
			s.Log.WithError(err).Warn("failed getting header batch")
		} else {
			batch = append(batch, nextBatch[:min(int(limit)-len(batch), len(nextBatch))]...)
		}
	}

	events := make([]structs.BidTraceExtended, 0, len(batch))
	for _, event := range batch {
		events = append(events, event.BidTraceExtended)
	}
	return events, nil
}

func (s *Service) GetBlockReceived(ctx context.Context, query structs.HeaderTraceQuery) ([]structs.BidTraceWithTimestamp, error) {
	var (
		events []structs.HeaderAndTrace
		err    error
	)

	if query.HasSlot() {
		events, err = s.Datastore.GetHeadersBySlot(ctx, uint64(query.Slot))
	} else if query.HasBlockHash() {
		events, err = s.Datastore.GetHeadersByBlockHash(ctx, query.BlockHash)
	} else if query.HasBlockNum() {
		events, err = s.Datastore.GetHeadersByBlockNum(ctx, query.BlockNum)
	} else {
		events, err = s.Datastore.GetLatestHeaders(ctx, query.Limit, uint64(s.Config.TTL/DurationPerSlot))
	}

	if err == nil {
		traces := make([]structs.BidTraceWithTimestamp, 0, len(events))
		for _, event := range events {
			traces = append(traces, *event.Trace)
		}
		return traces, err
	} else if errors.Is(err, ds.ErrNotFound) {
		return []structs.BidTraceWithTimestamp{}, nil
	}
	return nil, err
}
