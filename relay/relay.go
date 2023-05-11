//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/relay DataAPIStore,Datastore,State,ValidatorStore,ValidatorCache,BlockValidationClient,Auctioneer,Verifier,Beacon
package relay

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/beacon"
	rpctypes "github.com/blocknative/dreamboat/client/sim/types"
	wh "github.com/blocknative/dreamboat/datastore/warehouse"
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
	ErrTraceMismatch           = errors.New("trace and payload mismatch")
	ErrZeroBid                 = errors.New("zero valued builder bid")
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
	ErrWrongPayload            = errors.New("wrong publish payload")
	ErrFailedToPublish         = errors.New("failed to publish block")
	ErrLateRequest             = errors.New("request too late")
	ErrInvalidExecutionPayload = errors.New("invalid execution payload")
)

type BlockValidationClient interface {
	IsSet() bool
	ValidateBlock(ctx context.Context, block *rpctypes.BuilderBlockValidationRequest) (err error)
	ValidateBlockV2(ctx context.Context, block *rpctypes.BuilderBlockValidationRequestV2) (err error)
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
	Withdrawals(uint64) structs.WithdrawalsState
	Randao(uint64) structs.RandaoState
	ForkVersion(slot structs.Slot) structs.ForkVersion
}

type Verifier interface {
	Enqueue(ctx context.Context, sig [96]byte, pubkey [48]byte, msg [32]byte) (err error)
}

type DataAPIStore interface {
	//CheckSlotDelivered(context.Context, uint64) (bool, error)

	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error
	GetDeliveredPayloads(ctx context.Context, headSlot uint64, queryArgs structs.PayloadTraceQuery) (bts []structs.BidTraceExtended, err error)

	PutBuilderBlockSubmission(ctx context.Context, bid structs.BidTraceWithTimestamp, isMostProfitable bool) (err error)
	GetBuilderBlockSubmissions(ctx context.Context, headSlot uint64, payload structs.SubmissionTraceQuery) ([]structs.BidTraceWithTimestamp, error)
}

type Datastore interface {
	CacheBlock(ctx context.Context, key structs.PayloadKey, block *structs.CompleteBlockstruct) error

	PutPayload(context.Context, structs.PayloadKey, structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.ForkVersion, structs.PayloadKey) (structs.BlockBidAndTrace, bool, error)
}

type Streamer interface {
	PublishBlockSubmission(context.Context, structs.BlockBidAndTrace) error
	PublishCacheBlock(context.Context, structs.BlockBidAndTrace) error
	PublishSlotDelivered(context.Context, structs.Slot) error
	SlotDeliveredChan() <-chan structs.Slot
}

type Auctioneer interface {
	AddBlock(block *structs.CompleteBlockstruct) bool
	MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool)
}

type Beacon interface {
	PublishBlock(ctx context.Context, block structs.SignedBeaconBlock) error
}

type Warehouse interface {
	StoreAsync(ctx context.Context, req wh.StoreRequest) error
}

type RelayConfig struct {
	BuilderSigningDomain       types.Domain
	ProposerSigningDomain      map[structs.ForkVersion]types.Domain
	PubKey                     types.PublicKey
	SecretKey                  *bls.SecretKey
	GetPayloadResponseDelay    time.Duration
	GetPayloadRequestTimeLimit time.Duration

	AllowedListedBuilders map[[48]byte]struct{}

	PublishBlock bool

	TTL time.Duration

	RegistrationCacheTTL time.Duration

	Distributed, StreamSubmissions bool
}

type Relay struct {
	d   Datastore
	das DataAPIStore

	a Auctioneer
	l log.Logger

	ver    Verifier
	config RelayConfig

	cache  ValidatorCache
	vstore ValidatorStore

	s  Streamer
	sc *lru.Cache[structs.PayloadKey, struct{}]

	bvc BlockValidationClient

	beacon      Beacon
	beaconState State

	wh Warehouse

	lastDeliveredSlot *atomic.Uint64

	m RelayMetrics

	runnignAsyncs *structs.TimeoutWaitGroup
}

