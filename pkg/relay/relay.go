//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg/relay Datastore,State
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/verify"
)

type State interface {
	Beacon() *structs.BeaconState
}

type Verifier interface {
	Enqueue(ctx context.Context, sig [96]byte, pubkey [48]byte, msg [32]byte) (err error)
}

var (
	ErrNoPayloadFound        = errors.New("no payload found")
	ErrMissingRequest        = errors.New("req is nil")
	ErrMissingSecretKey      = errors.New("secret key is nil")
	UnregisteredValidatorMsg = "unregistered validator"
	ErrNoBuilderBid          = errors.New("no builder bid")
	ErrOldSlot               = errors.New("requested slot is old")
	ErrBadHeader             = "invalid block header from datastore"
)

type EvidenceStore interface {
	CheckSlotDelivered(context.Context, uint64) (bool, error)
	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error

	GetDelivered(ctx context.Context, slot uint64, payload structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error)
	GetBlocksReceived(ctx context.Context, slot uint64, payload structs.HeaderTraceQuery) ([]structs.BidTraceWithTimestamp, error)
	// GetDeliveredBatch(context.Context, []structs.PayloadQuery) ([]structs.BidTraceWithTimestamp, error)
}

type Datastore interface {
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error)

	PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) error
	CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error
	GetMaxProfitHeader(ctx context.Context, slot uint64) (structs.HeaderAndTrace, error)

	// to be changed
	// GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error)
	// GetHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.HeaderAndTrace, error)
	// GetHeadersByBlockNum(ctx context.Context, num uint64) ([]structs.HeaderAndTrace, error)
	// GetLatestHeaders(ctx context.Context, limit uint64, stopLag uint64) ([]structs.HeaderAndTrace, error)
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
}

type Relay struct {
	d       Datastore
	evStore EvidenceStore

	a Auctioneer
	l log.Logger

	ver    Verifier
	config RelayConfig

	beacon      Beacon
	beaconState State

	m RelayMetrics
}

// NewRelay relay service
func NewRelay(l log.Logger, config RelayConfig, beacon Beacon, ver Verifier, beaconState State, d Datastore, a Auctioneer) *Relay {
	rs := &Relay{
		d:           d,
		a:           a,
		l:           l,
		ver:         ver,
		config:      config,
		beacon:      beacon,
		beaconState: beaconState,
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
		logger.Debug(ErrBadHeader)
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
		return nil, fmt.Errorf("internal server error")
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
		return nil, fmt.Errorf("invalid signature")
	}

	proposerPubkey, err := rs.beaconState.Beacon().KnownValidatorByIndex(payloadRequest.Message.ProposerIndex)
	if err != nil && errors.Is(err, structs.ErrUnknownValue) {
		return nil, fmt.Errorf("unknown validator for index %d", payloadRequest.Message.ProposerIndex)
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
		return nil, fmt.Errorf("signature invalid") // err
	}
	ok, err := verify.VerifySignatureBytes(msg, payloadRequest.Signature[:], pk[:])
	if err != nil || !ok {
		return nil, fmt.Errorf("signature invalid")
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
		if err := rs.evStore.PutDelivered(ctx, slot, trace, rs.config.TTL); err != nil {
			logger.WithError(err).Warn("failed to set payload after delivery")
		}

		if rs.config.PublishBlock {
			beaconBlock := structs.SignedBlindedBeaconBlockToBeaconBlock(payloadRequest, payload.Payload.Data)
			if err := rs.beacon.PublishBlock(beaconBlock); err != nil {
				logger.WithError(err).Warn("fail to publish block to beacon node")
			} else {
				logger.Info("published block to beacon node")
			}
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

// ***** Relay Domain *****
// SubmitBlockRequestToSignedBuilderBid converts a builders block submission to a bid compatible with mev-boost
func SubmitBlockRequestToSignedBuilderBid(req *types.BuilderSubmitBlockRequest, sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (*types.SignedBuilderBid, error) { // TODO(l): remove FB type
	if req == nil {
		return nil, ErrMissingRequest
	}

	if sk == nil {
		return nil, ErrMissingSecretKey
	}

	header, err := types.PayloadToPayloadHeader(req.ExecutionPayload)
	if err != nil {
		return nil, err
	}

	builderBid := types.BuilderBid{
		Value:  req.Message.Value,
		Header: header,
		Pubkey: *pubkey,
	}

	sig, err := types.SignMessage(&builderBid, domain, sk)
	if err != nil {
		return nil, err
	}

	return &types.SignedBuilderBid{
		Message:   &builderBid,
		Signature: sig,
	}, nil
}

// SubmitBlock Accepts block from trusted builder and stores
func (rs *Relay) SubmitBlock(ctx context.Context, m *structs.MetricGroup, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	tStart := time.Now()
	defer m.AppendSince(tStart, "submitBlock", "all")

	logger := rs.l.With(log.F{
		"method":    "SubmitBlock",
		"builder":   submitBlockRequest.Message.BuilderPubkey,
		"blockHash": submitBlockRequest.ExecutionPayload.BlockHash,
		"slot":      submitBlockRequest.Message.Slot,
		"proposer":  submitBlockRequest.Message.ProposerPubkey,
		"bid":       submitBlockRequest.Message.Value.String(),
	})

	_, err := rs.verifyBlock(submitBlockRequest, rs.beaconState.Beacon())
	if err != nil {
		return fmt.Errorf("verify block: %w", err)
	}

	tCheckDelivered := time.Now()
	slot := structs.Slot(submitBlockRequest.Message.Slot)
	ok, err := rs.evStore.CheckSlotDelivered(ctx, uint64(slot)) // TODO(l): We should take is out from datastore, and it to cache memory
	m.AppendSince(tCheckDelivered, "submitBlock", "checkDelivered")
	if ok {
		return structs.ErrPayloadAlreadyDelivered
	}
	if err != nil {
		return err
	}

	tVerify := time.Now()
	msg, err := types.ComputeSigningRoot(submitBlockRequest.Message, rs.config.BuilderSigningDomain)
	if err != nil {
		return fmt.Errorf("signature invalid")
	}

	err = rs.ver.Enqueue(ctx, submitBlockRequest.Signature, submitBlockRequest.Message.BuilderPubkey, msg)
	m.AppendSince(tVerify, "submitBlock", "verify")

	if err != nil {
		return fmt.Errorf("verify block: %w", err)
	}

	complete, err := rs.prepareContents(submitBlockRequest)
	if err != nil {
		return fmt.Errorf("fail to generate contents from block submission: %w", err)
	}

	b, err := json.Marshal(complete.Header)
	if err != nil {
		return fmt.Errorf("fail to marshal block as header: %w", err)
	}

	tPutPayload := time.Now()
	if err := rs.d.PutPayload(ctx, SubmissionToKey(submitBlockRequest), &complete.Payload, rs.config.TTL); err != nil {
		return fmt.Errorf("fail to store block as payload: %w", err)
	}
	m.AppendSince(tPutPayload, "submitBlock", "putPayload")

	tAddAuction := time.Now()
	isNewMax := rs.a.AddBlock(&complete)
	m.AppendSince(tAddAuction, "submitBlock", "addAuction")

	tPutHeader := time.Now()
	err = rs.d.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		Marshaled:      b,
		HeaderAndTrace: complete.Header,
	}, rs.config.TTL)
	if err != nil {
		return fmt.Errorf("fail to store block as header: %w", err)
	}
	m.AppendSince(tPutHeader, "submitBlock", "putHeader")

	logger.With(log.F{
		"processingTimeMs": time.Since(tStart).Milliseconds(),
		"is_new_max":       isNewMax,
	}).Trace("builder block stored")

	return nil
}

func (rs *Relay) prepareContents(submitBlockRequest *types.BuilderSubmitBlockRequest) (structs.CompleteBlockstruct, error) {
	s := structs.CompleteBlockstruct{}

	signedBuilderBid, err := SubmitBlockRequestToSignedBuilderBid(
		submitBlockRequest,
		rs.config.SecretKey,
		&rs.config.PubKey,
		rs.config.BuilderSigningDomain,
	)
	if err != nil {
		return s, err
	}

	s.Payload = SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitBlockRequest)

	header, err := types.PayloadToPayloadHeader(submitBlockRequest.ExecutionPayload)
	if err != nil {
		return s, err
	}

	s.Header = structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 submitBlockRequest.Message.Slot,
					ParentHash:           s.Payload.Payload.Data.ParentHash,
					BlockHash:            s.Payload.Payload.Data.BlockHash,
					BuilderPubkey:        s.Payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       s.Payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: s.Payload.Trace.Message.ProposerFeeRecipient,
					Value:                submitBlockRequest.Message.Value,
					GasLimit:             s.Payload.Trace.Message.GasLimit,
					GasUsed:              s.Payload.Trace.Message.GasUsed,
				},
				BlockNumber: s.Payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(s.Payload.Payload.Data.Transactions)),
			},
			Timestamp: s.Payload.Payload.Data.Timestamp,
		},
	}

	return s, nil
}

