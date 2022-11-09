//go:generate mockgen -source=relay.go -destination=../internal/mock/pkg/relay.go -package=mock_relay
package relay

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/lthibault/log"
	"golang.org/x/sync/errgroup"

	"github.com/blocknative/dreamboat/pkg/structs"
)

var (
	ErrNoPayloadFound        = errors.New("no payload found")
	ErrBeaconNodeSyncing     = errors.New("beacon node is syncing")
	ErrMissingRequest        = errors.New("req is nil")
	ErrMissingSecretKey      = errors.New("secret key is nil")
	ErrUnknownValue          = errors.New("value is unknown")
	UnregisteredValidatorMsg = "unregistered validator"
	noBuilderBidMsg          = "no builder bid"
	badHeaderMsg             = "invalid block header from datastore"
)

type State interface {
	Datastore() Datastore
	Beacon() BeaconState
}

type BeaconState interface {
	KnownValidatorByIndex(uint64) (types.PubkeyHex, error)
	IsKnownValidator(types.PubkeyHex) (bool, error)
	Genesis() GenesisInfo
	HeadSlot() structs.Slot
	ValidatorsMap() BuilderGetValidatorsResponseEntrySlice
}

type Relay interface {
	// Proposer APIs
	RegisterValidator(context.Context, []structs.SignedValidatorRegistration, State) error
	GetHeader(context.Context, structs.HeaderRequest, State) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock, State) (*types.GetPayloadResponse, error)

	// Builder APIs
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest, State) error
	GetValidators(State) BuilderGetValidatorsResponseEntrySlice
}

type DefaultRelay struct {
	config                Config
	builderSigningDomain  types.Domain
	proposerSigningDomain types.Domain
	regMngr               *RegisteredManager
}

// NewRelay relay service
func NewRelay(config Config, store Datastore) (*DefaultRelay, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	domainBuilder, err := ComputeDomain(types.DomainTypeAppBuilder, config.genesisForkVersion, types.Root{}.String())
	if err != nil {
		return nil, err
	}

	domainBeaconProposer, err := ComputeDomain(types.DomainTypeBeaconProposer, config.bellatrixForkVersion, config.genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	rm := NewRegisteredManager(20000)
	rs := &DefaultRelay{
		config:                config,
		builderSigningDomain:  domainBuilder,
		proposerSigningDomain: domainBeaconProposer,
		regMngr:               rm,
	}
	rm.RunWorkers(store, 300)
	return rs, nil
}

func (rs *DefaultRelay) Log() log.Logger {
	return rs.config.Log
}

// verifyTimestamp ensures timestamp is not too far in the future
func verifyTimestamp(timestamp uint64) bool {
	return timestamp > uint64(time.Now().Add(10*time.Second).Unix())
}

// ***** Builder Domain *****

// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *DefaultRelay) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration, state State) error {
	logger := rs.Log().WithField("method", "RegisterValidator")
	timeStart := time.Now()

	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < runtime.NumCPU(); i++ {
		start := i * (len(payload) / runtime.NumCPU())
		end := (i + 1) * (len(payload) / runtime.NumCPU())
		if i == runtime.NumCPU()-1 {
			end = len(payload)
		}
		g.Go(func() error {
			return rs.processValidator(ctx, payload[start:end], state)
		})
	}

	if err := g.Wait(); err != nil {
		logger.
			WithError(err).
			Debug("validator registration failed")
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"numberValidators": len(payload),
	}).Trace("validator registrations succeeded")
	log.Info("timeStart ", time.Since(timeStart))
	return nil
}

