package relay

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"

	wh "github.com/blocknative/dreamboat/datastore/warehouse"
	rpctypes "github.com/blocknative/dreamboat/sim/client/types"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
)

const (
	StateRecheckDelay = time.Second
)

var (
	ErrWrongFeeRecipient     = errors.New("wrong fee recipient")
	ErrInvalidWithdrawalSlot = errors.New("invalid withdrawal slot")
	ErrInvalidWithdrawalRoot = errors.New("invalid withdrawal root")
	ErrInvalidRandao         = errors.New("randao is invalid")
)

// SubmitBlock Accepts block from trusted builder and stores
func (rs *Relay) SubmitBlock(ctx context.Context, m *structs.MetricGroup, uc structs.UserContent, sbr structs.SubmitBlockRequest) error {
	tStart := time.Now()
	defer m.AppendSince(tStart, "submitBlock", "all")
	value := sbr.Value()
	headSlot := rs.beaconState.HeadSlot()
	logger := rs.l.With(log.F{
		"method":         "SubmitBlock",
		"ip":             uc.IP,
		"builder":        sbr.BuilderPubkey(),
		"blockHash":      sbr.BlockHash(),
		"headSlot":       headSlot,
		"slot":           sbr.Slot(),
		"slotDiff":       int64(sbr.Slot()) - int64(headSlot),
		"proposer":       sbr.ProposerPubkey(),
		"bid":            value.String(),
		"withdrawalsNum": len(sbr.Withdrawals()),
		"size":           len(sbr.Raw()),
	})

	bRetried, err := verifyBlock(sbr, rs.beaconState)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}

	tCheckRegistration := time.Now()
	gasLimit, err := rs.checkRegistration(ctx, sbr.ProposerPubkey(), sbr.ProposerFeeRecipient())
	if err != nil {
		return err
	}
	m.AppendSince(tCheckRegistration, "submitBlock", "checkRegistration")

	valErr := make(chan error, 1)
	go func(ctx context.Context, gasLimit uint64, sbr structs.SubmitBlockRequest, chErr chan error) {
		defer close(chErr)

		tValidateBlock := time.Now()
		if err := rs.validateBlock(ctx, gasLimit, sbr); err != nil {
			chErr <- err
			return
		}
		m.AppendSince(tValidateBlock, "submitBlock", "validateBlock")
	}(ctx, gasLimit, sbr, valErr)

	tVerify := time.Now()
	if err := rs.verifySignature(ctx, sbr); err != nil {
		return err
	}
	m.AppendSince(tVerify, "submitBlock", "verify")

	root, wRetried, err := verifyWithdrawals(rs.beaconState, sbr)
	logger = logger.With(log.F{"withdrawalsRoot": root, "registeredGasLimit": gasLimit})
	if err != nil {
		return fmt.Errorf("failed to verify withdrawals: %w", err)
	}

	// wait for validations
	select {
	case err := <-valErr:
		if err != nil {
			logger.WithError(err).Debug("block validation failure")
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	isNewMax, err := rs.storeSubmission(ctx, logger, m, sbr)
	if err != nil {
		return err
	}

	if rs.wh != nil {
		tStoreWarehouse := time.Now()
		req := wh.StoreRequest{
			DataType:  "SubmitBlockRequest",
			Data:      sbr.Raw(),
			Slot:      sbr.Slot(),
			Id:        sbr.BlockHash().String(),
			Timestamp: tStart,
		}
		if err := rs.wh.StoreAsync(context.Background(), req); err != nil {
			logger.WithError(err).Warn("failed to store in warehouse")
			// we should not return error because it's already been stored for delivery
		} else {
			m.AppendSince(tStoreWarehouse, "submitBlock", "storeWarehouse")
		}
	}

	processingTime := time.Since(tStart)
	// subtract the retry waiting times
	if wRetried {
		processingTime -= StateRecheckDelay
	}
	if bRetried {
		processingTime -= StateRecheckDelay
	}
	logger.With(log.F{
		"processingTimeMs":  processingTime.Milliseconds(),
		"is_new_max":        isNewMax,
		"retry-withdrawals": wRetried,
		"retry-block":       bRetried,
	}).Trace("builder block stored")

	return nil
}

func (rs *Relay) validateBlock(ctx context.Context, gasLimit uint64, sbr structs.SubmitBlockRequest) (err error) {
	if !rs.bvc.IsSet() {
		return nil
	}

	if rs.config.AllowedListedBuilders != nil && sbr.Slot() > 0 {
		if _, ok := rs.config.AllowedListedBuilders[sbr.BuilderPubkey()]; ok {
			return nil
		}
	}

	switch t := sbr.(type) {
	case *bellatrix.SubmitBlockRequest:
		rpccall := &rpctypes.BuilderBlockValidationRequest{
			SubmitBlockRequest: t,
			RegisteredGasLimit: gasLimit,
		}

		if err = rs.bvc.ValidateBlock(ctx, rpccall); err != nil {
			return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
		}
		return

	case *capella.SubmitBlockRequest:
		hW := structs.HashWithdrawals{Withdrawals: t.Withdrawals()}
		withdrawalsRoot, err2 := hW.HashTreeRoot()
		if err2 != nil {
			return fmt.Errorf("%w: %s", ErrVerification, err2.Error()) // TODO: multiple err wrapping in Go 1.20
		}
		rpccall := &rpctypes.BuilderBlockValidationRequestV2{
			SubmitBlockRequest: t,
			RegisteredGasLimit: gasLimit,
			WithdrawalsRoot:    withdrawalsRoot,
		}
		if err = rs.bvc.ValidateBlockV2(ctx, rpccall); err != nil {
			return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
		}
		return

		// case *deneb.SubmitBlockRequest:
		// 	hW := structs.HashWithdrawals{Withdrawals: t.Withdrawals()}
		// 	withdrawalsRoot, err2 := hW.HashTreeRoot()
		// 	if err2 != nil {
		// 		return fmt.Errorf("%w: %s", ErrVerification, err2.Error()) // TODO: multiple err wrapping in Go 1.20
		// 	}
		// 	rpccall := &rpctypes.BuilderBlockValidationRequestV3{
		// 		SubmitBlockRequest: t,
		// 		RegisteredGasLimit: gasLimit,
		// 		WithdrawalsRoot:    withdrawalsRoot,
		// 	}
		// 	if err = rs.bvc.ValidateBlockV3(ctx, rpccall); err != nil {
		// 		return fmt.Errorf("%w: %s", ErrVerification, err.Error()) // TODO: multiple err wrapping in Go 1.20
		// 	}
		// 	return
	}

	return nil
}

func (rs *Relay) verifySignature(ctx context.Context, sbr structs.SubmitBlockRequest) (err error) {

	if rs.config.AllowedListedBuilders != nil && sbr.Slot() > 0 {
		if _, ok := rs.config.AllowedListedBuilders[sbr.BuilderPubkey()]; ok {
			return nil
		}
	}

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

func (rs *Relay) checkRegistration(ctx context.Context, pubkey types.PublicKey, proposerFeeRecipient types.Address) (gasLimit uint64, err error) {
	if v, ok := rs.cache.Get(pubkey); ok {
		if int(time.Since(v.Time)) > rand.Intn(int(rs.config.RegistrationCacheTTL))+int(rs.config.RegistrationCacheTTL) {
			rs.cache.Remove(pubkey)
		}

		if v.Entry.Message.FeeRecipient == proposerFeeRecipient {
			return v.Entry.Message.GasLimit, nil
		}
	}

	v, err := rs.vstore.GetRegistration(ctx, pubkey)
	if err != nil {
		return 0, fmt.Errorf("fail to check registration: %w", err)
	}

	if v.Message.FeeRecipient != proposerFeeRecipient {
		return 0, ErrWrongFeeRecipient
	}

	rs.cache.Add(pubkey, structs.ValidatorCacheEntry{
		Time:  time.Now(),
		Entry: v,
	})
	return v.Message.GasLimit, nil
}

func (rs *Relay) storeSubmission(ctx context.Context, logger log.Logger, m *structs.MetricGroup, sbr structs.SubmitBlockRequest) (newMax bool, err error) {
	if rs.config.SecretKey == nil {
		return false, ErrMissingSecretKey
	}

	complete, err := sbr.PreparePayloadContents(rs.config.SecretKey, &rs.config.PubKey, rs.config.BuilderSigningDomain)
	if err != nil {
		return false, fmt.Errorf("fail to generate contents from block submission: %w", err)
	}

	tPutPayload := time.Now()

	if err := rs.d.PutPayload(context.Background(), sbr.ToPayloadKey(), complete.Payload, rs.config.PayloadDataTTL); err != nil {
		return false, fmt.Errorf("%w block as payload: %s", ErrStore, err.Error()) // TODO: multiple err wrapping in Go 1.20
	}
	m.AppendSince(tPutPayload, "submitBlock", "putPayload")

	tAddAuction := time.Now()
	bid, err := rs.bidExtended(sbr, complete.Header.Header)
	if err != nil {
		return false, fmt.Errorf("failed to generate bid: %w", err)
	}
	newMax = rs.a.AddBlock(bid)
	m.AppendSince(tAddAuction, "submitBlock", "addAuction")

	rs.runnignAsyncs.Add(1)
	go func(wg *structs.TimeoutWaitGroup, trace structs.BidTraceWithTimestamp, newMax bool) {
		defer wg.Done()
		if err = rs.das.PutBuilderBlockSubmission(context.Background(), trace, newMax); err != nil {
			rs.l.WithField("trace", trace).WithError(err).Error("error storing block builder submission")
		}
	}(rs.runnignAsyncs, complete.Header.Trace, newMax)

	if rs.config.Distributed {
		go rs.s.PublishBuilderBid(context.Background(), bid)
	}

	return newMax, nil
}

func (rs *Relay) bidExtended(sbr structs.SubmitBlockRequest, header structs.ExecutionPayloadHeader) (structs.BuilderBidExtended, error) {
	fork := rs.beaconState.ForkVersion(structs.Slot(sbr.Slot()))
	if fork == structs.ForkBellatrix {
		header, ok := header.(*bellatrix.ExecutionPayloadHeader)
		if !ok {
			return nil, errors.New("failed to cast header to bellatrix")
		}
		return &bellatrix.BuilderBidExtended{
			BellatrixBuilderBid: bellatrix.BuilderBid{
				BellatrixHeader: header,
				BellatrixValue:  sbr.Value(),
				BellatrixPubkey: sbr.BuilderPubkey(),
			},
			BellatrixProposer: sbr.ProposerPubkey(),
			BellatrixSlot:     sbr.Slot(),
		}, nil
	} else if fork == structs.ForkCapella {
		header, ok := header.(*capella.ExecutionPayloadHeader)
		if !ok {
			return nil, errors.New("failed to cast header to capella")
		}
		return &capella.BuilderBidExtended{
			CapellaBuilderBid: capella.BuilderBid{
				CapellaHeader: header,
				CapellaValue:  sbr.Value(),
				CapellaPubkey: sbr.BuilderPubkey(),
			},
			CapellaProposer: sbr.ProposerPubkey(),
			CapellaSlot:     sbr.Slot(),
		}, nil
	}

	return nil, fmt.Errorf("unkown fork: %d", fork)
}

// returns a bool and an error, the bool indicates whether the block verification retried before succeeding
func verifyBlock(sbr structs.SubmitBlockRequest, beaconState State) (retry bool, err error) {
	if sbr == nil || sbr.Slot() == 0 {
		return false, ErrEmptyBlock
	}

	if expectedTimestamp := beaconState.Genesis().GenesisTime + (sbr.Slot() * 12); sbr.Timestamp() != expectedTimestamp {
		return false, fmt.Errorf("%w: got %d, expected %d", ErrInvalidTimestamp, sbr.Timestamp(), expectedTimestamp)
	}

	maxSlot := beaconState.HeadSlot() + 1
	minSlot := maxSlot - (structs.NumberOfSlotsInState - 1)
	if slot := structs.Slot(sbr.Slot()); slot > maxSlot || slot < minSlot {
		return false, fmt.Errorf("%w: got %d, expected slot in range [%d-%d]", ErrInvalidSlot, sbr.Slot(), minSlot, maxSlot)
	}

	if randao := beaconState.Randao(sbr.Slot()-1, sbr.ParentHash()); randao.Randao == "" || randao.Randao != sbr.Random().String() {
		time.Sleep(StateRecheckDelay) // recheck sync state for early blocks
		randao := beaconState.Randao(sbr.Slot()-1, sbr.ParentHash())
		if randao.Randao == "" {
			return true, fmt.Errorf("randao for slot %d not found", sbr.Slot())
		}
		if randao.Randao != sbr.Random().String() {
			return true, fmt.Errorf("%w: got %s, expected %s", ErrInvalidRandao, sbr.Random().String(), randao.Randao)
		}
		return true, nil
	}

	if bid := sbr.Value(); bid.BigInt().Cmp(big.NewInt(0)) == 0 && sbr.NumTx() == 0 {
		return false, ErrZeroBid
	}

	if sbr.BlockHash() != sbr.TraceBlockHash() {
		return false, ErrTraceMismatch
	}

	if sbr.ParentHash() != sbr.TraceParentHash() {
		return false, ErrTraceMismatch
	}

	return false, nil
}

func verifyWithdrawals(state State, submitBlockRequest structs.SubmitBlockRequest) (root types.Root, retried bool, err error) {
	withdrawals := submitBlockRequest.Withdrawals()
	if withdrawals == nil {
		return types.Root{}, false, nil
	}

	withdrawalState := state.Withdrawals(submitBlockRequest.Slot()-1, submitBlockRequest.ParentHash())
	retried = false
	if (withdrawalState.Root == types.Hash{}) {
		// recheck beacon sync state for early blocks
		time.Sleep(StateRecheckDelay)
		retried = true
		withdrawalState = state.Withdrawals(submitBlockRequest.Slot()-1, submitBlockRequest.ParentHash())
		if (withdrawalState.Root == types.Hash{}) {
			return root, retried, fmt.Errorf("withdrawals for slot %d not found", submitBlockRequest.Slot())
		}
	}

	// get latest withdrawals and verify the roots match
	hW := structs.HashWithdrawals{Withdrawals: withdrawals}
	withdrawalsRoot, err := hW.HashTreeRoot()
	if err != nil {
		return root, retried, fmt.Errorf("failed to compute withdrawals root: %w", err)
	}

	root = types.Root(withdrawalsRoot)
	if withdrawalState.Root != withdrawalsRoot {
		err = fmt.Errorf("%w: got %s, expected %s", ErrInvalidWithdrawalRoot, types.Root(withdrawalsRoot).String(), withdrawalState.Root.String())
	}

	return root, retried, err
}

func SubmissionToKey(submission *types.BuilderSubmitBlockRequest) structs.PayloadKey {
	return structs.PayloadKey{
		BlockHash: submission.ExecutionPayload.BlockHash,
		Proposer:  submission.Message.ProposerPubkey,
		Slot:      structs.Slot(submission.Message.Slot),
	}
}
