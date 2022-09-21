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
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
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
	HeadSlot() Slot
	ValidatorsMap() BuilderGetValidatorsResponseEntrySlice
}

type Relay interface {
	// Proposer APIs
	RegisterValidator(context.Context, []types.SignedValidatorRegistration, State) error
	GetHeader(context.Context, HeaderRequest, State) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock, State) (*types.GetPayloadResponse, error)

	// Builder APIs
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest, State) error
	GetValidators(State) BuilderGetValidatorsResponseEntrySlice
}

type DefaultRelay struct {
	config                Config
	builderSigningDomain  types.Domain
	proposerSigningDomain types.Domain
}

// NewRelay relay service
func NewRelay(config Config) (*DefaultRelay, error) {
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

	rs := &DefaultRelay{
		config:                config,
		builderSigningDomain:  domainBuilder,
		proposerSigningDomain: domainBeaconProposer,
	}
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
func (rs *DefaultRelay) RegisterValidator(ctx context.Context, payload []types.SignedValidatorRegistration, state State) error {
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
		logger.WithError(err).Debug("validator registration failed")
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"numberValidators": len(payload),
	}).Trace("validator registrations succeeded")

	return nil
}

func (rs *DefaultRelay) processValidator(ctx context.Context, payload []types.SignedValidatorRegistration, state State) error {
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
			logger.WithError(err).Debug("signature invalid")
			return fmt.Errorf("signature invalid")
		}

		if verifyTimestamp(registerRequest.Message.Timestamp) {
			return fmt.Errorf("request too far in future")
		}

		pk := PubKey{registerRequest.Message.Pubkey}

		ok, err = state.Beacon().IsKnownValidator(pk.PubkeyHex())
		if err != nil {
			return err
		} else if !ok {
			if rs.config.CheckKnownValidator {
				return fmt.Errorf("not a validator")
			} else {
				logger.WithField("pubkey", pk.PublicKey).Debug("is not a known validator")
			}
		}

		// check previous validator registration
		previousValidator, err := state.Datastore().GetRegistration(ctx, pk)
		if err != nil && !errors.Is(err, ds.ErrNotFound) {
			log.Warn(err)
		}

		if err == nil {
			// skip registration if
			if registerRequest.Message.Timestamp < previousValidator.Message.Timestamp {
				rs.Log().Debug("request timestamp less than previous")
				continue
			}

			if registerRequest.Message.Timestamp == previousValidator.Message.Timestamp &&
				(registerRequest.Message.FeeRecipient != previousValidator.Message.FeeRecipient ||
					registerRequest.Message.GasLimit != previousValidator.Message.GasLimit) {
				// to help debug issues with validator set ups
				rs.Log().With(log.F{
					"prevFeeRecipient":    previousValidator.Message.FeeRecipient,
					"requestFeeRecipient": registerRequest.Message.FeeRecipient,
					"prevGasLimit":        previousValidator.Message.GasLimit,
					"requestGasLimit":     registerRequest.Message.GasLimit,
				}).Debug("different registration fields")
			}
		}

		// officially register validator
		if err := state.Datastore().PutRegistration(ctx, pk, registerRequest, rs.config.TTL); err != nil {
			rs.Log().WithError(err).Debug("Error in PutRegistration")
			return err
		}

		logger.WithField("pubkey", pk).Trace("validator registered")
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"numberValidators": len(payload),
	}).Trace("validator batch registered")

	return nil
}

