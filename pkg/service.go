//go:generate mockgen -source=service.go -destination=../internal/mock/pkg/service.go -package=mock_relay
package relay

import (
	"context"
	"errors"
	"strings"
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
	GetPayloadDelivered(context.Context, TraceQuery) ([]types.BidTrace, error)
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

type DefaultService struct {
	Log             log.Logger
	Config          Config
	Relay           Relay
	Storage         TTLStorage
	Datastore       Datastore
	NewBeaconClient func() (BeaconClient, error)

	once  sync.Once
	ready chan struct{}

	state atomicState
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

	if s.NewBeaconClient == nil {
		s.NewBeaconClient = func() (BeaconClient, error) {
			return NewBeaconClient(s.Config)
		}
	}

	client, err := s.NewBeaconClient()
	if err != nil {
		s.Log.WithError(err).Warn("failed beacon client registration")
		return err
	}
	s.Log.Info("beacon client initialized")

	s.state.Upsert(s.Datastore, client)

	return s.handleBeaconClient(ctx, client)
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

// handleBeaconClient starts beacon cliet subscription and restart if connection fails or exits
func (s *DefaultService) handleBeaconClient(ctx context.Context, client BeaconClient) error {
	logger := s.Log.WithField("methodName", "handleBeaconClient")
	var err error

	for ctx.Err() == nil {
		if err := s.beaconEventLoop(ctx, client); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {

				return err
			}
			logger.WithError(err).Debug("beacon connection failed")
		}
		client, err = s.NewBeaconClient()
		if err != nil {
			return err
		}
		s.state.Upsert(s.Datastore, client)
	}
	return ctx.Err()
}

// beaconEventLoop subscribes to and process beacon head events to keep beacon chain data lively
func (s *DefaultService) beaconEventLoop(ctx context.Context, client BeaconClient) error {
	syncStatus, err := client.SyncStatus()
	if err != nil {
		return err
	}
	if syncStatus.IsSyncing {
		return ErrBeaconNodeSyncing
	}

	err = client.UpdateProposerDuties(ctx, Slot(syncStatus.HeadSlot), s.Datastore)
	if err != nil {
		return err
	}

	for ev := range client.SubscribeToHeadEvents(ctx) {
		if err := client.ProcessNewSlot(ctx, Slot(ev.Slot), s.Datastore); err != nil {
			if errors.Is(err, ErrOldSlot) {
				s.Log.
					With(ev).
					WithError(err).
					Warn("received old slot")
				continue
			}
			return err
		} else {
			s.setReady()
		}
	}
	s.Log.Debug("end of event loop")
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

func (s *DefaultService) GetPayloadDelivered(ctx context.Context, query TraceQuery) ([]types.BidTrace, error) {
	var (
		event HeaderAndTrace
		err   error
	)

	if query.Slot > Slot(0) {
		event, err = s.state.Datastore().GetHeader(ctx, query.Slot, true)
	} else if strings.Compare(query.BlockHash.String(), "0x0000000000000000000000000000000000000000000000000000000000000000") != 0 {
		event, err = s.state.Datastore().GetHeaderByBlockHash(ctx, query.BlockHash, true)
	} else if query.BlockNum != 0 {
		event, err = s.state.Datastore().GetHeaderByBlockNum(ctx, query.BlockNum, true)
	} else if strings.Compare(query.Pubkey.String(), "0x000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000") != 0 {
		event, err = s.state.Datastore().GetHeaderByPubkey(ctx, query.Pubkey, true)
	} else {
		return s.getTailDelivered(ctx, query.Limit, query.Cursor)
	}

	if err == nil {
		return []types.BidTrace{event.Trace.BidTrace}, err
	}
	return nil, err
}

func (s *DefaultService) getTailDelivered(ctx context.Context, limit, cursor uint64) ([]types.BidTrace, error) {
	headSlot := s.state.Beacon().HeadSlot()
	stop := headSlot - Slot(s.Config.TTL/DurationPerSlot)

	if cursor != 0 {
		stop = Max(Slot(cursor), stop)
		if headSlot <= stop {
			return nil, errors.New("invalid cursor, is higher than headslot range")
		}

		limit = uint64(headSlot - stop)
	}

	batch := make([]HeaderAndTrace, 0, limit)
	slots := make([]Slot, 0, limit)

	s.Log.WithField("limit", limit).
		WithField("start", s.state.Beacon().HeadSlot()).
		WithField("stop", stop).
		Debug("querying delivered payload traces")

	for highSlot := s.state.Beacon().HeadSlot(); len(batch) < int(limit) && stop <= highSlot; highSlot -= Slot(limit) {
		slots = slots[:0]
		for s := highSlot; highSlot-Slot(limit) < s && stop <= s; s-- {
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

func (s *DefaultService) GetBlockReceived(ctx context.Context, query TraceQuery) ([]BidTraceWithTimestamp, error) {
	var (
		event HeaderAndTrace
		err   error
	)

	if query.Slot > 0 {
		event, err = s.state.Datastore().GetHeader(ctx, query.Slot, false)
	} else if strings.Compare(query.BlockHash.String(), "0x0000000000000000000000000000000000000000000000000000000000000000") != 0 {
		event, err = s.state.Datastore().GetHeaderByBlockHash(ctx, query.BlockHash, false)
	} else if query.BlockNum != 0 {
		event, err = s.state.Datastore().GetHeaderByBlockNum(ctx, query.BlockNum, false)
	} else {
		return s.getTailBlockReceived(ctx, query.Limit)
	}

	if err == nil {
		return []BidTraceWithTimestamp{*event.Trace}, err
	}
	return nil, err
}

func (s *DefaultService) getTailBlockReceived(ctx context.Context, limit uint64) ([]BidTraceWithTimestamp, error) {
	batch := make([]HeaderAndTrace, 0, limit)
	stop := s.state.Beacon().HeadSlot() - Slot(s.Config.TTL/DurationPerSlot)
	slots := make([]Slot, 0)

	s.Log.WithField("limit", limit).
		WithField("start", s.state.Beacon().HeadSlot()).
		WithField("stop", stop).
		Debug("querying received block traces")

	for highSlot := s.state.Beacon().HeadSlot(); len(batch) < int(limit) && stop <= highSlot; highSlot -= Slot(limit) {
		slots = slots[:0]
		for s := highSlot; highSlot-Slot(limit) < s && stop <= s; s-- {
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

type atomicState atomic.Value

func (as *atomicState) load() *state {
	return (*atomic.Value)(as).Load().(*state)
}

func (as *atomicState) Datastore() Datastore { return as.load().Datastore }
func (as *atomicState) Beacon() BeaconClient { return as.load().BeaconClient }

func (as *atomicState) Upsert(ds Datastore, bc BeaconClient) {
	(*atomic.Value)(as).Store(&state{
		Datastore:    ds,
		BeaconClient: bc,
	})
}

type state struct {
	Datastore
	BeaconClient
}