func (rs *DefaultRelay) processValidator(ctx context.Context, payload []structs.SignedValidatorRegistration, state State) error {
	logger := rs.Log().WithField("method", "RegisterValidator")
	timeStart := time.Now()

	for i := 0; i < len(payload) && ctx.Err() == nil; i++ {
		registerRequest := payload[i]
		ok, err := types.VerifySignature(
			registerRequest.Message,
			rs.builderSigningDomain,
			registerRequest.Message.Pubkey[:],
			registerRequest.Signature[:],
		)
		if !ok || err != nil {
			logger.
				WithError(err).
				WithField("pubkey", registerRequest.Message.Pubkey).
				Debug("signature invalid")
			return fmt.Errorf("signature invalid for %s", registerRequest.Message.Pubkey.String())
		}

		if verifyTimestamp(registerRequest.Message.Timestamp) {
			return fmt.Errorf("request too far in future for %s", registerRequest.Message.Pubkey.String())
		}

		pk := structs.PubKey{registerRequest.Message.Pubkey}

		ok, err = state.Beacon().IsKnownValidator(pk.PubkeyHex())
		if err != nil {
			return err
		} else if !ok {
			if rs.config.CheckKnownValidator {
				return fmt.Errorf("%s not a known validator", registerRequest.Message.Pubkey.String())
			} else {
				logger.
					WithField("pubkey", pk.PublicKey).
					WithField("slot", state.Beacon().HeadSlot()).
					Debug("not a known validator")
			}
		}

		// check previous validator registration
		previousValidator, err := state.Datastore().GetRegistration(ctx, pk)
		if err != nil && !errors.Is(err, ds.ErrNotFound) {
			logger.Warn(err)
		}

		if err == nil {
			// skip registration if
			if registerRequest.Message.Timestamp < previousValidator.Message.Timestamp {
				logger.WithField("pubkey", registerRequest.Message.Pubkey).Debug("request timestamp less than previous")
				continue
			}

			if registerRequest.Message.Timestamp == previousValidator.Message.Timestamp &&
				(registerRequest.Message.FeeRecipient != previousValidator.Message.FeeRecipient ||
					registerRequest.Message.GasLimit != previousValidator.Message.GasLimit) {
				// to help debug issues with validator set ups
				logger.With(log.F{
					"prevFeeRecipient":    previousValidator.Message.FeeRecipient,
					"requestFeeRecipient": registerRequest.Message.FeeRecipient,
					"prevGasLimit":        previousValidator.Message.GasLimit,
					"requestGasLimit":     registerRequest.Message.GasLimit,
					"pubkey":              registerRequest.Message.Pubkey,
				}).Debug("different registration fields")
			}
		}

		// officially register validator
		if err := state.Datastore().PutRegistration(ctx, pk, registerRequest.SignedValidatorRegistration, rs.config.TTL); err != nil {
			logger.WithField("pubkey", registerRequest.Message.Pubkey).WithError(err).Debug("Error in PutRegistration")
			return fmt.Errorf("failed to store %s", registerRequest.Message.Pubkey.String())
		}
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"numberValidators": len(payload),
	}).Trace("validator batch registered")

	return nil
}