// GetHeader is called by a block proposer communicating through mev-boost and returns a bid along with an execution payload header
func (rs *DefaultRelay) GetHeader(ctx context.Context, request HeaderRequest, state State) (*types.GetHeaderResponse, error) {
	logger := rs.Log().WithField("method", "GetHeader")
	timeStart := time.Now()

	slot, err := request.Slot()
	if err != nil {
		return nil, err
	}

	parentHash, err := request.parentHash()
	if err != nil {
		return nil, err
	}

	pk, err := request.pubkey()
	if err != nil {
		return nil, err
	}

	logger.With(log.F{
		"slot":       slot,
		"parentHash": parentHash,
		"pubkey":     pk,
	}).Debug("header requested")

	vd, err := state.Datastore().GetRegistration(ctx, pk)
	if err != nil {
		logger.Warn("unregistered validator")
		return nil, fmt.Errorf(noBuilderBidMsg)
	}
	if vd.Message.Pubkey != pk.PublicKey {
		logger.Warn("registration and request pubkey mismatch")
		return nil, fmt.Errorf("unknown validator")
	}

	header, err := state.Datastore().GetHeader(ctx, Query{Slot: slot})
	if err != nil {
		log.Warn(noBuilderBidMsg)
		return nil, fmt.Errorf(noBuilderBidMsg)
	}

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
	rs.Log().WithFields(logrus.Fields{
		"slot":      payloadRequest.Message.Slot,
		"pubKey":    proposerPubkey,
		"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		"proposer":  pk,
	}).Debug("payload requested")

	ok, err := types.VerifySignature(
		payloadRequest.Message,
		rs.proposerSigningDomain,
		pk[:],
		payloadRequest.Signature[:],
	)
	if !ok || err != nil {
		rs.Log().WithField(
			"pubKey", proposerPubkey,
		).Error("signature invalid")
		return nil, fmt.Errorf("signature invalid")
	}

	key := PayloadKey{
		BlockHash: payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		Proposer:  pk,
		Slot:      Slot(payloadRequest.Message.Slot),
	}

	payload, err := state.Datastore().GetPayload(ctx, key)
	if err != nil || payload == nil {
		logger.With(log.F{
			"slot":      payloadRequest.Message.Slot,
			"blockHash": payloadRequest.Message.Body.ExecutionPayloadHeader.BlockHash,
		}).WithError(err).Error("no payload found")
		return nil, ErrNoPayloadFound
	}

	logger.With(log.F{
		"slot":         payloadRequest.Message.Slot,
		"blockHash":    payload.Payload.Data.BlockHash,
		"blockNumber":  payload.Payload.Data.BlockNumber,
		"stateRoot":    payload.Payload.Data.StateRoot,
		"feeRecipient": payload.Payload.Data.FeeRecipient,
	}).Info("payload fetched")

	response := types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    payload.Payload.Data,
	}

	trace := DeliveredTrace{
		Trace: BidTraceWithTimestamp{
			BidTrace: types.BidTrace{
				Slot:                 payloadRequest.Message.Slot,
				ParentHash:           payload.Payload.Data.ParentHash,
				BlockHash:            payload.Payload.Data.BlockHash,
				BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
				ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
				ProposerFeeRecipient: payload.Payload.Data.FeeRecipient,
				Value:                payload.Trace.Message.Value,
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
		BlockNumber: payload.Payload.Data.BlockNumber,
	}

	if err := state.Datastore().PutDelivered(ctx, Slot(payloadRequest.Message.Slot), trace, rs.config.TTL); err != nil {
		rs.Log().WithError(err).Warn("failed to set payload after delivery")
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"slot":             payloadRequest.Message.Slot,
		"blockHash":        payload.Payload.Data.BlockHash,
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
	logger := rs.Log().WithField("method", "SubmitBlock")
	timeStart := time.Now()

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"slot":             submitBlockRequest.Message.Slot,
		"blockHash":        submitBlockRequest.ExecutionPayload.BlockHash,
		"proposer":         submitBlockRequest.Message.ProposerPubkey,
	}).Trace("block submission requested")

	_, err := rs.verifyBlock(submitBlockRequest)
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
			BidTrace: types.BidTrace{
				Slot:                 submitBlockRequest.Message.Slot,
				ParentHash:           payload.Payload.Data.ParentHash,
				BlockHash:            payload.Payload.Data.BlockHash,
				BuilderPubkey:        payload.Trace.Message.BuilderPubkey,
				ProposerPubkey:       payload.Trace.Message.ProposerPubkey,
				ProposerFeeRecipient: payload.Payload.Data.FeeRecipient,
				Value:                payload.Trace.Message.Value,
			},
			Timestamp: payload.Payload.Data.Timestamp,
		},
	}

	s := Slot(submitBlockRequest.Message.Slot)

	err = state.Datastore().PutHeader(ctx, s, h, rs.config.TTL)

	if err != nil {
		logger.WithError(err).Error("PutHeader failed")
		return err
	}

	logger.With(log.F{
		"processingTimeMs": time.Since(timeStart).Milliseconds(),
		"slot":             submitBlockRequest.Message.Slot,
		"blockHash":        submitBlockRequest.ExecutionPayload.BlockHash,
		"proposer":         submitBlockRequest.Message.ProposerPubkey,
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

func (rs *DefaultRelay) verifyBlock(SubmitBlockRequest *types.BuilderSubmitBlockRequest) (bool, error) {
	if SubmitBlockRequest == nil {
		return false, fmt.Errorf("block empty")
	}

	_ = simulateBlock()

	return types.VerifySignature(SubmitBlockRequest.Message, rs.builderSigningDomain, SubmitBlockRequest.Message.BuilderPubkey[:], SubmitBlockRequest.Signature[:])
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
		Slot:      Slot(submission.Message.Slot),
	}
}
