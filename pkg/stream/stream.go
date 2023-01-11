//go:generate mockgen  -destination=./mocks/stream.go -package=mocks github.com/blocknative/dreamboat/pkg/stream Pubsub,RemoteDatastore
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"

	"github.com/blocknative/dreamboat/pkg/datastore"
	"github.com/blocknative/dreamboat/pkg/structs"
)

type Pubsub interface {
	Publish(context.Context, string, []byte) error
	Subscribe(context.Context, string) (chan []byte, error)
}

type RemoteDatastore interface {
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockAndTrace, error)
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockAndTrace, time.Duration) error
}

type StreamConfig struct {
	Logger          log.Logger
	ID              string
	TTL             time.Duration
	PubsubTopic     string // pubsub topic name for block submissions
	PublishAll      bool
	StreamQueueSize int
}

type StreamDatastore struct {
	*datastore.Datastore
	Pubsub          Pubsub
	RemoteDatastore RemoteDatastore

	cacheRequests chan *structs.BlockAndTrace
	storeRequests chan *structs.BlockAndTrace

	Config StreamConfig
	Logger log.Logger

	m StreamMetrics
}

func NewStreamDatastore(ds *datastore.Datastore, ps Pubsub, rds RemoteDatastore, cfg StreamConfig) *StreamDatastore {
	s := StreamDatastore{
		Datastore:       ds,
		Pubsub:          ps,
		RemoteDatastore: rds,
		cacheRequests:   make(chan *structs.BlockAndTrace, cfg.StreamQueueSize),
		storeRequests:   make(chan *structs.BlockAndTrace, cfg.StreamQueueSize),
		Config:          cfg,
		Logger:          cfg.Logger.WithField("relay-service", "stream"),
	}

	s.initMetrics()

	return &s
}

func (s *StreamDatastore) RunSubscriber(ctx context.Context) error {
	blocks, err := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic)
	if err != nil {
		return err
	}

	sBlock := StreamBlock{}
	for rawSBlock := range blocks {
		if err := proto.Unmarshal(rawSBlock, &sBlock); err != nil {
			s.Logger.Warnf("fail to decode stream block: %s", err.Error())
		}

		if sBlock.Source == s.Config.ID {
			continue
		}

		block := ToBlockAndTrace(&sBlock)
		if sBlock.IsCache {
			s.m.StreamRecvCounter.WithLabelValues("cache").Inc()
			if err := s.cachePayload(ctx, block); err != nil {
				s.Logger.With(block).Warnf("fail to cache payload: %s", err.Error())
			}
		} else {
			s.m.StreamRecvCounter.WithLabelValues("store").Inc()
			if err := s.storePayload(ctx, block); err != nil {
				s.Logger.With(block).Warnf("fail to store payload: %s", err.Error())
			}
		}

	}

	return ctx.Err()
}

func (s *StreamDatastore) RunPublisher(ctx context.Context) error {
	for {
		select {
		case req := <-s.cacheRequests:
			s.encodeAndPublish(ctx, req, true)
			continue
		default:
		}

		select {
		case req := <-s.cacheRequests:
			s.encodeAndPublish(ctx, req, true)
		case req := <-s.storeRequests:
			s.encodeAndPublish(ctx, req, false)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *StreamDatastore) encodeAndPublish(ctx context.Context, block *structs.BlockAndTrace, isCache bool) {
	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "encode"))
	protoBlock := ToStreamBlock(block, isCache, s.Config.ID)
	rawBlock, err := proto.Marshal(protoBlock)
	if err != nil {
		s.Logger.Warnf("fail to encode encode and stream block: %s", err.Error())
		timer1.ObserveDuration()
		return
	}
	timer1.ObserveDuration()

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "publish"))
	defer timer2.ObserveDuration()
	if err := s.Pubsub.Publish(ctx, s.Config.PubsubTopic, rawBlock); err != types.ErrNilPayload {
		s.Logger.Warnf("fail to encode encode and stream block: %s", err.Error())
		return
	}
}

func (s *StreamDatastore) cachePayload(ctx context.Context, payload *structs.BlockAndTrace) error {
	header, err := types.PayloadToPayloadHeader(payload.Payload.Data)
	if err != nil {
		return err
	}

	h := structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 payload.Trace.Message.Slot,
					ParentHash:           payload.Payload.Data.ParentHash,
					BlockHash:            payload.Payload.Data.BlockHash,
					BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: payload.Trace.Message.ProposerFeeRecipient,
					Value:                payload.Trace.Message.Value,
					GasLimit:             payload.Trace.Message.GasLimit,
					GasUsed:              payload.Trace.Message.GasUsed,
				},
				BlockNumber: payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(payload.Payload.Data.Transactions)),
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
	}

	completeBlock := structs.CompleteBlockstruct{
		Payload: *payload,
		Header:  h,
	}

	return s.Datastore.CacheBlock(ctx, &completeBlock)
}

