//go:generate mockgen  -destination=./mocks/stream.go -package=mocks github.com/blocknative/dreamboat/pkg/datastore Pubsub,RemoteDatastore
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/protobuf/proto"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"

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
	ID          string
	PubsubTopic string // pubsub topic name for block submissions
	TTL         time.Duration
	Logger      log.Logger
}

type StreamDatastore struct {
	*datastore.Datastore
	Pubsub          Pubsub
	RemoteDatastore RemoteDatastore
	Config          StreamConfig

	Logger log.Logger

	m StreamMetrics
}

func NewStreamDatastore(ds *datastore.Datastore, ps Pubsub, rds RemoteDatastore, cfg StreamConfig) *StreamDatastore {
	s := StreamDatastore{
		Datastore:       ds,
		Pubsub:          ps,
		RemoteDatastore: rds,
		Config:          cfg,
		Logger:          cfg.Logger.WithField("relay-service", "stream"),
	}

	s.initMetrics()

	return &s
}

func (s *StreamDatastore) Run(ctx context.Context, logger log.Logger) error {
	blocks, err := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic)
	if err != nil {
		return err
	}

	sBlock := StreamBlock{}
	for rawSBlock := range blocks {
		if err := proto.Unmarshal(rawSBlock, &sBlock); err != nil {
			logger.Warnf("fail to decode stream block: %s", err.Error())
		}

		if sBlock.Source == s.Config.ID {
			continue
		}

		block := toBlockAndTrace(&sBlock)
		if sBlock.IsCache {
			s.m.StreamRecvCounter.WithLabelValues("cache").Inc()
			if err := s.cachePayload(ctx, block); err != nil {
				logger.With(block).Warnf("fail to cache payload: %s", err.Error())
			}
		} else {
			s.m.StreamRecvCounter.WithLabelValues("store").Inc()
			if err := s.storePayload(ctx, block); err != nil {
				logger.With(block).Warnf("fail to store payload: %s", err.Error())
			}
		}

	}

	return ctx.Err()
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
		resp := <-responses
		if resp.block != nil && resp.err == nil {
			if resp.isLocal {
				s.m.StreamPayloadHitCounter.WithLabelValues("local").Inc()
			} else {
				s.m.StreamPayloadHitCounter.WithLabelValues("remote").Inc()
			}
			return resp.block, resp.fromCache, resp.err
		}
		if resp.err != nil {
			s.Logger.With(key).WithField("isLocal", resp.isLocal).Debugf("payload not found: %s", resp.err.Error())
		} else {
			s.Logger.With(key).WithField("isLocal", resp.isLocal).Debugf("payload not found")
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

	timer1 = prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "pub"))
	defer timer1.ObserveDuration()

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "encode"))
	block := toStreamBlock(payload, false, s.Config.ID)
	rawBlock, err := proto.Marshal(block)
	if err != nil {
		return fmt.Errorf("fail to encode encode and stream block: %w", err)
	}
	timer2.ObserveDuration()

	return s.Pubsub.Publish(ctx, s.Config.PubsubTopic, rawBlock)
}

func (s *StreamDatastore) CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("cacheBlock", "all"))
	defer timer0.ObserveDuration()

	if err := s.Datastore.CacheBlock(ctx, block); err != nil {
		return err
	}

	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("cacheBlock", "pub"))
	defer timer1.ObserveDuration()

	timer2 := prometheus.NewTimer(s.m.Timing.WithLabelValues("cacheBlock", "encode"))
	sBlock := toStreamBlock(&block.Payload, true, s.Config.ID)
	b, err := proto.Marshal(sBlock)
	if err != nil {
		return fmt.Errorf("fail to encode stream block: %w", err)
	}
	timer2.ObserveDuration()

	return s.Pubsub.Publish(ctx, s.Config.PubsubTopic, b)
}

