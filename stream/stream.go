//go:generate mockgen  -destination=./mocks/stream.go -package=mocks github.com/blocknative/dreamboat/pkg/stream Pubsub,Datastore

package stream

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blocknative/dreamboat/structs"
)

var (
	blockSuffix     = "/blocks"
	deliveredSuffix = "/delivered"
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
	blocks := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic+blockSuffix)
	delivered := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic+deliveredSuffix)

	for i := uint(0); i < num; i++ {
		go s.RunBlockSubscriber(ctx, ds, blocks)
	}

	go s.RunSlotDeliveredSubscriber(ctx, delivered)

	return nil
}

func (s *Client) RunBlockSubscriber(ctx context.Context, ds Datastore, blocks chan []byte) error {
	for rawSBlock := range blocks {
		sBlock, err := s.decode(rawSBlock)
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

func (s *Client) PublishBlockSubmission(ctx context.Context, block structs.BlockBidAndTrace) error {
	return s.encodeAndPublish(ctx, block, false)
}

func (s *Client) PublishCacheBlock(ctx context.Context, block structs.BlockBidAndTrace) error {
	return s.encodeAndPublish(ctx, block, true)
}

func (s *Client) PublishSlotDelivered(ctx context.Context, slot structs.Slot) error {
	return nil // TODO
}

func (s *Client) SlotDeliveredChan() <-chan structs.Slot {
	return s.slotDelivered
}

func (s *Client) encodeAndPublish(ctx context.Context, block structs.BlockBidAndTrace, isCache bool) error {
	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "encode"))
	b, err := s.encode(block, isCache)
	if err != nil {
		timer1.ObserveDuration()
		return fmt.Errorf("fail to encode encode and stream block: %w", err)
	}
	timer1.ObserveDuration()

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("encodeAndPublish", "publish"))
	defer timer2.ObserveDuration()

	if err := s.Pubsub.Publish(ctx, s.Config.PubsubTopic+blockSuffix, b); err != nil {
		return fmt.Errorf("fail to encode encode and stream block: %w", err)
	}

	return nil
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

func (s *Client) encode(block structs.BlockBidAndTrace, isCache bool) ([]byte, error) {
	gBlock := GenericStreamBlock{BlockBidAndTrace: block, IsBlockCache: isCache, StreamSource: s.Config.ID}
	rawBlock, err := json.Marshal(gBlock)
	if err != nil {
		return nil, err
	}

	// encode the varint with a variable size
	forkFormat := toJsonFormat(s.st.ForkVersion(structs.Slot(block.Slot())))
	varintBytes := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(varintBytes, uint64(forkFormat))
	varintBytes = varintBytes[:n]

	// append the varint
	return append(varintBytes, rawBlock...), nil
}

func (s *Client) decode(b []byte) (StreamBlock, error) {
	varint, n := binary.Uvarint(b)
	if n <= 0 {
		return nil, ErrDecodeVarint
	}

	b = b[n:]
	forkFormat := ForkVersionFormat(varint)

	switch forkFormat {
	case CapellaJson:
		var creq CapellaStreamBlock
		if err := json.Unmarshal(b, &creq); err != nil {
			return nil, fmt.Errorf("failed to unmarshal capella block: %w", err)
		}
		return &creq, nil
	case BellatrixJson:
		var breq BellatrixStreamBlock
		if err := json.Unmarshal(b, &breq); err != nil {
			return nil, fmt.Errorf("failed to unmarshal capella block: %w", err)
		}
		return &breq, nil
	default:
		return nil, fmt.Errorf("invalid fork version format: %d", forkFormat)
	}
}

type ForkVersionFormat uint64

const (
	Unknown ForkVersionFormat = iota
	AltairJson
	BellatrixJson
	CapellaJson
)

var (
	ErrDecodeVarint = errors.New("error decoding varint value")
)

func toJsonFormat(fork structs.ForkVersion) ForkVersionFormat {
	switch fork {
	case structs.ForkAltair:
		return AltairJson
	case structs.ForkBellatrix:
		return BellatrixJson
	case structs.ForkCapella:
		return CapellaJson
	}
	return Unknown
}
