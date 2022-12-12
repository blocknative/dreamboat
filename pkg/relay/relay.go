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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type State interface {
	Beacon() *structs.BeaconState
}

var (
	ErrNoPayloadFound        = errors.New("no payload found")
	ErrMissingRequest        = errors.New("req is nil")
	ErrMissingSecretKey      = errors.New("secret key is nil")
	UnregisteredValidatorMsg = "unregistered validator"
	noBuilderBidMsg          = "no builder bid"
	badHeaderMsg             = "invalid block header from datastore"
)

type Datastore interface {
	CheckSlotDelivered(context.Context, uint64) (bool, error)
	PutDelivered(context.Context, structs.Slot, structs.DeliveredTrace, time.Duration) error
	GetDelivered(context.Context, structs.PayloadQuery) (structs.BidTraceWithTimestamp, error)

	PutPayload(context.Context, structs.PayloadKey, *structs.BlockBidAndTrace, time.Duration) error
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockBidAndTrace, bool, error)

	PutHeader(ctx context.Context, hd structs.HeaderData, ttl time.Duration) error
	CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error
	GetMaxProfitHeader(ctx context.Context, slot uint64) (structs.HeaderAndTrace, error)

	PutRegistrationRaw(context.Context, structs.PubKey, []byte, time.Duration) error
	GetRegistration(context.Context, structs.PubKey) (types.SignedValidatorRegistration, error)
}

type Auctioneer interface {
	AddBlock(block *structs.CompleteBlockstruct) bool
	MaxProfitBlock(slot structs.Slot) (*structs.CompleteBlockstruct, bool)
}

type RegistrationManager interface {
	//GetStoreChan() chan StoreReq
	GetVerifyChan(buffer uint) chan VerifyReq
	//Set(k string, value uint64)

	SendStore(sReq StoreReq)
	Get(k string) (value uint64, ok bool)
}

type RelayConfig struct {
	BuilderSigningDomain  types.Domain
	ProposerSigningDomain types.Domain
	PubKey                types.PublicKey
	SecretKey             *bls.SecretKey

	TTL time.Duration
}

type Relay struct {
	d Datastore
	a Auctioneer
	l log.Logger

	regMngr RegistrationManager
	config  RelayConfig

	beaconState State

	m RelayMetrics
}

// NewRelay relay service
func NewRelay(l log.Logger, config RelayConfig, beaconState State, d Datastore, regMngr RegistrationManager, a Auctioneer) *Relay {
	rs := &Relay{
		d:           d,
		a:           a,
		l:           l,
		config:      config,
		beaconState: beaconState,
		regMngr:     regMngr,
	}
	rs.initMetrics()
	return rs
}

// verifyTimestamp ensures timestamp is not too far in the future
func verifyTimestamp(timestamp uint64) bool {
	return timestamp > uint64(time.Now().Add(10*time.Second).Unix())
}

