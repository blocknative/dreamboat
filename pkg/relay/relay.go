//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg/relay Datastore,State,ValidatorStore,ValidatorCache,BlockValidationClient
package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"

	rpctypes "github.com/blocknative/dreamboat/pkg/client/sim/types"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/verify"
)

var (
	ErrUnknownValue            = errors.New("value is unknown")
	ErrPayloadAlreadyDelivered = errors.New("slot payload already delivered")
	ErrNoPayloadFound          = errors.New("no payload found")
	ErrMissingRequest          = errors.New("req is nil")
	ErrMissingSecretKey        = errors.New("secret key is nil")
	ErrNoBuilderBid            = errors.New("no builder bid")
	ErrOldSlot                 = errors.New("requested slot is old")
	ErrBadHeader               = errors.New("invalid block header from datastore")
	ErrInvalidSignature        = errors.New("invalid signature")
	ErrStore                   = errors.New("failed to store")
	ErrMarshal                 = errors.New("failed to marshal")
	ErrInternal                = errors.New("internal server error")
	ErrUnknownValidator        = errors.New("unknown validator")
	ErrVerification            = errors.New("failed to verify")
	ErrInvalidTimestamp        = errors.New("invalid timestamp")
	ErrInvalidSlot             = errors.New("invalid slot")
	ErrEmptyBlock              = errors.New("block is empty")
)

type BlockValidationClient interface {
	//ValidateBlock(ctx context.Context, params []byte) (rrr rpctypes.RpcRawResponse, err error)
	ValidateBlock(ctx context.Context, block *rpctypes.BuilderBlockValidationRequest) (rrr rpctypes.RpcRawResponse, err error)
}

type ValidatorStore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type ValidatorCache interface {
	Add(types.PublicKey, structs.ValidatorCacheEntry) (evicted bool)
	Get(types.PublicKey) (structs.ValidatorCacheEntry, bool)
	Remove(types.PublicKey) (existed bool)
}

type State interface {
	Beacon() *structs.BeaconState
}

type Verifier interface {
	Enqueue(ctx context.Context, sig [96]byte, pubkey [48]byte, msg [32]byte) (err error)
}

type Datastore interface {
	CheckSlotDelivered(context.Context, uint64) (bool, error)
	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, structs.PayloadQuery) (structs.BidTraceWithTimestamp, error)

	PutPayload(context.Context, structs.PayloadKey, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error)

	PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) error
	CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error
	GetMaxProfitHeader(ctx context.Context, slot uint64) (structs.HeaderAndTrace, error)

	// to be changed
	GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error)
	GetHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.HeaderAndTrace, error)
	GetHeadersByBlockNum(ctx context.Context, num uint64) ([]structs.HeaderAndTrace, error)
	GetLatestHeaders(ctx context.Context, limit uint64, stopLag uint64) ([]structs.HeaderAndTrace, error)
	GetDeliveredBatch(context.Context, []structs.PayloadQuery) ([]structs.BidTraceWithTimestamp, error)
}

type Auctioneer interface {
	AddBlock(block *structs.CompleteBlockstruct) bool
	MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool)
}

type Beacon interface {
	PublishBlock(block *types.SignedBeaconBlock) error
}

type RelayConfig struct {
	BuilderSigningDomain  types.Domain
	ProposerSigningDomain types.Domain
	PubKey                types.PublicKey
	SecretKey             *bls.SecretKey

	PublishBlock bool

	TTL time.Duration

	RegistrationCacheTTL time.Duration
}

type Relay struct {
	d Datastore

	a Auctioneer
	l log.Logger

	ver    Verifier
	config RelayConfig

	cache  ValidatorCache
	vstore ValidatorStore

	bvc BlockValidationClient

	beacon      Beacon
	beaconState State

	deliveredCache     map[uint64]struct{}
	deliveredCacheLock sync.RWMutex

	m RelayMetrics
}

// NewRelay relay service
func NewRelay(l log.Logger, config RelayConfig, beacon Beacon, cache ValidatorCache, vstore ValidatorStore, ver Verifier, beaconState State, d Datastore, a Auctioneer, bvc BlockValidationClient) *Relay {
	rs := &Relay{
		d:              d,
		a:              a,
		l:              l,
		bvc:            bvc,
		ver:            ver,
		config:         config,
		cache:          cache,
		vstore:         vstore,
		beacon:         beacon,
		beaconState:    beaconState,
		deliveredCache: make(map[uint64]struct{}),
	}
	rs.initMetrics()
	return rs
}

