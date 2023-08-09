//go:generate mockgen  -destination=./mocks/stream.go -package=mocks github.com/blocknative/dreamboat/stream Pubsub

package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/metrics"
	"github.com/blocknative/dreamboat/stream/transport"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
)

const (
	CacheTopic         = "/block/cache"
	BidTopic           = "/block/bid"
	SlotDeliveredTopic = "/slot/delivered"
)

type PubSub interface {
	String() string
	Publish(context.Context, transport.Message) error
	Subscribe(context.Context) transport.Subscription
}

type State interface {
	ForkVersion(epoch structs.Slot) structs.ForkVersion
	HeadSlot() structs.Slot
}

type Metrics interface {
	Register(prometheus.Collector) error
}

type Client struct {
	Logger                log.Logger
	Bids, Cache           PubSub
	QueueSize, NumWorkers int

	Metrics Metrics
	m       streamMetrics

	cancel context.CancelFunc

	builderBidOut chan structs.BuilderBidExtended
	cacheOut      chan structs.BlockAndTraceExtended

	st State
}

func NewClient(l log.Logger, m *metrics.Metrics, s State, bids, cache PubSub, cfg *config.DistributedConfig) *Client {
	c := &Client{
		Logger: l.
			WithField("subService", "stream").
			WithField("type", "redis"),
		Bids:       bids,
		Cache:      cache,
		QueueSize:  cfg.StreamQueueSize,
		NumWorkers: cfg.WorkerNumber,
		Metrics:    m,
		st:         s,
	}

	c.initMetrics()

	c.builderBidOut = make(chan structs.BuilderBidExtended, c.QueueSize)
	c.cacheOut = make(chan structs.BlockAndTraceExtended, c.QueueSize)

	var ctx context.Context
	ctx, c.cancel = context.WithCancel(context.Background())
	go subscribe(ctx, c.Logger, c.Bids, c.handleBid)
	go subscribe(ctx, c.Logger, c.Cache, c.handleCache)

	return c
}

func (c *Client) Close() error {
	c.cancel()
	return nil
}

func (s *Client) BlockCache() <-chan structs.BlockAndTraceExtended {
	return s.cacheOut
}

func (c *Client) handleCache(ctx context.Context, msg transport.Message) error {
	var bbt structs.BlockAndTraceExtended

	switch forkEncoding := msg.Encoding; forkEncoding {
	case transport.BellatrixJson:
		var bbbt bellatrix.BlockBidAndTrace
		if err := json.Unmarshal(msg.Payload, &bbbt); err != nil {
			return err
		}
		bbt = &bbbt
	case transport.CapellaJson:
		var cbbt capella.BlockAndTraceExtended
		if err := json.Unmarshal(msg.Payload, &cbbt); err != nil {
			return err
		}
		bbt = &cbbt
	case transport.CapellaSSZ:
		var cbbt capella.BlockAndTraceExtended
		if err := cbbt.UnmarshalSSZ(msg.Payload); err != nil {
			return err
		}
		bbt = &cbbt
	default:
		return fmt.Errorf("unknown fork encoding: %d", forkEncoding)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case c.cacheOut <- bbt:
		c.m.RecvCounter.WithLabelValues("cache").Inc()
		return nil
	}
}

func (s *Client) BuilderBid() <-chan structs.BuilderBidExtended {
	return s.builderBidOut
}

func subscribe(ctx context.Context, log log.Logger, ps PubSub, handle func(context.Context, transport.Message) error) {
	s := ps.Subscribe(ctx)
	defer s.Close()

	log = log.WithField("topic", ps.String())

	for {
		msg, err := s.Next(ctx)
		if err != nil {
			return // subscription only returns fatal errors
		}

		if err = handle(ctx, msg); err != nil {
			if err == context.Canceled {
				return
			}

			log.With(msg).
				WithError(err).
				Warn("failed to handle subscription event")
			continue
		}

		log.With(msg).Debug("handled subscription event")
	}
}

