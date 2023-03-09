package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/pkg/structs"
)

var (
	ErrWrongFeeRecipient     = errors.New("wrong fee recipient")
	ErrInvalidWithdrawalSlot = errors.New("invalid withdrawal slot")
	ErrInvalidWithdrawalRoot = errors.New("invalid withdrawal root")
	ErrInvalidRandao     = errors.New("randao is invalid")
)

// SubmitBlock Accepts block from trusted builder and stores
func (rs *Relay) SubmitBlock(ctx context.Context, m *structs.MetricGroup, sbr structs.SubmitBlockRequest) error {
	tStart := time.Now()
	defer m.AppendSince(tStart, "submitBlock", "all")
	value := sbr.Value()
	logger := rs.l.With(log.F{
		"method":    "SubmitBlock",
		"builder":   sbr.BuilderPubkey(),
		"blockHash": sbr.BlockHash(),
		"slot":      sbr.Slot(),
		"proposer":  sbr.ProposerPubkey(),
		"bid":       value.String(),
	})

	_, err := verifyBlock(sbr, rs.beaconState)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}

	tCheckDelivered := time.Now()
	if err := rs.isPayloadDelivered(ctx, sbr.Slot()); err != nil {
		return err
	}
	m.AppendSince(tCheckDelivered, "submitBlock", "checkDelivered")

	tCheckRegistration := time.Now()
	if err := rs.checkRegistration(ctx, sbr.ProposerPubkey(), sbr.ProposerFeeRecipient()); err != nil {
		return err
	}
	m.AppendSince(tCheckRegistration, "submitBlock", "checkRegistration")

	tVerify := time.Now()
	if err := rs.verifySignature(ctx, sbr); err != nil {
		return err
	}
	m.AppendSince(tVerify, "submitBlock", "verify")

	tValidateBlock := time.Now()
	if err := rs.validateBlock(ctx, sbr); err != nil {
		return err
	}
	m.AppendSince(tValidateBlock, "submitBlock", "validateBlock")

	isNewMax, err := rs.storeSubmission(ctx, m, sbr)
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

func (rs *Relay) validateBlock(ctx context.Context, sbr structs.SubmitBlockRequest) (err error) {
	if rs.config.AllowedListedBuilders != nil && sbr.Slot() > 0 {
		if _, ok := rs.config.AllowedListedBuilders[sbr.BuilderPubkey()]; ok {
			return nil
		}
	}
	/*
		err = rs.bvc.ValidateBlock(ctx, &rpctypes.BuilderBlockValidationRequest{
			//Todo  Pass correct structure
			// BuilderSubmitBlockRequest: sbr,
		})
		if err != nil {
			return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
		}
	*/
	return nil
}

func (rs *Relay) verifySignature(ctx context.Context, sbr structs.SubmitBlockRequest) (err error) {
	msg, err := sbr.ComputeSigningRoot(rs.config.BuilderSigningDomain)
	if err != nil {
		return ErrInvalidSignature
	}

	err = rs.ver.Enqueue(ctx, sbr.Signature(), sbr.BuilderPubkey(), msg)
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

func (rs *Relay) storeSubmission(ctx context.Context, m *structs.MetricGroup, sbr structs.SubmitBlockRequest) (newMax bool, err error) {

	if rs.config.SecretKey == nil {
		return false, ErrMissingSecretKey
	}

	complete, err := sbr.PreparePayloadContents(rs.config.SecretKey, &rs.config.PubKey, rs.config.BuilderSigningDomain)
	if err != nil {
		return false, fmt.Errorf("fail to generate contents from block submission: %w", err)
	}

	tPutPayload := time.Now()

	// key := sbr.ToPayloadKey()
	// ma, _ := json.Marshal(complete.Payload)
	// rs.l.With(log.F{
	// 	"submissionkey":    fmt.Sprintf("payload-%s-%s-%d", key.BlockHash.String(), key.Proposer.String(), key.Slot),
	// 	"content":          complete.Payload,
	// 	"contentMarshaled": string(ma),
	// }).Debug("store key")

	if err := rs.d.PutPayload(ctx, sbr.ToPayloadKey(), complete.Payload, rs.config.TTL); err != nil {
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
		Slot:           structs.Slot(sbr.Slot()),
		Marshaled:      b,
		HeaderAndTrace: complete.Header,
	}, rs.config.TTL)
	if err != nil {
		return newMax, fmt.Errorf("%w block as header: %s", ErrStore, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	m.AppendSince(tPutHeader, "submitBlock", "putHeader")

	return newMax, nil
}

func verifyBlock(sbr structs.SubmitBlockRequest, beaconState State) (bool, error) { // TODO(l): remove FB type
	if sbr == nil || sbr.Slot() == 0 {
		return false, ErrEmptyBlock
	}

	expectedTimestamp := beaconState.Genesis().GenesisTime + (sbr.Slot() * 12)
	if sbr.Timestamp() != expectedTimestamp {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidTimestamp, sbr.Timestamp(), expectedTimestamp)
	}

	if structs.Slot(sbr.Slot()) < beaconState.HeadSlot() {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidSlot, sbr.Slot(), beaconState.HeadSlot())
	}

	if randao := beaconState.Randao(); randao != "" && randao != sbr.Random().String() { // DISABLE CHECK IF NOT OBTAINED BY BEACON
		return false, fmt.Errorf("%w: got %s, expected %s", ErrInvalidRandao, sbr.Random().String(), randao)
	}

	if err := verifyWithdrawals(beaconState, submitBlockRequest); err != nil {
		return false, fmt.Errorf("failed to verify withdrawals: %w", err)
	}

	return true, nil
}

func verifyWithdrawals(state State, submitBlockRequest *types.BuilderSubmitBlockRequest) error {
	var withdrawals []*capella.Withdrawal
	// TODO: withdrawals := payload.Withdrawals()

	if withdrawals != nil {
		// get latest withdrawals and verify the roots match
		withdrawalState := state.Withdrawals()
		withdrawalsRoot, err := structs.ComputeWithdrawalsRoot(withdrawals)
		if err != nil {
			return fmt.Errorf("failed to compute withdrawals root: %w", err)
		}
		if withdrawalState.Slot != structs.Slot(submitBlockRequest.Message.Slot) { // we still don't have the withdrawals yet
			return fmt.Errorf("%w: got %d, expected %d", ErrInvalidWithdrawalSlot, submitBlockRequest.Message.Slot, withdrawalState.Slot)
		} else if withdrawalState.Root != withdrawalsRoot {
			return fmt.Errorf("%w: got %s, expected %s", ErrInvalidWithdrawalRoot, withdrawalsRoot.String(), withdrawalState.Root.String())
		}
	}

	return nil
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

func SubmitBlockRequestToBlockBidAndTrace(versionType string, signedBuilderBid *types.SignedBuilderBid, submitBlockRequest *types.BuilderSubmitBlockRequest) structs.BlockBidAndTrace { // TODO(l): remove FB type
	getHeaderResponse := types.GetHeaderResponse{
		Version: types.VersionString(versionType),
		Data:    signedBuilderBid,
	}

	getPayloadResponse := types.GetPayloadResponse{
		Version: types.VersionString(versionType),
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