// GetHeader is called by a block proposer communicating through mev-boost and returns a bid along with an execution payload header
func (rs *Relay) GetHeader(ctx context.Context, m *structs.MetricGroup, request structs.HeaderRequest) (*types.GetHeaderResponse, error) {
	tStart := time.Now()
	defer m.AppendSince(tStart, "getHeader", "all")

	logger := rs.l.WithField("method", "GetHeader")

	slot, err := request.Slot()
	if err != nil {
		return nil, err
	}

	parentHash, err := request.ParentHash()
	if err != nil {
		return nil, err
	}

	pk, err := request.Pubkey()
	if err != nil {
		return nil, err
	}

	logger = logger.With(log.F{
		"slot":       slot,
		"parentHash": parentHash,
		"pubkey":     pk,
	})

	logger.Info("header requested")
	tGet := time.Now()

	maxProfitBlock, ok := rs.a.MaxProfitBlock(slot)
	if !ok {
		if slot < rs.beaconState.Beacon().HeadSlot()-1 {
			rs.m.MissHeaderCount.WithLabelValues("oldSlot").Add(1)
			return nil, ErrOldSlot
		}
		rs.m.MissHeaderCount.WithLabelValues("noSubmission").Add(1)
		return nil, ErrNoBuilderBid
	}

	m.AppendSince(tGet, "getHeader", "get")

	if err := rs.d.CacheBlock(ctx, maxProfitBlock); err != nil {
		logger.Warnf("fail to cache block: %s", err.Error())
	}
	logger.Debug("payload cached")

	header := maxProfitBlock.Header

	if header.Header == nil || (header.Header.ParentHash != parentHash) {
		logger.Debug(ErrBadHeader.Error())
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	bid := types.BuilderBid{
		Header: header.Header,
		Value:  header.Trace.Value,
		Pubkey: rs.config.PubKey,
	}

	tSignature := time.Now()
	signature, err := types.SignMessage(&bid, rs.config.BuilderSigningDomain, rs.config.SecretKey)
	m.AppendSince(tSignature, "getHeader", "signature")
	if err != nil {
		return nil, ErrInternal
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(tStart).Milliseconds(),
		"bidValue":         bid.Value.String(),
		"blockHash":        bid.Header.BlockHash.String(),
		"feeRecipient":     bid.Header.FeeRecipient.String(),
		"slot":             slot,
	}).Info("bid sent")

	return &types.GetHeaderResponse{
		Version: "bellatrix",
		Data:    &types.SignedBuilderBid{Message: &bid, Signature: signature},
	}, nil
}

// GetPayload is called by a block proposer communicating through mev-boost and reveals execution payload of given signed beacon block if stored
func (rs *Relay) GetPayload(ctx context.Context, m *structs.MetricGroup, payloadRequest *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) { // TODO(l): remove FB type
	tStart := time.Now()
	defer m.AppendSince(tStart, "getPayload", "all")

	if len(payloadRequest.Signature) != 96 {
		return nil, ErrInvalidSignature
	}

	proposerPubkey, err := rs.beaconState.Beacon().KnownValidatorByIndex(payloadRequest.Message.ProposerIndex)
	if err != nil && errors.Is(err, ErrUnknownValue) {
		return nil, fmt.Errorf("%w for index %d", ErrUnknownValidator, payloadRequest.Message.ProposerIndex)
	} else if err != nil {
		return nil, err
	}

	tVerify := time.Now()
	pk, err := types.HexToPubkey(proposerPubkey.String())
	if err != nil {
		return nil, err
	}

	logger := rs.l.With(log.F{
		"method":    "GetPayload",
		"slot":      payloadRequest.Message.Slot,
		"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		"pubkey":    pk,
	})
	logger.Info("payload requested")

	msg, err := types.ComputeSigningRoot(payloadRequest.Message, rs.config.ProposerSigningDomain)
	if err != nil {
		return nil, ErrInvalidSignature // err
	}
	ok, err := verify.VerifySignatureBytes(msg, payloadRequest.Signature[:], pk[:])
	if err != nil || !ok {
		return nil, ErrInvalidSignature
	}
	m.AppendSince(tVerify, "getPayload", "verify")

	tGet := time.Now()
	key := structs.PayloadKey{
		BlockHash: payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  pk,
		Slot:      structs.Slot(payloadRequest.Message.Slot),
	}

	payload, fromCache, err := rs.d.GetPayload(ctx, key)
	if err != nil || payload == nil {
		return nil, ErrNoPayloadFound
	}
	m.AppendSince(tGet, "getPayload", "get")

	trace := structs.DeliveredTrace{
		Trace: structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 payloadRequest.Message.Slot,
					ParentHash:           payload.Payload.Data.ParentHash,
					BlockHash:            payload.Payload.Data.BlockHash,
					BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: payload.Trace.Message.ProposerFeeRecipient,
					GasLimit:             payload.Payload.Data.GasLimit,
					GasUsed:              payload.Payload.Data.GasUsed,
					Value:                payload.Trace.Message.Value,
				},
				BlockNumber: payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(payload.Payload.Data.Transactions)),
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
		BlockNumber: payload.Payload.Data.BlockNumber,
	}

	// defer put delivered datastore write
	go func(rs *Relay, slot structs.Slot, trace structs.DeliveredTrace) {
		if rs.config.PublishBlock {
			beaconBlock := structs.SignedBlindedBeaconBlockToBeaconBlock(payloadRequest, payload.Payload.Data)
			if err := rs.beacon.PublishBlock(beaconBlock); err != nil {
				logger.WithError(err).Warn("fail to publish block to beacon node")
			} else {
				logger.Info("published block to beacon node")
			}
		}

		if err := rs.d.PutDelivered(context.Background(), slot, trace, rs.config.TTL); err != nil {
			logger.WithError(err).Warn("failed to set payload after delivery")
		}
	}(rs, structs.Slot(payloadRequest.Message.Slot), trace)

	logger.With(log.F{
		"slot":             payloadRequest.Message.Slot,
		"blockHash":        payload.Payload.Data.BlockHash,
		"blockNumber":      payload.Payload.Data.BlockNumber,
		"stateRoot":        payload.Payload.Data.StateRoot,
		"feeRecipient":     payload.Payload.Data.FeeRecipient,
		"bid":              payload.Bid.Data.Message.Value,
		"from_cache":       fromCache,
		"numTx":            len(payload.Payload.Data.Transactions),
		"processingTimeMs": time.Since(tStart).Milliseconds(),
	}).Info("payload sent")

	return &types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    payload.Payload.Data,
	}, nil
}