func toStreamBlock(block *structs.BlockAndTrace, isCache bool, source string) *StreamBlock {
	var transactions [][]byte
	for _, tx := range block.Payload.Data.Transactions {
		transactions = append(transactions, tx)
	}

	return &StreamBlock{
		Source:  source,
		IsCache: isCache,
		Block: &PayloadAndTrace{
			Trace: &SignedBidTrace{
				Signature: block.Trace.Signature[:],
				Message: &BidTrace{
					Slot:                 block.Trace.Message.Slot,
					ParentHash:           block.Trace.Message.ParentHash[:],
					BlockHash:            block.Trace.Message.BlockHash[:],
					BuilderPubkey:        block.Trace.Message.BuilderPubkey[:],
					ProposerPubkey:       block.Trace.Message.ProposerPubkey[:],
					ProposerFeeRecipient: block.Trace.Message.ProposerFeeRecipient[:],
					GasLimit:             block.Trace.Message.GasLimit,
					GasUsed:              block.Trace.Message.GasUsed,
					Value:                block.Trace.Message.Value[:],
				},
			},
			Payload: &PayloadWithVersion{
				Version: string(block.Payload.Version),
				Payload: &ExecutionPayload{
					ParentHash:    block.Payload.Data.ParentHash[:],
					FeeRecipient:  block.Payload.Data.FeeRecipient[:],
					StateRoot:     block.Payload.Data.StateRoot[:],
					ReceiptsRoot:  block.Payload.Data.ReceiptsRoot[:],
					LogsBloom:     block.Payload.Data.LogsBloom[:],
					Random:        block.Payload.Data.Random[:],
					BlockNumber:   block.Payload.Data.BlockNumber,
					GasLimit:      block.Payload.Data.GasLimit,
					GasUsed:       block.Payload.Data.GasUsed,
					Timestamp:     block.Payload.Data.Timestamp,
					ExtraData:     block.Payload.Data.ExtraData,
					BaseFeePerGas: block.Payload.Data.BaseFeePerGas[:],
					BlockHash:     block.Payload.Data.BlockHash[:],
					Transactions:  transactions,
				},
			},
		},
	}
}

func toBlockAndTrace(streamBlock *StreamBlock) *structs.BlockAndTrace {
	block := structs.BlockAndTrace{
		Trace: &types.SignedBidTrace{
			Message: &types.BidTrace{},
		},
		Payload: &types.GetPayloadResponse{
			Data: &types.ExecutionPayload{},
		},
	}

	// copy trace
	copy(block.Trace.Signature[:], streamBlock.Block.Trace.Signature)
	block.Trace.Message.Slot = streamBlock.Block.Trace.Message.Slot
	copy(block.Trace.Message.ParentHash[:], streamBlock.Block.Trace.Message.ParentHash)
	copy(block.Trace.Message.BlockHash[:], streamBlock.Block.Trace.Message.BlockHash)
	copy(block.Trace.Message.BuilderPubkey[:], streamBlock.Block.Trace.Message.BuilderPubkey)
	copy(block.Trace.Message.ProposerPubkey[:], streamBlock.Block.Trace.Message.ProposerPubkey)
	copy(block.Trace.Message.ProposerFeeRecipient[:], streamBlock.Block.Trace.Message.ProposerFeeRecipient)
	block.Trace.Message.GasLimit = streamBlock.Block.Trace.Message.GasLimit
	block.Trace.Message.GasUsed = streamBlock.Block.Trace.Message.GasUsed
	copy(block.Trace.Message.Value[:], streamBlock.Block.Trace.Message.Value)

	// copy payload
	block.Payload.Version = types.VersionString(streamBlock.Block.Payload.Version)
	copy(block.Payload.Data.ParentHash[:], streamBlock.Block.Payload.Payload.ParentHash)
	copy(block.Payload.Data.FeeRecipient[:], streamBlock.Block.Payload.Payload.FeeRecipient)
	copy(block.Payload.Data.StateRoot[:], streamBlock.Block.Payload.Payload.StateRoot)
	copy(block.Payload.Data.ReceiptsRoot[:], streamBlock.Block.Payload.Payload.ReceiptsRoot)
	copy(block.Payload.Data.LogsBloom[:], streamBlock.Block.Payload.Payload.LogsBloom)
	copy(block.Payload.Data.Random[:], streamBlock.Block.Payload.Payload.Random)
	block.Payload.Data.BlockNumber = streamBlock.Block.Payload.Payload.BlockNumber
	block.Payload.Data.GasLimit = streamBlock.Block.Payload.Payload.GasLimit
	block.Payload.Data.GasUsed = streamBlock.Block.Payload.Payload.GasUsed
	block.Payload.Data.Timestamp = streamBlock.Block.Payload.Payload.Timestamp
	copy(block.Payload.Data.ExtraData[:], streamBlock.Block.Payload.Payload.ExtraData)
	copy(block.Payload.Data.BaseFeePerGas[:], streamBlock.Block.Payload.Payload.BaseFeePerGas)
	copy(block.Payload.Data.BlockHash[:], streamBlock.Block.Payload.Payload.BlockHash)
	block.Payload.Data.Transactions = make([]hexutil.Bytes, 0)
	for _, tx := range streamBlock.Block.Payload.Payload.Transactions {
		block.Payload.Data.Transactions = append(block.Payload.Data.Transactions, tx)
	}

	return &block
}

func payloadToKey(payload *structs.BlockAndTrace) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: payload.Payload.Data.BlockHash,
		Proposer:  payload.Trace.Message.ProposerPubkey,
		Slot:      structs.Slot(payload.Trace.Message.Slot),
	}
}
