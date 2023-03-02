package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"

	rpctypes "github.com/blocknative/dreamboat/pkg/client/sim/types"
	"github.com/blocknative/dreamboat/pkg/structs"
)

var (
	ErrWrongFeeRecipient = errors.New("wrong fee recipient")
	ErrInvalidRandao     = errors.New("randao is invalid")
)

// SubmitBlock Accepts block from trusted builder and stores
func (rs *Relay) SubmitBlock(ctx context.Context, m *structs.MetricGroup, sbr structs.BuilderSubmitBlockRequest) error {
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

func (rs *Relay) validateBlock(ctx context.Context, sbr structs.BuilderSubmitBlockRequest) (err error) {
	if rs.config.AllowedListedBuilders != nil && sbr.Message != nil {
		if _, ok := rs.config.AllowedListedBuilders[sbr.BuilderPubkey()]; ok {
			return nil
		}
	}

	err = rs.bvc.ValidateBlock(ctx, &rpctypes.BuilderBlockValidationRequest{
		//Todo  Pass correct structure
		// BuilderSubmitBlockRequest: sbr,
	})
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	return nil
}

func (rs *Relay) verifySignature(ctx context.Context, sbr structs.BuilderSubmitBlockRequest) (err error) {
	msg, err := types.ComputeSigningRoot(sbr.Message(), rs.config.BuilderSigningDomain)
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

func (rs *Relay) storeSubmission(ctx context.Context, m *structs.MetricGroup, sbr structs.BuilderSubmitBlockRequest) (newMax bool, err error) {
	complete, err := prepareContents(sbr, rs.config)
	if err != nil {
		return false, fmt.Errorf("fail to generate contents from block submission: %w", err)
	}

	tPutPayload := time.Now()
	if err := rs.d.PutPayload(ctx, SubmissionToKey(sbr), &complete.Payload, rs.config.TTL); err != nil {
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

func prepareContents(sbr structs.BuilderSubmitBlockRequest, conf RelayConfig) (s structs.CompleteBlockstruct, err error) {

	signedBuilderBid, err := SubmitBlockRequestToSignedBuilderBid(
		sbr,
		conf.SecretKey,
		&conf.PubKey,
		conf.BuilderSigningDomain,
	)
	if err != nil {
		return s, err
	}

	header, err := PayloadToPayloadHeader(sbr.ExecutionPayload())
	if err != nil {
		return s, err
	}

	s.Payload = SubmitBlockRequestToBlockBidAndTrace("bellatrix", signedBuilderBid, sbr)
	s.Header = structs.HeaderAndTrace{
		Header: header,
		Trace: &structs.BidTraceWithTimestamp{
			BidTraceExtended: structs.BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 sbr.Slot(),
					ParentHash:           s.Payload.Payload.Data.ParentHash(),
					BlockHash:            s.Payload.Payload.Data.BlockHash(),
					BuilderPubkey:        s.Payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       s.Payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: s.Payload.Trace.Message.ProposerFeeRecipient,
					Value:                sbr.Value(),
					GasLimit:             s.Payload.Trace.Message.GasLimit,
					GasUsed:              s.Payload.Trace.Message.GasUsed,
				},
				BlockNumber: s.Payload.Payload.Data.BlockNumber(),
				NumTx:       uint64(len(s.Payload.Payload.Data.Transactions())),
			},
			Timestamp:   uint64(time.Now().UnixMilli() / 1_000),
			TimestampMs: uint64(time.Now().UnixMilli()),
		},
	}
	return s, nil
}

func verifyBlock(sbr structs.BuilderSubmitBlockRequest, beaconState State) (bool, error) { // TODO(l): remove FB type
	if sbr == nil || sbr.Message() == nil {
		return false, ErrEmptyBlock
	}

	expectedTimestamp := beaconState.Genesis().GenesisTime + (sbr.Slot() * 12)
	if sbr.Timestamp() != expectedTimestamp {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidTimestamp, sbr.Timestamp(), expectedTimestamp)
	}

	if structs.Slot(sbr.Slot()) < beaconState.HeadSlot() {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidSlot, sbr.Slot(), beaconState.HeadSlot())
	}

	if randao := beaconState.Randao(); randao != submitBlockRequest.ExecutionPayload.Random.String() {
		return false, fmt.Errorf("%w: got %s, expected %s", ErrInvalidRandao, submitBlockRequest.ExecutionPayload.Random.String(), randao)
	}

	return true, nil
}

func SubmissionToKey(submission structs.BuilderSubmitBlockRequest) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: submission.BlockHash(),
		Proposer:  submission.ProposerPubkey(),
		Slot:      structs.Slot(submission.Slot()),
	}
}

func SubmitBlockRequestToBlockBidAndTrace(versionType string, signedBuilderBid *types.SignedBuilderBid, sbr structs.BuilderSubmitBlockRequest) structs.BlockBidAndTrace { // TODO(l): remove FB type
	return structs.BlockBidAndTrace{
		Trace: &types.SignedBidTrace{
			Message:   sbr.Message(),
			Signature: sbr.Signature(),
		},
		Bid: &types.GetHeaderResponse{
			Version: types.VersionString(versionType),
			Data:    signedBuilderBid,
		},
		Payload: &structs.GetPayloadResponse{
			Version: types.VersionString(versionType),
			Data:    sbr.ExecutionPayload(),
		},
	}
}

func PayloadToPayloadHeader(p structs.ExecutionPayload) (*structs.ExecutionPayloadHeader, error) {
	if p == nil {
		return nil, types.ErrNilPayload
	}

	txs := [][]byte{}
	for _, tx := range p.Transactions() {
		txs = append(txs, tx)
	}

	transactions := types.Transactions{Transactions: txs}
	txroot, err := transactions.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	var withdrawalsRoot [32]byte
	w := p.Withdrawals()
	if w != nil {
		withdrawalsRoot, err = w.HashTreeRoot()
		if err != nil {
			return nil, err
		}
	}

	return &structs.ExecutionPayloadHeader{
		ExecutionPayloadHeader: types.ExecutionPayloadHeader{
			ParentHash:       p.ParentHash(),
			FeeRecipient:     p.FeeRecipient(),
			StateRoot:        p.StateRoot(),
			ReceiptsRoot:     p.ReceiptsRoot(),
			LogsBloom:        p.LogsBloom(),
			Random:           p.Random(),
			BlockNumber:      p.BlockNumber(),
			GasLimit:         p.GasLimit(),
			GasUsed:          p.GasUsed(),
			Timestamp:        p.Timestamp(),
			ExtraData:        p.ExtraData(),
			BaseFeePerGas:    p.BaseFeePerGas(),
			BlockHash:        p.BlockHash(),
			TransactionsRoot: txroot,
		},
		WithdrawalsRoot: withdrawalsRoot,
	}, nil
}
