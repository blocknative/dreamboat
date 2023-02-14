package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"

	rpctypes "github.com/blocknative/dreamboat/pkg/client/sim/types"
	"github.com/blocknative/dreamboat/pkg/structs"
)

var ErrWrongFeeRecipient = errors.New("wrong fee recipient")

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

	resp, err := rs.bvc.ValidateBlock(ctx, &rpctypes.BuilderBlockValidationRequest{
		BuilderSubmitBlockRequest: *submitBlockRequest,
		RegisteredGasLimit:        10000000000,
	})
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	if resp.Error != nil {
		return fmt.Errorf("%w: %s", ErrVerification, resp.Error.Message) // TODO: multiple err wrapping in Go 1.20
	}

	_, err = verifyBlock(submitBlockRequest, rs.beaconState.Beacon())
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}

	tCheckDelivered := time.Now()
	if err := rs.isPayloadDelivered(ctx, submitBlockRequest.Message.Slot); err != nil {
		return err
	}
	m.AppendSince(tCheckDelivered, "submitBlock", "checkDelivered")

	tCheckRegistration := time.Now()
	if err := rs.checkRegistration(ctx, submitBlockRequest.Message.ProposerPubkey, submitBlockRequest.Message.ProposerFeeRecipient); err != nil {
		return err
	}
	m.AppendSince(tCheckRegistration, "submitBlock", "checkRegistration")

	tVerify := time.Now()
	if err := rs.verifySignature(ctx, submitBlockRequest); err != nil {
		return err
	}
	m.AppendSince(tVerify, "submitBlock", "verify")

	isNewMax, err := rs.storeSubmission(ctx, m, submitBlockRequest)
	if err != nil {
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(tStart).Milliseconds(),
		"is_new_max":       isNewMax,
	}).Trace("builder block stored")

	return nil
}

func (rs *Relay) isPayloadDelivered(ctx context.Context, slot uint64) (err error) {
	rs.deliveredCacheLock.RLock()
	_, ok := rs.deliveredCache[slot]
	rs.deliveredCacheLock.RUnlock()
	if ok {
		return ErrPayloadAlreadyDelivered
	}

	ok, err = rs.d.CheckSlotDelivered(ctx, slot)
	if ok {
		rs.deliveredCacheLock.Lock()
		if len(rs.deliveredCache) > 50 { // clean everything after every 50 slots
			for k := range rs.deliveredCache {
				delete(rs.deliveredCache, k)
			}
		}
		rs.deliveredCache[slot] = struct{}{}
		rs.deliveredCacheLock.Unlock()

		return ErrPayloadAlreadyDelivered
	}
	if err != nil {
		return err
	}

	return nil
}

func (rs *Relay) verifySignature(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest) (err error) {
	msg, err := types.ComputeSigningRoot(submitBlockRequest.Message, rs.config.BuilderSigningDomain)
	if err != nil {
		return ErrInvalidSignature
	}

	err = rs.ver.Enqueue(ctx, submitBlockRequest.Signature, submitBlockRequest.Message.BuilderPubkey, msg)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	return
}

func (rs *Relay) checkRegistration(ctx context.Context, pubkey types.PublicKey, proposerFeeRecipient types.Address) (err error) {
	if v, ok := rs.cache.Get(pubkey); ok {
		if int(time.Since(v.Time)) > rand.Intn(int(rs.config.RegistrationCacheTTL))+int(rs.config.RegistrationCacheTTL) {
			rs.cache.Remove(pubkey)
		}

		if v.Entry.Message.FeeRecipient == proposerFeeRecipient {
			return
		}
	}

	v, err := rs.vstore.GetRegistration(ctx, pubkey)
	if err != nil {
		return fmt.Errorf("fail to check registration: %w", err)
	}

	if v.Message.FeeRecipient != proposerFeeRecipient {
		return ErrWrongFeeRecipient
	}

	rs.cache.Add(pubkey, structs.ValidatorCacheEntry{
		Time:  time.Now(),
		Entry: v,
	})
	return nil
}

func (rs *Relay) storeSubmission(ctx context.Context, m *structs.MetricGroup, submitBlockRequest *types.BuilderSubmitBlockRequest) (newMax bool, err error) {
	complete, err := prepareContents(submitBlockRequest, rs.config)
	if err != nil {
		return false, fmt.Errorf("fail to generate contents from block submission: %w", err)
	}

	tPutPayload := time.Now()
	if err := rs.d.PutPayload(ctx, SubmissionToKey(submitBlockRequest), &complete.Payload, rs.config.TTL); err != nil {
		return false, fmt.Errorf("%w block as payload: %s", ErrStore, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	m.AppendSince(tPutPayload, "submitBlock", "putPayload")

	tAddAuction := time.Now()
	newMax = rs.a.AddBlock(&complete)
	m.AppendSince(tAddAuction, "submitBlock", "addAuction")

	tPutHeader := time.Now()

	b, err := json.Marshal(complete.Header)
	if err != nil {
		return newMax, fmt.Errorf("%w block as header: %s", ErrMarshal, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	err = rs.d.PutHeader(ctx, structs.HeaderData{
		Slot:           structs.Slot(submitBlockRequest.Message.Slot),
		Marshaled:      b,
		HeaderAndTrace: complete.Header,
	}, rs.config.TTL)
	if err != nil {
		return newMax, fmt.Errorf("%w block as header: %s", ErrStore, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	m.AppendSince(tPutHeader, "submitBlock", "putHeader")

	return newMax, nil
}

func prepareContents(submitBlockRequest *types.BuilderSubmitBlockRequest, conf RelayConfig) (s structs.CompleteBlockstruct, err error) {

	signedBuilderBid, err := SubmitBlockRequestToSignedBuilderBid(
		submitBlockRequest,
		conf.SecretKey,
		&conf.PubKey,
		conf.BuilderSigningDomain,
	)
	if err != nil {
		return s, err
	}

	header, err := types.PayloadToPayloadHeader(submitBlockRequest.ExecutionPayload)
	if err != nil {
		return s, err
	}

	s.Payload = SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitBlockRequest)
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
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}
	return s, nil
}

func verifyBlock(submitBlockRequest *types.BuilderSubmitBlockRequest, beaconState *structs.BeaconState) (bool, error) { // TODO(l): remove FB type
	if submitBlockRequest == nil || submitBlockRequest.Message == nil {
		return false, ErrEmptyBlock
	}

	expectedTimestamp := beaconState.GenesisTime + (submitBlockRequest.Message.Slot * 12)
	if submitBlockRequest.ExecutionPayload.Timestamp != expectedTimestamp {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidTimestamp, submitBlockRequest.ExecutionPayload.Timestamp, expectedTimestamp)
	}

	if structs.Slot(submitBlockRequest.Message.Slot) < beaconState.CurrentSlot {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidSlot, submitBlockRequest.Message.Slot, beaconState.CurrentSlot)
	}

	return true, nil
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