// NewRelay relay service
func NewRelay(l log.Logger, config RelayConfig, beacon Beacon, cache ValidatorCache, vstore ValidatorStore, ver Verifier, beaconState State, d Datastore, das DataAPIStore, a Auctioneer, bvc BlockValidationClient, wh Warehouse, s Streamer, sc *lru.Cache[structs.PayloadKey, struct{}]) *Relay {
	rs := &Relay{
		d:                 d,
		das:               das,
		a:                 a,
		l:                 l,
		bvc:               bvc,
		ver:               ver,
		s:                 s,
		sc:                sc,
		config:            config,
		cache:             cache,
		vstore:            vstore,
		beacon:            beacon,
		wh:                wh,
		beaconState:       beaconState,
		lastDeliveredSlot: &atomic.Uint64{},
		runnignAsyncs:     structs.NewTimeoutWaitGroup(),
	}
	rs.initMetrics()
	return rs
}

func (rs *Relay) RunSlotDeliveredUpdater(ctx context.Context) error {
	for {
		select {
		case slot := <-rs.s.SlotDeliveredChan():
			rs.lastDeliveredSlot.Store(uint64(slot))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (rs *Relay) Close(ctx context.Context) {
	rs.l.Info("Awaiting relay processes to finish")
	select {
	case <-rs.runnignAsyncs.C():
		rs.l.Info("Relay processes finished")
	case <-ctx.Done():
	}
}

// GetHeader is called by a block proposer communicating through mev-boost and returns a bid along with an execution payload header
func (rs *Relay) GetHeader(ctx context.Context, m *structs.MetricGroup, uc structs.UserContent, request structs.HeaderRequest) (structs.GetHeaderResponse, error) {

	tStart := time.Now()
	defer m.AppendSince(tStart, "getHeader", "all")

	slot, err := request.Slot()
	if err != nil {
		return nil, err
	}

	if slot < (rs.beaconState.HeadSlot()+1)-(beacon.NumberOfSlotsInState-1) {
		rs.m.MissHeaderCount.WithLabelValues("oldSlot").Add(1)
		return nil, ErrOldSlot
	}

	parentHash, err := request.ParentHash()
	if err != nil {
		return nil, err
	}

	pk, err := request.Pubkey()
	if err != nil {
		return nil, err
	}

	logger := rs.l.With(log.F{
		"method":     "GetHeader",
		"ip":         uc.IP,
		"slot":       slot,
		"parentHash": parentHash,
		"pubkey":     pk,
	})

	logger.Info("header requested")
	tGet := time.Now()

	maxProfitBlock, ok := rs.a.MaxProfitBlock(slot)
	if !ok {
		rs.m.MissHeaderCount.WithLabelValues("noSubmission").Add(1)
		return nil, ErrNoBuilderBid
	}

	m.AppendSince(tGet, "getHeader", "get")

	key := structs.PayloadKey{
		BlockHash: maxProfitBlock.Header.Trace.BlockHash,
		Slot:      structs.Slot(maxProfitBlock.Header.Trace.Slot),
		Proposer:  maxProfitBlock.Header.Trace.ProposerPubkey}
	if err := rs.d.CacheBlock(ctx, key, maxProfitBlock); err != nil {
		logger.Warnf("fail to cache block: %s", err.Error())
	}
	logger.Debug("payload cached")

	header := maxProfitBlock.Header
	if header.Header == nil {
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	if header.Header.GetParentHash() != parentHash {
		logger.WithField("expected", header.Header.GetParentHash()).WithField("got", parentHash).Debug("invalid parentHash")
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	if header.Trace.ProposerPubkey != pk.PublicKey {
		logger.WithField("expected", header.Trace.BuilderPubkey).WithField("got", pk.PublicKey).Debug("invalid pubkey")
		rs.m.MissHeaderCount.WithLabelValues("badHeader").Add(1)
		return nil, ErrNoBuilderBid
	}

	if zero := types.IntToU256(0); header.Trace.Value.Cmp(&zero) == 0 {
		rs.m.MissHeaderCount.WithLabelValues("zeroBid").Add(1)
		return nil, ErrZeroBid
	}

	fork := rs.beaconState.ForkVersion(slot)
	if fork == structs.ForkBellatrix {
		h, ok := header.Header.(*bellatrix.ExecutionPayloadHeader)
		if !ok {
			return nil, errors.New("incompatible fork state")
		}
		bid := &bellatrix.BuilderBid{
			BellatrixHeader: h,
			BellatrixValue:  header.Trace.Value,
			BellatrixPubkey: rs.config.PubKey,
		}
		tSignature := time.Now()
		signature, err := types.SignMessage(bid, rs.config.BuilderSigningDomain, rs.config.SecretKey)
		m.AppendSince(tSignature, "getHeader", "signature")
		if err != nil {
			return nil, ErrInternal
		}

		if rs.config.Distributed {
			go rs.streamCacheBlock(logger, key, maxProfitBlock.Payload)
		}

		logger.With(log.F{
			"processingTimeMs": time.Since(tStart).Milliseconds(),
			"bidValue":         header.Trace.Value.String(),
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
		h, ok := header.Header.(*capella.ExecutionPayloadHeader)
		if !ok {
			return nil, errors.New("incompatible fork state")
		}
		bid := capella.BuilderBid{
			CapellaHeader: h,
			CapellaValue:  header.Trace.Value,
			CapellaPubkey: rs.config.PubKey,
		}
		tSignature := time.Now()
		signature, err := types.SignMessage(&bid, rs.config.BuilderSigningDomain, rs.config.SecretKey)
		m.AppendSince(tSignature, "getHeader", "signature")
		if err != nil {
			return nil, ErrInternal
		}

		if rs.config.Distributed {
			go rs.streamCacheBlock(logger, key, maxProfitBlock.Payload)
		}

		logger.With(log.F{
			"processingTimeMs": time.Since(tStart).Milliseconds(),
			"bidValue":         header.Trace.Value.String(),
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

func (rs *Relay) streamCacheBlock(logger log.Logger, key structs.PayloadKey, block structs.BlockBidAndTrace) {
	ctx, cancel := context.WithTimeout(context.Background(), structs.DurationPerSlot)
	defer cancel()

	if rs.sc.Contains(key) {
		logger.Debug("already streamed, skipping")
		return
	}

	if err := rs.s.PublishCacheBlock(ctx, block); err != nil {
		rs.l.WithError(err).Warn("failed to stream cache block: %w", err)
	}
	rs.sc.Add(key, struct{}{})
}

// GetPayload is called by a block proposer communicating through mev-boost and reveals execution payload of given signed beacon block if stored
func (rs *Relay) GetPayload(ctx context.Context, m *structs.MetricGroup, uc structs.UserContent, payloadRequest structs.SignedBlindedBeaconBlock) (structs.GetPayloadResponse, error) {
	tStart := time.Now()
	defer m.AppendSince(tStart, "getPayload", "all")

	logger := rs.l.With(log.F{
		"method":        "GetPayload",
		"ip":            uc.IP,
		"slot":          payloadRequest.Slot(),
		"block_number":  payloadRequest.BlockNumber(),
		"blockHash":     payloadRequest.BlockHash(),
		"responseDelay": rs.config.GetPayloadResponseDelay.String(),
	})

	if len(payloadRequest.Signature()) != 96 {
		return nil, ErrInvalidSignature
	}

	slotStart := int64(rs.beaconState.Genesis().GenesisTime+(payloadRequest.Slot()*12)) * 1000
	now := time.Now().UnixMilli()
	msIntoSlot := now - slotStart
	if msIntoSlot > int64(rs.config.GetPayloadRequestTimeLimit.Milliseconds()) {
		logger.WithField("msIntoSlot", msIntoSlot).Debug("requested too late")
		return nil, ErrLateRequest
	}

	if msIntoSlot < 0 {
		delayMillis := (msIntoSlot * -1) + int64(rand.Intn(50)) //nolint:gosec
		logger.WithField("msIntoSlot", msIntoSlot).Debug("requested too early - delaying")
		time.Sleep(time.Duration(delayMillis) * time.Millisecond)
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

	logger = logger.WithField("pubkey", pk)
	logger.WithField("event", "payload_requested").Info("payload requested")

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
		logger.WithField("event", "invalid_payload_key").WithError(err).Warn("error getting payload")
		return nil, ErrNoPayloadFound
	}

	payload, fromCache, err := rs.d.GetPayload(ctx, forkv, key)
	if err != nil || payload == nil {
		logger.WithField("event", "storage_error").WithError(err).Warn("error getting payload")
		return nil, ErrNoPayloadFound
	}
	m.AppendSince(tGet, "getPayload", "get")

	tValidatePayload := time.Now()
	if err := validatePayload(payload, payloadRequest); err != nil {
		logger.WithField("event", "invalid_payload").WithError(err).Warn("error validating payload")
		return nil, err
	}
	m.AppendSince(tValidatePayload, "getPayload", "validatePayload")

	logger = logger.With(log.F{
		"from_cache":       fromCache,
		"builder":          payload.BuilderPubkey().String(),
		"processingTimeMs": time.Since(tStart).Milliseconds(),
	})

	var (
		storeRequest = rs.wh != nil
		storeTrace   = false
	)
	defer func() {
		go func() {
			rs.runnignAsyncs.Add(1)
			defer rs.runnignAsyncs.Done()

			if storeRequest {
				rs.storeGetPayloadRequest(logger, m, tStart, payloadRequest)
			}

			if storeTrace {
				rs.storeTraceDelivered(logger, payloadRequest.Slot(), payload)
			}
		}()
	}()

	if rs.config.PublishBlock {
		beaconBlock, err := payloadRequest.ToBeaconBlock(payload.ExecutionPayload())
		if err != nil {
			logger.WithField("event", "wrong_publish_payload").WithError(err).Error("fail to create block for publication")
			return nil, ErrWrongPayload
		}
		if err = rs.beacon.PublishBlock(ctx, beaconBlock); err != nil {
			logger.WithField("event", "publish_error").WithError(err).Error("fail to publish block to beacon node")
			return nil, ErrFailedToPublish
		}
		logger.WithField("event", "published").Info("published block to beacon node")
		// Delay the return of response block publishing
		time.Sleep(rs.config.GetPayloadResponseDelay)
	}

	storeTrace = true // everything was correct, so flag to store the trace

	if rs.lastDeliveredSlot.Load() < payloadRequest.Slot() {
		rs.lastDeliveredSlot.Store(payloadRequest.Slot())
	} else {
		return nil, ErrPayloadAlreadyDelivered
	}

	exp := payload.ExecutionPayload()

	go rs.streamDeliveredSlot(key.Slot)

	rs.m.PayloadCacheHitCount.WithLabelValues(strconv.FormatBool(fromCache)).Add(1)

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
			"event":        "payload_sent",
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
			"event":        "payload_sent",
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
	logger.Error("unknown fork failure")
	return nil, errors.New("unknown fork")

}

func (rs *Relay) streamDeliveredSlot(slot structs.Slot) {
	ctx, cancel := context.WithTimeout(context.Background(), structs.DurationPerSlot)
	defer cancel()

	if err := rs.s.PublishSlotDelivered(ctx, slot); err != nil {
		rs.l.WithError(err).Warn("failed to stream delivered slot: %w", err)
	}
}

func (rs *Relay) storeGetPayloadRequest(logger log.Logger, m *structs.MetricGroup, ts time.Time, payloadRequest structs.SignedBlindedBeaconBlock) {
	tStoreWarehouse := time.Now()

	req := wh.StoreRequest{
		DataType:  "GetPayloadRequest",
		Data:      payloadRequest.Raw(),
		Slot:      payloadRequest.Slot(),
		Id:        payloadRequest.BlockHash().String(),
		Timestamp: ts,
	}

	if err := rs.wh.StoreAsync(context.Background(), req); err != nil {
		logger.WithError(err).Warn("failed to store in warehouse")
		return
	}

	m.AppendSince(tStoreWarehouse, "getPayload", "storeWarehouse")
}

func validatePayload(expected structs.BlockBidAndTrace, requested structs.SignedBlindedBeaconBlock) error {
	have, err := expected.ExecutionHeaderHash()
	if err != nil {
		return fmt.Errorf("failed to read expected header hash: %w", err)
	}

	got, err := requested.ExecutionHeaderHash()
	if err != nil {
		return fmt.Errorf("failed to read requested header hash: %w", err)
	}

	if have != got {
		return fmt.Errorf("%w: expected %s, received %s", ErrInvalidExecutionPayload, have, got)
	}

	return nil
}

func (rs *Relay) storeTraceDelivered(logger log.Logger, slot uint64, payload structs.BlockBidAndTrace) {
	trace, err := payload.ToDeliveredTrace(slot)
	if err != nil {
		logger.WithField("event", "wrong_evidence_payload").WithError(err).Error("failed to generate delivered payload")
		return
	}

	if err := rs.das.PutDelivered(context.Background(), structs.Slot(slot), trace, rs.config.TTL); err != nil {
		logger.WithField("event", "evidence_failure").WithError(err).Warn("failed to set payload after delivery")
		return
	}
}

type TimeoutWaitGroup struct {
	running int64
	done    chan struct{}
}