// GetHeader is called by a block proposer communicating through mev-boost and returns a bid along with an execution payload header
func (rs *Relay) GetHeader(ctx context.Context, request structs.HeaderRequest) (*types.GetHeaderResponse, error) {
	timeStart := time.Now()

	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getHeader", "all"))
	defer timer.ObserveDuration()

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
	/*
		vd, err := rs.d.GetRegistration(ctx, pk)
		if err != nil {
			logger.Warn("unregistered validator")
			return nil, fmt.Errorf(noBuilderBidMsg)
		}
		if vd.Message.Pubkey != pk.PublicKey {
			logger.Warn("registration and request pubkey mismatch")
			return nil, fmt.Errorf("unknown validator")
		}*/

	logger = logger.With(log.F{
		"slot":       slot,
		"parentHash": parentHash,
		"pubkey":     pk,
	})

	logger.Info("header requested")
	timer2 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getHeader", "getters"))

	maxProfitBlock, ok := rs.a.MaxProfitBlock(slot)
	if !ok {
		logger.Warn(noBuilderBidMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

	if err := rs.d.CacheBlock(ctx, maxProfitBlock); err != nil {
		logger.Warnf("fail to cache blocks: %s", err.Error())
	}
	logger.Debug("payload cached")

	header := maxProfitBlock.Header

	timer2.ObserveDuration()

	if header.Header == nil || (header.Header.ParentHash != parentHash) {
		logger.Debug(badHeaderMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

	bid := types.BuilderBid{
		Header: header.Header,
		Value:  header.Trace.Value,
		Pubkey: rs.config.PubKey,
	}

	signature, err := types.SignMessage(&bid, rs.config.BuilderSigningDomain, rs.config.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("internal server error")
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
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
func (rs *Relay) GetPayload(ctx context.Context, payloadRequest *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error) { // TODO(l): remove FB type
	timeStart := time.Now()
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getPayload", "all"))
	defer timer.ObserveDuration()

	logger := rs.l.WithField("method", "GetPayload")

	if len(payloadRequest.Signature) != 96 {
		return nil, fmt.Errorf("invalid signature")
	}

	proposerPubkey, err := rs.beaconState.Beacon().KnownValidatorByIndex(payloadRequest.Message.ProposerIndex)
	if err != nil && errors.Is(err, structs.ErrUnknownValue) {
		return nil, fmt.Errorf("unknown validator for index %d", payloadRequest.Message.ProposerIndex)
	} else if err != nil {
		return nil, err
	}

	timer2 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getPayload", "verify"))
	pk, err := types.HexToPubkey(proposerPubkey.String())
	if err != nil {
		return nil, err
	}
	logger.With(log.F{
		"slot":      payloadRequest.Message.Slot,
		"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		"pubkey":    pk,
	}).Info("payload requested")

	msg, err := types.ComputeSigningRoot(payloadRequest.Message, rs.config.ProposerSigningDomain)
	if err != nil {
		return nil, fmt.Errorf("signature invalid") // err
	}

	respChA := NewRespC(1)
	rs.regMngr.GetVerifyChan(ResponseQueueOther) <- VerifyReq{
		Signature: payloadRequest.Signature,
		Pubkey:    pk,
		Msg:       msg,
		Response:  respChA}

	select {
	case err = <-respChA.Done():
	case <-ctx.Done():
		err = ctx.Err()
		respChA.Close(0, err)
		return nil, err
	}

	timer2.ObserveDuration()
	if err != nil {
		logger.WithField(
			"pubkey", proposerPubkey,
		).Error("signature invalid")
		return nil, fmt.Errorf("signature invalid")
	}

	timer3 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getPayload", "getPayload"))
	key := structs.PayloadKey{
		BlockHash: payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  pk,
		Slot:      structs.Slot(payloadRequest.Message.Slot),
	}

	payload, fromCache, err := rs.d.GetPayload(ctx, key)
	if err != nil || payload == nil {
		logger.WithError(err).With(log.F{
			"pubkey":    pk,
			"slot":      payloadRequest.Message.Slot,
			"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		}).Error("no payload found")
		return nil, ErrNoPayloadFound
	}
	timer3.ObserveDuration()

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"slot":             payloadRequest.Message.Slot,
		"blockHash":        payload.Payload.Data.BlockHash,
		"blockNumber":      payload.Payload.Data.BlockNumber,
		"stateRoot":        payload.Payload.Data.StateRoot,
		"feeRecipient":     payload.Payload.Data.FeeRecipient,
		"bid":              payload.Bid.Data.Message.Value,
		"from_cache":       fromCache,
		"numTx":            len(payload.Payload.Data.Transactions),
	}).Info("payload fetched")

	timer4 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getPayload", "putDelivered"))
	response := types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    payload.Payload.Data,
	}

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

	if err := rs.d.PutDelivered(ctx, structs.Slot(payloadRequest.Message.Slot), trace, rs.config.TTL); err != nil {
		rs.l.WithError(err).Warn("failed to set payload after delivery")
	}
	timer4.ObserveDuration()

	logger.With(log.F{
		"slot":             payloadRequest.Message.Slot,
		"blockHash":        payload.Payload.Data.BlockHash,
		"bid":              payload.Bid.Data.Message.Value,
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Info("payload sent")

	return &response, nil
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
func (rs *Relay) SubmitBlock(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	timeStart := time.Now()

	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("submitBlock", "all"))
	defer timer.ObserveDuration()

	logger := rs.l.With(log.F{
		"method":    "SubmitBlock",
		"builder":   submitBlockRequest.Message.BuilderPubkey,
		"blockHash": submitBlockRequest.ExecutionPayload.BlockHash,
		"slot":      submitBlockRequest.Message.Slot,
		"proposer":  submitBlockRequest.Message.ProposerPubkey,
		"bid":       submitBlockRequest.Message.Value.String(),
	})

	logger.Trace("block submission requested")
	_, err := rs.verifyBlock(submitBlockRequest, rs.beaconState.Beacon())
	if err != nil {
		logger.WithError(err).
			WithField("slot", submitBlockRequest.Message.Slot).
			WithField("builder", submitBlockRequest.Message.BuilderPubkey).
			Debug("block verification failed")
		return fmt.Errorf("verify block: %w", err)
	}

	timer2 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("submitBlock", "checkDelivered"))
	slot := structs.Slot(submitBlockRequest.Message.Slot)
	ok, err := rs.d.CheckSlotDelivered(ctx, uint64(slot))
	timer2.ObserveDuration()
	if ok {
		logger.Debug("block submission after payload delivered")
		return structs.ErrPayloadAlreadyDelivered
	}
	if err != nil {
		return err
	}

	timer3 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("submitBlock", "verify"))
	_, err = rs.verifySubmitSignature(ctx, submitBlockRequest)
	timer3.ObserveDuration()
	if err != nil {
		logger.WithError(err).
			WithField("slot", submitBlockRequest.Message.Slot).
			WithField("builder", submitBlockRequest.Message.BuilderPubkey).
			Debug("block verification failed")
		return fmt.Errorf("verify block: %w", err)
	}

	complete, err := rs.prepareContents(submitBlockRequest)
	if err != nil {
		logger.WithError(err).
			With(log.F{
				"slot":    submitBlockRequest.Message.Slot,
				"builder": submitBlockRequest.Message.BuilderPubkey,
			}).Debug("signature failed")

		return fmt.Errorf("block submission failed: %w", err)
	}

	b, err := json.Marshal(complete.Header)
	if err != nil {
		logger.WithError(err).Error("PutHeader marshal failed")
		return err
	}

	timer4 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("submitBlock", "putPayload"))
	timer4.ObserveDuration()
	timer5 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("submitBlock", "putHeader"))

	if err := rs.d.PutPayload(ctx, SubmissionToKey(submitBlockRequest), &complete.Payload, rs.config.TTL); err != nil {
		return err
	}

	isNewMax := rs.a.AddBlock(&complete)
	logger.WithField("is_new_max", isNewMax).Trace("block added to auctioneer")

	err = rs.d.PutHeader(ctx, structs.HeaderData{
		Slot:           slot,
		Marshaled:      b,
		HeaderAndTrace: complete.Header,
	}, rs.config.TTL)
	if err != nil {
		logger.WithError(err).Error("PutHeader failed")
		return err
	}
	timer5.ObserveDuration()

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
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

// GetValidators returns a list of registered block proposers in current and next epoch
func (rs *Relay) GetValidators() structs.BuilderGetValidatorsResponseEntrySlice {
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("getValidators", "all"))
	defer timer.ObserveDuration()

	//log := rs.l.WithField("method", "GetValidators")
	validators := rs.beaconState.Beacon().ValidatorsMap()
	//log.With(validators).Debug("validatored map sent")
	return validators
}

func (rs *Relay) verifyBlock(submitBlockRequest *types.BuilderSubmitBlockRequest, beaconState *structs.BeaconState) (bool, error) { // TODO(l): remove FB type
	if submitBlockRequest == nil || submitBlockRequest.Message == nil {
		return false, fmt.Errorf("block empty")
	}

	expectedTimestamp := beaconState.GenesisTime + (submitBlockRequest.Message.Slot * 12)
	if submitBlockRequest.ExecutionPayload.Timestamp != expectedTimestamp {
		return false, fmt.Errorf("builder submission with wrong timestamp. got %d, expected %d", submitBlockRequest.ExecutionPayload.Timestamp, expectedTimestamp)
	}

	if structs.Slot(submitBlockRequest.Message.Slot) <= beaconState.CurrentSlot {
		return false, fmt.Errorf("builder submission with wrong slot. got %d, expected %d", submitBlockRequest.Message.Slot, beaconState.CurrentSlot)
	}

	return true, nil
}

func (rs *Relay) verifySubmitSignature(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) (ok bool, err error) { // TODO(l): remove FB type
	msg, err := types.ComputeSigningRoot(submitBlockRequest.Message, rs.config.BuilderSigningDomain)
	if err != nil {
		return false, fmt.Errorf("signature invalid")
	}

	respChA := NewRespC(1)
	rs.regMngr.GetVerifyChan(ResponseQueueSubmit) <- VerifyReq{
		Signature: submitBlockRequest.Signature,
		Pubkey:    submitBlockRequest.Message.BuilderPubkey,
		Msg:       msg,
		Response:  respChA}

	select {
	case err = <-respChA.Done():
	case <-ctx.Done():
		err = ctx.Err()
		respChA.Close(0, err)
		return false, err
	}

	return (err != nil), err
	//return VerifySignature(SubmitBlockRequest.Message, rs.config.BuilderSigningDomain, SubmitBlockRequest.Message.BuilderPubkey[:], SubmitBlockRequest.Signature[:])
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
