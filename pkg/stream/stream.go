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
	PubsubTopic string // pubsub topic name for block submissions
	TTL         time.Duration
}

type StreamDatastore struct {
	*datastore.Datastore
	Pubsub          Pubsub
	RemoteDatastore RemoteDatastore
	ID              string // TODO: must be unique for every relay

	Config StreamConfig
}

func (s *StreamDatastore) Run(ctx context.Context, logger log.Logger) error {
	logger = logger.WithField("relay-service", "stream")

	blocks, err := s.Pubsub.Subscribe(ctx, s.Config.PubsubTopic)
	if err != nil {
		return err
	}

	sBlock := StreamBlock{}
	for rawSBlock := range blocks {
		if err := proto.Unmarshal(rawSBlock, &sBlock); err != nil {
			logger.Warnf("fail to decode stream block: %s", err.Error())
		}

		if sBlock.Source == s.ID {
			continue
		}

		block := toBlockAndTrace(&sBlock)
		if sBlock.IsCache {
			if err := s.cachePayload(ctx, block); err != nil {
				logger.With(block).Warnf("fail to cache payload: %s", err.Error())
			}
		} else {
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

func (s *StreamDatastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockAndTrace, bool, error) {
	block, from_cache, err := s.Datastore.GetPayload(ctx, key)
	if err != nil || block == nil {
		block, err = s.RemoteDatastore.GetPayload(ctx, key)
		return block, false, err
	}

	return block, from_cache, err
}

func (s *StreamDatastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload *structs.BlockAndTrace, ttl time.Duration) error {
	if err := s.RemoteDatastore.PutPayload(ctx, key, payload, ttl); err != nil {
		return err
	}

	if err := s.Datastore.PutPayload(ctx, key, payload, ttl); err != nil {
		return err
	}

	block := toStreamBlock(payload, false, s.ID)
	rawBlock, err := proto.Marshal(block)
	if err != nil {
		return fmt.Errorf("fail to encode encode and stream block: %w", err)
	}

	if err := s.Pubsub.Publish(ctx, s.Config.PubsubTopic, rawBlock); err != nil {
		return fmt.Errorf("fail to stream payload: %w", err)
	}

	return nil
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
