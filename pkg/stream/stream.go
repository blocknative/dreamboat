//go:generate mockgen  -destination=./mocks/stream.go -package=mocks github.com/blocknative/dreamboat/pkg/stream Pubsub,RemoteDatastore

package stream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type Pubsub interface {
	Publish(context.Context, string, []byte) error
	Subscribe(context.Context, string) (chan []byte, error)
}

type Datastore interface {
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockAndTrace, time.Duration) error
	PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) error
	CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error
}

type StreamConfig struct {
	Logger          log.Logger
	ID              string
	TTL             time.Duration
	PubsubTopic     string // pubsub topic name for block submissions
	StreamQueueSize int
}

type RedisStream struct {
	Pubsub Pubsub

	cacheRequests chan structs.BlockAndTrace
	storeRequests chan structs.BlockAndTrace

	Config StreamConfig
	Logger log.Logger

	m StreamMetrics
}

func NewRedisStream(ps Pubsub, cfg StreamConfig) *RedisStream {
	s := RedisStream{
		Pubsub:        ps,
		cacheRequests: make(chan structs.BlockAndTrace, cfg.StreamQueueSize),
		storeRequests: make(chan structs.BlockAndTrace, cfg.StreamQueueSize),
		Config:        cfg,
		Logger:        cfg.Logger.WithField("relay-service", "stream").WithField("type", "redis"),
	}

	s.initMetrics()

	return &s
}

func (s *RedisStream) RunSubscriberParallel(ctx context.Context, ds Datastore, num uint) error {
	blocks, err := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic)
	if err != nil {
		return err
	}

	for i := uint(0); i < num; i++ {
		go s.RunSubscriber(ctx, ds, blocks)
	}

	return nil
}

func (s *RedisStream) RunSubscriber(ctx context.Context, ds Datastore, blocks chan []byte) error {
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
			if err := s.cachePayload(ctx, ds, block); err != nil {
				s.Logger.WithError(err).With(block).Warn("failed to cache payload")
			}
		} else {
			s.m.StreamRecvCounter.WithLabelValues("store").Inc()
			if err := s.storePayload(ctx, ds, block); err != nil {
				s.Logger.WithError(err).With(block).Warn("failed to store payload: %s")
			}
		}
	}

	return ctx.Err()
}

func (s *RedisStream) RunPublisherParallel(ctx context.Context, num uint) {
	for i := uint(0); i < num; i++ {
		go s.RunPublisher(ctx)
	}
}

func (s *RedisStream) RunPublisher(ctx context.Context) error {
	for {
		select {
		case req := <-s.cacheRequests:
			s.encodeAndPublish(ctx, &req, true)
			continue
		default:
		}

		select {
		case req := <-s.cacheRequests:
			s.encodeAndPublish(ctx, &req, true)
		case req := <-s.storeRequests:
			s.encodeAndPublish(ctx, &req, false)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

}

func (s *RedisStream) PublishStoreBlock() chan structs.BlockAndTrace {
	return s.storeRequests
}

func (s *RedisStream) PublishCacheBlock() chan structs.BlockAndTrace {
	return s.cacheRequests
}

func (s *RedisStream) encodeAndPublish(ctx context.Context, block *structs.BlockAndTrace, isCache bool) {
	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "encode"))
	rawBlock, err := proto.Marshal(ToStreamBlock(block, isCache, s.Config.ID))
	if err != nil {
		s.Logger.Warnf("fail to encode encode and stream block: %s", err.Error())
		timer1.ObserveDuration()
		return
	}
	timer1.ObserveDuration()

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "publish"))
	defer timer2.ObserveDuration()
	if err := s.Pubsub.Publish(ctx, s.Config.PubsubTopic, rawBlock); err != nil {
		s.Logger.Warnf("fail to encode encode and stream block: %s", err.Error())
		return
	}
}

func (s *RedisStream) cachePayload(ctx context.Context, ds Datastore, payload *structs.BlockAndTrace) error {
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

	return ds.CacheBlock(ctx, &completeBlock)
}

func (s *RedisStream) storePayload(ctx context.Context, ds Datastore, payload *structs.BlockAndTrace) error {
	if err := ds.PutPayload(ctx, payloadToKey(payload), payload, s.Config.TTL); err != nil {
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

	return ds.PutHeader(ctx, structs.HeaderData{
		Slot:           structs.Slot(payload.Trace.Message.Slot),
		Marshaled:      b,
		HeaderAndTrace: h,
	}, s.Config.TTL)
}

func payloadToKey(payload *structs.BlockAndTrace) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: payload.Payload.Data.BlockHash,
		Proposer:  payload.Trace.Message.ProposerPubkey,
		Slot:      structs.Slot(payload.Trace.Message.Slot),
	}
}
