//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/relay Datastore,State,ValidatorStore,ValidatorCache,BlockValidationClient,Auctioneer,Verifier,Beacon
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

	rpctypes "github.com/blocknative/dreamboat/client/sim/types"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/blocknative/dreamboat/verify"
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
	IsSet() bool
	ValidateBlock(ctx context.Context, block *rpctypes.BuilderBlockValidationRequest) (err error)
	ValidateBlockV2(ctx context.Context, block *rpctypes.BuilderBlockValidationRequest) (err error)
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
	KnownValidators() structs.ValidatorsState
	HeadSlot() structs.Slot
	Genesis() structs.GenesisInfo
	Withdrawals() structs.WithdrawalsState
	Randao() string
	ForkVersion(slot structs.Slot) structs.ForkVersion
}

type Verifier interface {
	Enqueue(ctx context.Context, sig [96]byte, pubkey [48]byte, msg [32]byte) (err error)
}

type Datastore interface {
	CheckSlotDelivered(context.Context, uint64) (bool, error)
	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, structs.PayloadQuery) (structs.BidTraceWithTimestamp, error)

	PutPayload(context.Context, structs.PayloadKey, structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.ForkVersion, structs.PayloadKey) (structs.BlockBidAndTrace, bool, error)

	PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) error
	CacheBlock(ctx context.Context, key structs.PayloadKey, block *structs.CompleteBlockstruct) error
	GetMaxProfitHeader(ctx context.Context, fork structs.ForkVersion, slot uint64) (structs.HeaderAndTrace, error)

	// to be changed
	GetHeadersBySlot(ctx context.Context, fork structs.ForkVersion, slot uint64) ([]structs.HeaderAndTrace, error)
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
	PublishBlock(block structs.SignedBeaconBlock) error
}