// GetHeader is called by a block proposer communicating through mev-boost and returns a bid along with an execution payload header
func (rs *DefaultRelay) GetHeader(ctx context.Context, request structs.HeaderRequest, state State) (*types.GetHeaderResponse, error) {
	logger := rs.Log().WithField("method", "GetHeader")
	timeStart := time.Now()

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

	logger.Trace("header requested")

	vd, err := state.Datastore().GetRegistration(ctx, pk)
	if err != nil {
		logger.Warn("unregistered validator")
		return nil, fmt.Errorf(noBuilderBidMsg)
	}
	if vd.Message.Pubkey != pk.PublicKey {
		logger.Warn("registration and request pubkey mismatch")
		return nil, fmt.Errorf("unknown validator")
	}

	headers, err := state.Datastore().GetMaxProfitHeadersDesc(ctx, slot)
	if err != nil || len(headers) < 1 {
		logger.Warn(noBuilderBidMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

	header := headers[0] // choose the highest bid, which is index 0

	if header.Header == nil || (header.Header.ParentHash != parentHash) {
		log.Debug(badHeaderMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

	bid := types.BuilderBid{
		Header: header.Header,
		Value:  header.Trace.Value,
		Pubkey: rs.config.PubKey,
	}

	signature, err := types.SignMessage(&bid, rs.builderSigningDomain, rs.config.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("internal server error")
	}

	response := &types.GetHeaderResponse{
		Version: "bellatrix",
		Data:    &types.SignedBuilderBid{Message: &bid, Signature: signature},
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"bidValue":         bid.Value.String(),
		"blockHash":        bid.Header.BlockHash.String(),
		"feeRecipient":     bid.Header.FeeRecipient.String(),
		"slot":             slot,
	}).Trace("bid sent")

	return response, nil
}

// GetPayload is called by a block proposer communicating through mev-boost and reveals execution payload of given signed beacon block if stored
func (rs *DefaultRelay) GetPayload(ctx context.Context, payloadRequest *types.SignedBlindedBeaconBlock, state State) (*types.GetPayloadResponse, error) {
	logger := rs.Log().WithField("method", "GetPayload")
	timeStart := time.Now()

	if len(payloadRequest.Signature) != 96 {
		return nil, fmt.Errorf("invalid signature")
	}

	proposerPubkey, err := state.Beacon().KnownValidatorByIndex(payloadRequest.Message.ProposerIndex)
	if err != nil && errors.Is(err, ErrUnknownValue) {
		return nil, fmt.Errorf("unknown validator for index %d", payloadRequest.Message.ProposerIndex)
	} else if err != nil {
		return nil, err
	}

	pk, err := types.HexToPubkey(proposerPubkey.String())
	if err != nil {
		return nil, err
	}
	logger.With(log.F{
		"slot":      payloadRequest.Message.Slot,
		"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		"pubkey":    pk,
	}).Debug("payload requested")

	ok, err := types.VerifySignature(
		payloadRequest.Message,
		rs.proposerSigningDomain,
		pk[:],
		payloadRequest.Signature[:],
	)
	if !ok || err != nil {
		logger.WithField(
			"pubkey", proposerPubkey,
		).Error("signature invalid")
		return nil, fmt.Errorf("signature invalid")
	}

	key := PayloadKey{
		BlockHash: payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  pk,
		Slot:      structs.Slot(payloadRequest.Message.Slot),
	}

	payload, err := state.Datastore().GetPayload(ctx, key)
	if err != nil || payload == nil {
		logger.WithError(err).With(log.F{
			"pubkey":    pk,
			"slot":      payloadRequest.Message.Slot,
			"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		}).Error("no payload found")
		return nil, ErrNoPayloadFound
	}

	logger.With(log.F{
		"slot":         payloadRequest.Message.Slot,
		"blockHash":    payload.Payload.Data.BlockHash,
		"blockNumber":  payload.Payload.Data.BlockNumber,
		"stateRoot":    payload.Payload.Data.StateRoot,
		"feeRecipient": payload.Payload.Data.FeeRecipient,
		"bid":          payload.Bid.Data.Message.Value,
		"numTx":        len(payload.Payload.Data.Transactions),
	}).Info("payload fetched")

	response := types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    payload.Payload.Data,
	}

	trace := DeliveredTrace{
		Trace: BidTraceWithTimestamp{
			BidTraceExtended: BidTraceExtended{
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

	if err := state.Datastore().PutDelivered(ctx, structs.Slot(payloadRequest.Message.Slot), trace, rs.config.TTL); err != nil {
		rs.Log().WithError(err).Warn("failed to set payload after delivery")
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"slot":             payloadRequest.Message.Slot,
		"blockHash":        payload.Payload.Data.BlockHash,
		"bid":              payload.Bid.Data.Message.Value,
	}).Trace("payload sent")

	return &response, nil
}

// ***** Relay Domain *****

// SubmitBlockRequestToSignedBuilderBid converts a builders block submission to a bid compatible with mev-boost
func SubmitBlockRequestToSignedBuilderBid(req *types.BuilderSubmitBlockRequest, sk *bls.SecretKey, pubkey *types.PublicKey, domain types.Domain) (*types.SignedBuilderBid, error) {
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
func (rs *DefaultRelay) SubmitBlock(ctx context.Context, submitBlockRequest *types.BuilderSubmitBlockRequest, state State) error {
	timeStart := time.Now()

	logger := rs.Log().With(log.F{
		"method":    "SubmitBlock",
		"builder":   submitBlockRequest.Message.BuilderPubkey,
		"blockHash": submitBlockRequest.ExecutionPayload.BlockHash,
		"slot":      submitBlockRequest.Message.Slot,
		"proposer":  submitBlockRequest.Message.ProposerPubkey,
		"bid":       submitBlockRequest.Message.Value.String(),
	})

	logger.Trace("block submission requested")

	_, err := rs.verifyBlock(submitBlockRequest, state)
	if err != nil {
		logger.WithError(err).
			WithField("slot", submitBlockRequest.Message.Slot).
			WithField("builder", submitBlockRequest.Message.BuilderPubkey).
			Debug("block verification failed")
		return fmt.Errorf("verify block: %w", err)
	}

	signedBuilderBid, err := SubmitBlockRequestToSignedBuilderBid(
		submitBlockRequest,
		rs.config.SecretKey,
		&rs.config.PubKey,
		rs.builderSigningDomain,
	)

	if err != nil {
		logger.WithError(err).
			With(log.F{
				"slot":    submitBlockRequest.Message.Slot,
				"builder": submitBlockRequest.Message.BuilderPubkey,
			}).Debug("signature failed")

		return fmt.Errorf("block submission failed: %w", err)
	}

	slot := structs.Slot(submitBlockRequest.Message.Slot)

	_, err = state.Datastore().GetDelivered(ctx, Query{Slot: slot})
	if err == nil {
		logger.Debug("block submission after payload delivered")
		return errors.New("the slot payload was already delivered")
	}

	payload := SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid, submitBlockRequest)

	if err := state.Datastore().PutPayload(ctx, SubmissionToKey(submitBlockRequest), &payload, rs.config.TTL); err != nil {
		return err
	}

	header, err := types.PayloadToPayloadHeader(submitBlockRequest.ExecutionPayload)
	if err != nil {
		return err
	}

	h := HeaderAndTrace{
		Header: header,
		Trace: &BidTraceWithTimestamp{
			BidTraceExtended: BidTraceExtended{
				BidTrace: types.BidTrace{
					Slot:                 submitBlockRequest.Message.Slot,
					ParentHash:           payload.Payload.Data.ParentHash,
					BlockHash:            payload.Payload.Data.BlockHash,
					BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
					ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
					ProposerFeeRecipient: payload.Trace.Message.ProposerFeeRecipient,
					Value:                submitBlockRequest.Message.Value,
					GasLimit:             payload.Trace.Message.GasLimit,
					GasUsed:              payload.Trace.Message.GasUsed,
				},
				BlockNumber: payload.Payload.Data.BlockNumber,
				NumTx:       uint64(len(payload.Payload.Data.Transactions)),
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
	}

	err = state.Datastore().PutHeader(ctx, slot, h, rs.config.TTL)
	if err != nil {
		logger.WithError(err).Error("PutHeader failed")
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
	}).Trace("builder block stored")

	return nil
}

// GetValidators returns a list of registered block proposers in current and next epoch
func (rs *DefaultRelay) GetValidators(state State) BuilderGetValidatorsResponseEntrySlice {
	log := rs.Log().WithField("method", "GetValidators")
	validators := state.Beacon().ValidatorsMap()
	log.With(validators).Debug("validatored map sent")
	return validators
}

func (rs *DefaultRelay) verifyBlock(submitRequest *types.BuilderSubmitBlockRequest, state State) (bool, error) {
	if submitRequest == nil {
		return false, fmt.Errorf("block empty")
	}

	_ = simulateBlock()

	expectedTimestamp := state.Beacon().Genesis().GenesisTime + (submitRequest.Message.Slot * 12)
	if submitRequest.ExecutionPayload.Timestamp != expectedTimestamp {
		return false, fmt.Errorf("builder submission with wrong timestamp. got %d, expected %d", submitRequest.ExecutionPayload.Timestamp, expectedTimestamp)
	}

	return types.VerifySignature(submitRequest.Message, rs.builderSigningDomain, submitRequest.Message.BuilderPubkey[:], submitRequest.Signature[:])
}

func simulateBlock() bool {
	// TODO : Simulate block here once support for external builders
	// we currently only support a single internally trusted builder
	return true
}

func SubmissionToKey(submission *types.BuilderSubmitBlockRequest) PayloadKey {
	return PayloadKey{
		BlockHash: submission.ExecutionPayload.BlockHash,
		Proposer:  submission.Message.ProposerPubkey,
		Slot:      structs.Slot(submission.Message.Slot),
	}
}