func (c *Client) handleBid(ctx context.Context, msg transport.Message) error {
	var bb structs.BuilderBidExtended

	switch forkEncoding := msg.Encoding; forkEncoding {
	case transport.BellatrixJson:
		var bbb bellatrix.BuilderBidExtended
		if err := json.Unmarshal(msg.Payload, &bbb); err != nil {
			return err
		}
		bb = &bbb
	case transport.CapellaJson:
		var cbb capella.BuilderBidExtended
		if err := json.Unmarshal(msg.Payload, &cbb); err != nil {
			return err
		}
		bb = &cbb
	case transport.CapellaSSZ:
		var cbb capella.BuilderBidExtended
		if err := cbb.UnmarshalSSZ(msg.Payload); err != nil {
			return err
		}
		bb = &cbb
	default:
		return fmt.Errorf("unknown fork encoding: %d", forkEncoding)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case c.builderBidOut <- bb:
		c.m.RecvCounter.WithLabelValues("bid").Inc()
		return nil
	}
}

func (s *Client) PublishBuilderBid(ctx context.Context, bid structs.BuilderBidExtended) error {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("publishBuilderBid", "all"))

	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("publishBuilderBid", "encode"))
	forkEncoding := toBidFormat(s.st.ForkVersion(structs.Slot(bid.Slot())))
	msg, err := s.encode(bid, forkEncoding)
	if err != nil {
		timer1.ObserveDuration()
		return fmt.Errorf("fail to encode encode and stream block: %w", err)
	}
	timer1.ObserveDuration()

	l := s.Logger.With(msg).With(log.F{
		"method":    "publishBuilderBid",
		"itemType":  "builderBid",
		"blockHash": bid.BuilderBid().Header().GetBlockHash(),
	})

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("publishBuilderBid", "publish"))
	if err := s.Bids.Publish(ctx, msg); err != nil {
		return fmt.Errorf("fail to encode encode and stream block: %w", err)
	}
	timer2.ObserveDuration()
	l.WithField("timestamp", time.Now()).
		Debug("published")

	// s.m.PublishSize.WithLabelValues("publishBuilderBid").Observe(float64(len(b)))
	// s.m.PublishCounter.WithLabelValues("publishBuilderBid").Add(float64(len(b)))

	timer0.ObserveDuration()
	return nil
}

func (s *Client) PublishBlockCache(ctx context.Context, block structs.BlockAndTraceExtended) error {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("publishCacheBlock", "all"))

	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("publishCacheBlock", "encode"))
	forkEncoding := toBlockCacheFormat(s.st.ForkVersion(structs.Slot(block.Slot())))
	msg, err := s.encode(block, forkEncoding)
	if err != nil {
		timer1.ObserveDuration()
		return fmt.Errorf("fail to encode cache block: %w", err)
	}
	timer1.ObserveDuration()

	l := s.Logger.With(msg).With(log.F{
		"method":    "publishBlockCache",
		"itemType":  "blockCache",
		"blockHash": block.ExecutionPayload().BlockHash(),
	})

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("publishCacheBlock", "publish"))
	if err := s.Cache.Publish(ctx, msg); err != nil {
		return fmt.Errorf("fail to publish cache block: %w", err)
	}
	timer2.ObserveDuration()
	l.WithField("timestamp", time.Now()).
		Debug("published")

	// s.m.PublishSize.WithLabelValues("publishBlockCache").Observe(float64(len(b)))
	// s.m.PublishCounter.WithLabelValues("publishBlockCache").Add(float64(len(b)))

	timer0.ObserveDuration()
	return nil
}

func (s *Client) PublishSlotDelivered(ctx context.Context, slot structs.Slot) error {
	return nil // TODO
}

func (s *Client) encode(data any, e transport.Encoding) (m transport.Message, err error) {
	switch m.Encoding = e; e { // NOTE:  we set the message's encoding field
	case transport.CapellaSSZ:
		if enc, ok := data.(EncoderSSZ); ok {
			m.Payload, err = enc.MarshalSSZ()
			break
		}
		err = errors.New("unable to cast to SSZ encoder")

	default:
		m.Payload, err = json.Marshal(data)
	}

	return
}

func toBidFormat(fork structs.ForkVersion) transport.Encoding {
	switch fork {
	case structs.ForkAltair:
		return transport.AltairJson
	case structs.ForkBellatrix:
		return transport.BellatrixJson
	case structs.ForkCapella:
		return transport.CapellaSSZ
	default:
		return transport.Unknown
	}
}

func toBlockCacheFormat(fork structs.ForkVersion) transport.Encoding {
	switch fork {
	case structs.ForkAltair:
		return transport.AltairJson
	case structs.ForkBellatrix:
		return transport.BellatrixJson
	case structs.ForkCapella:
		return transport.CapellaSSZ
	default:
		return transport.Unknown
	}
}

type EncoderSSZ interface {
	MarshalSSZ() ([]byte, error)
}