func (s *StreamDatastore) storePayload(ctx context.Context, payload *structs.BlockAndTrace) error {
	if err := s.Datastore.PutPayload(ctx, payloadToKey(payload), payload, s.Config.TTL); err != nil {
		return err
	}

	header, err := types.PayloadToPayloadHeader(payload.Payload.Data)
	if err != nil {
		return err
	}

	h := structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 payload.Trace.Message.Slot,
					ParentHash:           payload.Payload.Data.ParentHash,
					BlockHash:            payload.Payload.Data.BlockHash,
					BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: payload.Trace.Message.ProposerFeeRecipient,
					Value:                payload.Trace.Message.Value,
					GasLimit:             payload.Trace.Message.GasLimit,
					GasUsed:              payload.Trace.Message.GasUsed,
				},
				BlockNumber: payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(payload.Payload.Data.Transactions)),
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
	}

	b, err := json.Marshal(h)
	if err != nil {
		return err
	}

	return s.Datastore.PutHeader(ctx, structs.HeaderData{
		Slot:           structs.Slot(payload.Trace.Message.Slot),
		Marshaled:      b,
		HeaderAndTrace: h,
	}, s.Config.TTL)
}

type getPayloadResponse struct {
	block     *structs.BlockAndTrace
	isLocal   bool
	fromCache bool
	err       error
}

func (s *StreamDatastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockAndTrace, bool, error) {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("getPayload", "all"))
	defer timer0.ObserveDuration()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses := make(chan getPayloadResponse, 2)

	go func(ctx context.Context, resp chan getPayloadResponse) {
		timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("getPayload", "local"))
		block, fromCache, err := s.Datastore.GetPayload(ctx, key)
		timer1.ObserveDuration()
		responses <- getPayloadResponse{block: block, isLocal: true, fromCache: fromCache, err: err}
	}(ctx, responses)

	go func(ctx context.Context, resp chan getPayloadResponse) {
		timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("getPayload", "remote"))
		block, err := s.RemoteDatastore.GetPayload(ctx, key)
		timer1.ObserveDuration()
		responses <- getPayloadResponse{block: block, isLocal: false, fromCache: false, err: err}
	}(ctx, responses)

	for i := 0; i < cap(responses); i++ {
		var resp getPayloadResponse
		select {
		case resp = <-responses:
		case <-ctx.Done():
			return &structs.BlockAndTrace{}, false, ctx.Err()
		}

		if resp.block != nil && resp.err == nil {
			if resp.isLocal {
				s.m.StreamPayloadHitCounter.WithLabelValues("local", "hit").Inc()
			} else {
				s.m.StreamPayloadHitCounter.WithLabelValues("remote", "hit").Inc()
			}
			return resp.block, resp.fromCache, resp.err
		}

		s.Logger.With(key).WithField("isLocal", resp.isLocal).WithError(resp.err).Debug("payload not found")
		if resp.isLocal {
			s.m.StreamPayloadHitCounter.WithLabelValues("local", "miss").Inc()
		} else {
			s.m.StreamPayloadHitCounter.WithLabelValues("remote", "miss").Inc()
		}
	}

	return nil, false, fmt.Errorf("payload not found")
}

func (s *StreamDatastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload *structs.BlockAndTrace, ttl time.Duration) error {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "all"))
	defer timer0.ObserveDuration()

	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "remoteStore"))
	if err := s.RemoteDatastore.PutPayload(ctx, key, payload, ttl); err != nil {
		return err
	}
	timer1.ObserveDuration()

	timer1 = prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "localStore"))
	if err := s.Datastore.PutPayload(ctx, key, payload, ttl); err != nil {
		return err
	}
	timer1.ObserveDuration()

	// if PublishAll is deactivated, do not publish to Pubsub
	if !s.Config.PublishAll {
		return nil
	}

	timer1 = prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "pub"))
	defer timer1.ObserveDuration()

	select {
	case s.storeRequests <- payload:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *StreamDatastore) CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("cacheBlock", "all"))
	defer timer0.ObserveDuration()

	if err := s.Datastore.CacheBlock(ctx, block); err != nil {
		return err
	}

	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("cacheBlock", "pub"))
	defer timer1.ObserveDuration()

	select {
	case s.cacheRequests <- &block.Payload:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func payloadToKey(payload *structs.BlockAndTrace) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: payload.Payload.Data.BlockHash,
		Proposer:  payload.Trace.Message.ProposerPubkey,
		Slot:      structs.Slot(payload.Trace.Message.Slot),
	}
}