type RelayConfig struct {
	BuilderSigningDomain  types.Domain
	ProposerSigningDomain map[structs.ForkVersion]types.Domain
	PubKey                types.PublicKey
	SecretKey             *bls.SecretKey

	AllowedListedBuilders map[[48]byte]struct{}

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
func (rs *Relay) GetHeader(ctx context.Context, m *structs.MetricGroup, request structs.HeaderRequest) (structs.GetHeaderResponse, error) {

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
		if slot < rs.beaconState.HeadSlot()-1 {
			rs.m.MissHeaderCount.WithLabelValues("oldSlot").Add(1)
			return nil, ErrOldSlot
		}
		rs.m.MissHeaderCount.WithLabelValues("noSubmission").Add(1)
		return nil, ErrNoBuilderBid
	}

	m.AppendSince(tGet, "getHeader", "get")

	if err := rs.d.CacheBlock(ctx, structs.PayloadKey{
		BlockHash: maxProfitBlock.Header.Trace().BlockHash,
		Slot:      structs.Slot(maxProfitBlock.Header.Trace().Slot),
		Proposer:  maxProfitBlock.Header.Trace().ProposerPubkey}, maxProfitBlock); err != nil {
		logger.Warnf("fail to cache block: %s", err.Error())
	}
	logger.Debug("payload cached")

	header := maxProfitBlock.Header

	if header.Header == nil {
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	if header.Header().GetParentHash() != parentHash {
		logger.WithField("expected", parentHash).WithField("got", parentHash).Debug("invalid parentHash")
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	if header.Trace().ProposerPubkey != pk.PublicKey {
		logger.WithField("expected", header.Trace().BuilderPubkey).WithField("got", pk.PublicKey).Debug("invalid pubkey")
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	fork := rs.beaconState.ForkVersion(slot)
	if fork == structs.ForkBellatrix {
		h, ok := header.Header().(*bellatrix.ExecutionPayloadHeader)
		if !ok {
			return nil, errors.New("incompatible fork state")
		}
		bid := &bellatrix.BuilderBid{
			BellatrixHeader: h,
			BellatrixValue:  header.Trace().Value,
			BellatrixPubkey: rs.config.PubKey,
		}
		tSignature := time.Now()
		signature, err := types.SignMessage(bid, rs.config.BuilderSigningDomain, rs.config.SecretKey)
		m.AppendSince(tSignature, "getHeader", "signature")
		if err != nil {
			return nil, ErrInternal
		}

		value := header.Trace().Value
		logger.With(log.F{
			"processingTimeMs": time.Since(tStart).Milliseconds(),
			"bidValue":         value.String(),
			"blockHash":        bid.BellatrixHeader.BlockHash.String(),
			"feeRecipient":     bid.BellatrixHeader.FeeRecipient.String(),
			"slot":             slot,
		}).Info("bid sent")

		return &bellatrix.GetHeaderResponse{
			BellatrixVersion: types.VersionString("bellatrix"),
			BellatrixData: bellatrix.SignedBuilderBid{
				BellatrixMessage:   bid,
				BellatrixSignature: signature},
		}, nil
	} else if fork == structs.ForkCapella {
		h, ok := header.Header().(*capella.ExecutionPayloadHeader)
		if !ok {
			return nil, errors.New("incompatible fork state")
		}
		bid := capella.BuilderBid{
			CapellaHeader: h,
			CapellaValue:  header.Trace().Value,
			CapellaPubkey: rs.config.PubKey,
		}
		tSignature := time.Now()
		signature, err := types.SignMessage(&bid, rs.config.BuilderSigningDomain, rs.config.SecretKey)
		m.AppendSince(tSignature, "getHeader", "signature")
		if err != nil {
			return nil, ErrInternal
		}

		value := header.Trace().Value
		logger.With(log.F{
			"processingTimeMs": time.Since(tStart).Milliseconds(),
			"bidValue":         value.String(),
			"blockHash":        bid.CapellaHeader.BlockHash.String(),
			"feeRecipient":     bid.CapellaHeader.FeeRecipient.String(),
			"slot":             slot,
		}).Info("bid sent")
		return &capella.GetHeaderResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData: capella.SignedBuilderBid{
				CapellaMessage:   bid,
				CapellaSignature: signature},
		}, nil
	} else {
		return nil, errors.New("incompatible fork state")
	}

}

// GetPayload is called by a block proposer communicating through mev-boost and reveals execution payload of given signed beacon block if stored
func (rs *Relay) GetPayload(ctx context.Context, m *structs.MetricGroup, payloadRequest structs.SignedBlindedBeaconBlock) (structs.GetPayloadResponse, error) {

	tStart := time.Now()
	defer m.AppendSince(tStart, "getPayload", "all")

	if len(payloadRequest.Signature()) != 96 {
		return nil, ErrInvalidSignature
	}

	proposerPubkey, ok := rs.beaconState.KnownValidators().KnownValidatorsByIndex[payloadRequest.ProposerIndex()]
	if !ok {
		return nil, fmt.Errorf("%w for index %d", ErrUnknownValidator, payloadRequest.ProposerIndex())
	}

	tVerify := time.Now()
	pk, err := types.HexToPubkey(proposerPubkey.String())
	if err != nil {
		return nil, err
	}

	logger := rs.l.With(log.F{
		"method":    "GetPayload",
		"slot":      payloadRequest.Slot(),
		"blockHash": payloadRequest.BlockHash(),
		"pubkey":    pk,
	})
	logger.Info("payload requested")

	forkv := rs.beaconState.ForkVersion(structs.Slot(payloadRequest.Slot()))

	msg, err := payloadRequest.ComputeSigningRoot(rs.config.ProposerSigningDomain[forkv])
	if err != nil {
		return nil, ErrInvalidSignature // err
	}

	sig := payloadRequest.Signature()
	ok, err = verify.VerifySignatureBytes(msg, sig[:], pk[:])
	if err != nil || !ok {
		return nil, ErrInvalidSignature
	}
	m.AppendSince(tVerify, "getPayload", "verify")

	tGet := time.Now()

	key, err := payloadRequest.ToPayloadKey(pk)
	if err != nil {
		logger.WithError(err).Warn("error getting payload")
		return nil, ErrNoPayloadFound
	}

	payload, fromCache, err := rs.d.GetPayload(ctx, forkv, key)
	if err != nil || payload == nil {
		logger.WithError(err).Warn("error getting payload")
		return nil, ErrNoPayloadFound
	}
	m.AppendSince(tGet, "getPayload", "get")

	// defer put delivered datastore write
	go func(rs *Relay, slot structs.Slot, payloadRequest structs.SignedBlindedBeaconBlock) {
		if rs.config.PublishBlock {
			beaconBlock, err := payloadRequest.ToBeaconBlock(payload.ExecutionPayload())
			if err != nil {
				logger.WithError(err).Warn("fail to create block for publication")
			} else {
				if err = rs.beacon.PublishBlock(beaconBlock); err != nil {
					logger.With(log.F{
						"slot":         slot,
						"block_number": payloadRequest.BlockNumber(),
					}).WithError(err).Warn("fail to publish block to beacon node")
				} else {
					logger.Info("published block to beacon node")
				}
			}
		}

		trace, err := payload.ToDeliveredTrace(payloadRequest.Slot())
		if err != nil {
			logger.WithError(err).Warn("failed to generate delivered payload")
		}
		if err := rs.d.PutDelivered(context.Background(), slot, trace, rs.config.TTL); err != nil {
			logger.WithError(err).Warn("failed to set payload after delivery")
		}
	}(rs, structs.Slot(payloadRequest.Slot()), payloadRequest)

	exp := payload.ExecutionPayload()

	logger = logger.With(log.F{
		"slot":             payloadRequest.Slot(),
		"from_cache":       fromCache,
		"processingTimeMs": time.Since(tStart).Milliseconds(),
	})
	switch forkv {
	case structs.ForkBellatrix:
		bep := exp.(*bellatrix.ExecutionPayload)
		logger.With(log.F{
			"fork":         "bellatrix",
			"blockHash":    bep.EpBlockHash,
			"blockNumber":  bep.EpBlockNumber,
			"stateRoot":    bep.EpStateRoot,
			"feeRecipient": bep.EpFeeRecipient,
			"numTx":        len(bep.EpTransactions),
			"bid":          payload.BidValue(),
		}).Info("payload sent")
		return &bellatrix.GetPayloadResponse{
			BellatrixVersion: types.VersionString("bellatrix"),
			BellatrixData:    *bep,
		}, nil
	case structs.ForkCapella:
		cep := exp.(*capella.ExecutionPayload)
		logger.With(log.F{
			"fork":         "capella",
			"blockHash":    cep.EpBlockHash,
			"blockNumber":  cep.EpBlockNumber,
			"stateRoot":    cep.EpStateRoot,
			"feeRecipient": cep.EpFeeRecipient,
			"numTx":        len(cep.EpTransactions),
			"bid":          payload.BidValue(),
		}).Info("payload sent")
		return &capella.GetPayloadResponse{
			CapellaVersion: types.VersionString("capella"),
			CapellaData:    *cep,
		}, nil
	}
	return nil, errors.New("unknown fork")

}