func (rs *Relay) verifyBlock(submitBlockRequest *types.BuilderSubmitBlockRequest, beaconState *structs.BeaconState) (bool, error) { // TODO(l): remove FB type
	if submitBlockRequest == nil || submitBlockRequest.Message == nil {
		return false, fmt.Errorf("block empty")
	}

	expectedTimestamp := beaconState.GenesisTime + (submitBlockRequest.Message.Slot * 12)
	if submitBlockRequest.ExecutionPayload.Timestamp != expectedTimestamp {
		return false, fmt.Errorf("builder submission with wrong timestamp. got %d, expected %d", submitBlockRequest.ExecutionPayload.Timestamp, expectedTimestamp)
	}

	if structs.Slot(submitBlockRequest.Message.Slot) < beaconState.CurrentSlot {
		return false, fmt.Errorf("builder submission with wrong slot. got %d, expected %d", submitBlockRequest.Message.Slot, beaconState.CurrentSlot)
	}

	return true, nil
}

func SubmissionToKey(submission *types.BuilderSubmitBlockRequest) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: submission.ExecutionPayload.BlockHash,
		Proposer:  submission.Message.ProposerPubkey,
		Slot:      structs.Slot(submission.Message.Slot),
	}
}

func SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid *types.SignedBuilderBid, submitBlockRequest *types.BuilderSubmitBlockRequest) structs.BlockBidAndTrace { // TODO(l): remove FB type
	getHeaderResponse := types.GetHeaderResponse{
		Version: "bellatrix",
		Data:    signedBuilderBid,
	}

	getPayloadResponse := types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    submitBlockRequest.ExecutionPayload,
	}

	signedBidTrace := types.SignedBidTrace{
		Message:   submitBlockRequest.Message,
		Signature: submitBlockRequest.Signature,
	}

	return structs.BlockBidAndTrace{
		Trace:   &signedBidTrace,
		Bid:     &getHeaderResponse,
		Payload: &getPayloadResponse,
	}
}
