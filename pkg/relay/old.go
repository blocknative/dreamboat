package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"golang.org/x/sync/errgroup"
)

// ***** Builder Domain *****

// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Relay) OLDRegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) error {
	logger := rs.l.WithField("method", "RegisterValidator")
	timeStart := time.Now()

	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < runtime.NumCPU(); i++ {
		start := i * (len(payload) / runtime.NumCPU())
		end := (i + 1) * (len(payload) / runtime.NumCPU())
		if i == runtime.NumCPU()-1 {
			end = len(payload)
		}
		g.Go(func() error {
			return rs.OLDprocessValidator(ctx, payload[start:end], rs.beaconState)
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

func (rs *Relay) OLDprocessValidator(ctx context.Context, payload []structs.SignedValidatorRegistration, state State) error {
	logger := rs.l.WithField("method", "RegisterValidator")
	timeStart := time.Now()

	for i := 0; i < len(payload) && ctx.Err() == nil; i++ {
		registerRequest := payload[i]
		ok, err := types.VerifySignature(
			registerRequest.Message,
			rs.config.BuilderSigningDomain,
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
		previousValidator, err := rs.d.GetRegistration(ctx, pk)
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

		data, err := json.Marshal(registerRequest.SignedValidatorRegistration)
		if err != nil {
			return err
		}
		// officially register validator
		if err := rs.d.PutRegistrationRaw(ctx, pk, data, rs.config.TTL); err != nil {
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
