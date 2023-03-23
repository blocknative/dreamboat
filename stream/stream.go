//go:generate mockgen  -destination=./mocks/stream.go -package=mocks github.com/blocknative/dreamboat/pkg/stream Pubsub,Datastore

package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blocknative/dreamboat/structs"
)

type Pubsub interface {
	Publish(context.Context, string, []byte) error
	Subscribe(context.Context, string) chan []byte
}

type Datastore interface {
	PutPayload(context.Context, structs.PayloadKey, structs.BlockBidAndTrace, time.Duration) error
	CacheBlock(context.Context, structs.PayloadKey, *structs.CompleteBlockstruct) error
}

type StreamConfig struct {
	Logger          log.Logger
	ID              string
	TTL             time.Duration
	PubsubTopic     string // pubsub topic name for block submissions
	StreamQueueSize int
}

type State interface {
	ForkVersion(epoch structs.Slot) structs.ForkVersion
	HeadSlot() structs.Slot
}

type StreamBlock interface {
	Block() structs.BlockBidAndTrace
	CompleteBlock() (structs.CompleteBlockstruct, error)
	IsCache() bool
	Source() string

	Loggable() map[string]any
}

type Client struct {
	Pubsub Pubsub

	cacheRequests         chan structs.BlockBidAndTrace
	storeRequests         chan structs.BlockBidAndTrace
	slotDeliveredRequests chan structs.Slot

	Config StreamConfig
	Logger log.Logger

	m StreamMetrics

	slotDelivered chan structs.Slot

	st State
}

func NewClient(ps Pubsub, cfg StreamConfig) *Client {
	s := Client{
		Pubsub:                ps,
		cacheRequests:         make(chan structs.BlockBidAndTrace, cfg.StreamQueueSize),
		storeRequests:         make(chan structs.BlockBidAndTrace, cfg.StreamQueueSize),
		slotDeliveredRequests: make(chan structs.Slot, cfg.StreamQueueSize),
		slotDelivered:         make(chan structs.Slot, cfg.StreamQueueSize),
		Config:                cfg,
		Logger:                cfg.Logger.WithField("relay-service", "stream").WithField("type", "redis"),
	}

	s.initMetrics()

	return &s
}

func (s *Client) RunSubscriberParallel(ctx context.Context, ds Datastore, num uint) error {
	blocks := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic+"/blocks")
	delivered := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic+"/delivered")

	for i := uint(0); i < num; i++ {
		go s.RunBlockSubscriber(ctx, ds, blocks)
	}

	go s.RunSlotDeliveredSubscriber(ctx, delivered)

	return nil
}

func (s *Client) RunBlockSubscriber(ctx context.Context, ds Datastore, blocks chan []byte) error {
	for rawSBlock := range blocks {
		sBlock, err := s.Unmarshal(rawSBlock)
		if err != nil {
			s.Logger.Warnf("fail to decode stream block: %s", err.Error())
		}

		if sBlock.Source() == s.Config.ID {
			continue
		}

		if sBlock.IsCache() {
			s.m.StreamRecvCounter.WithLabelValues("cache").Inc()
			if err := s.cachePayload(ctx, ds, sBlock); err != nil {
				s.Logger.WithError(err).With(sBlock).Warn("failed to cache payload")
			}
		} else {
			s.m.StreamRecvCounter.WithLabelValues("store").Inc()
			if err := s.storePayload(ctx, ds, sBlock); err != nil {
				s.Logger.WithError(err).With(sBlock).Warn("failed to store payload: %s")
			}
		}
	}

	return ctx.Err()
}

func (s *Client) RunSlotDeliveredSubscriber(ctx context.Context, slots chan []byte) error {
	slotDelivered := SlotDelivered{}

	for rawSlot := range slots {
		if err := json.Unmarshal(rawSlot, &slotDelivered); err != nil {
			s.Logger.Warnf("fail to decode stream slot delivered: %s", err.Error())
		}

		select {
		case s.slotDelivered <- structs.Slot(slotDelivered.Slot):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return ctx.Err()
}

func (s *Client) RunPublisherParallel(ctx context.Context, num uint) {
	for i := uint(0); i < num; i++ {
		go s.RunPublisher(ctx)
	}
}

func (s *Client) RunPublisher(ctx context.Context) error {
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

func (s *Client) PublishBlockSubmission(ctx context.Context, block structs.BlockBidAndTrace) error {
	select {
	case s.storeRequests <- block:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Client) PublishCacheBlock(ctx context.Context, block structs.BlockBidAndTrace) error {
	select {
	case s.cacheRequests <- block:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Client) PublishSlotDelivered(ctx context.Context, slot structs.Slot) error {
	select {
	case s.slotDeliveredRequests <- slot:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Client) SlotDeliveredChan() <-chan structs.Slot {
	return s.slotDelivered
}

func (s *Client) encodeAndPublish(ctx context.Context, block structs.BlockBidAndTrace, isCache bool) {
	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "encode"))
	gBlock := GenericStreamBlock{BlockBidAndTrace: block, IsBlockCache: isCache, StreamSource: s.Config.ID}
	rawBlock, err := json.Marshal(gBlock)
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

func (s *Client) cachePayload(ctx context.Context, ds Datastore, sBlock StreamBlock) error {
	cbs, err := sBlock.CompleteBlock()
	if err != nil {
		return fmt.Errorf("failed to generate CompleteBlock from StreamBlock: %w", err)
	}
	return ds.CacheBlock(ctx, payloadToKey(sBlock.Block()), &cbs)
}

func (s *Client) storePayload(ctx context.Context, ds Datastore, sBlock StreamBlock) error {
	return ds.PutPayload(ctx, payloadToKey(sBlock.Block()), sBlock.Block(), s.Config.TTL)
}

func payloadToKey(bbt structs.BlockBidAndTrace) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: bbt.ExecutionPayload().BlockHash(),
		Proposer:  bbt.Proposer(),
		Slot:      structs.Slot(bbt.Slot()),
	}
}

func (s *Client) Unmarshal(b []byte) (StreamBlock, error) {
	fork := s.st.ForkVersion(s.st.HeadSlot())
	switch fork {
	case structs.ForkCapella:
		var creq CapellaStreamBlock
		if err := json.Unmarshal(b, &creq); err != nil {
			return nil, fmt.Errorf("failed to unmarshal capella block: %w", err)
		}
		return &creq, nil
	case structs.ForkBellatrix:
		var breq BellatrixStreamBlock
		if err := json.Unmarshal(b, &breq); err != nil {
			return nil, fmt.Errorf("failed to unmarshal capella block: %w", err)
		}
		return &breq, nil
	default:
		return nil, fmt.Errorf("invalid fork version: %d", fork)
	}
}
