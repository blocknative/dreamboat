//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg/validators RegistrationManager,Verifier,State
package validators

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/blocknative/dreamboat/pkg/verify"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrUnknownValidator = errors.New("unknown validator")
	ErrInvalidTimestamp = errors.New("invalid timestamp")
)

type State interface {
	Beacon() *structs.BeaconState
}

type Verifier interface {
	GetVerifyChan(stack uint) chan verify.Request
}

type RegistrationManager interface {
	SendStore(sReq StoreReq)
	GetRegistration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error)
	Check(rvg *types.RegisterValidatorRequestMessage) bool
}

type Register struct {
	regMngr     RegistrationManager
	ver         Verifier
	beaconState State

	builderSigningDomain types.Domain

	l log.Logger
	m RegisterMetrics
}

func NewRegister(l log.Logger, builderSigningDomain types.Domain, beaconState State, ver Verifier, regMngr RegistrationManager) *Register {
	reg := &Register{
		l:                    l,
		ver:                  ver,
		regMngr:              regMngr,
		builderSigningDomain: builderSigningDomain,
		beaconState:          beaconState,
	}
	reg.initMetrics()
	return reg
}

func (r *Register) Registration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	return r.regMngr.GetRegistration(ctx, pk)
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Register) RegisterValidator(ctx context.Context, m *structs.MetricGroup, payload []types.SignedValidatorRegistration) (err error) {
	logger := rs.l.WithField("method", "RegisterValidator")

	tStart := time.Now()
	defer m.AppendSince(tStart, "registerValidator", "all")

	be := rs.beaconState.Beacon()

	verifyChan := rs.ver.GetVerifyChan(verify.ResponseQueueRegister)
	response := verify.NewRespC(len(payload))

	timeStart := time.Now()

	var totalCheckTime time.Duration
SendPayloads:
	for i, p := range payload {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			response.Close(0, err)
			return err
		default:
		}

		if rs.regMngr.Check(p.Message) {
			response.SkipOne()
			rs.m.RegistrationsCacheHits.WithLabelValues("hit").Inc()
			continue SendPayloads
		}
		rs.m.RegistrationsCacheHits.WithLabelValues("miss").Inc()

		checkTime := time.Now()
		o, ok := verifyOther(be, rs.regMngr, i, p)
		if !ok {
			response.Close(i, o.Err)
			break SendPayloads
		}
		totalCheckTime += time.Since(checkTime)

		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.builderSigningDomain)
		if err != nil {
			response.Close(i, errors.New("invalid signature"))
			break SendPayloads
		}

		verifyChan <- verify.Request{
			Signature: p.Signature,
			Pubkey:    p.Message.Pubkey,
			Msg:       msg,
			ID:        i,
			Response:  response}
	}

	select {
	case <-response.Done():
	case <-ctx.Done():
		err := ctx.Err()
		response.Close(0, err)
		return err
	}
	processTime := time.Since(timeStart)
	m.Append(processTime-totalCheckTime, "registerValidator", "verify")
	m.Append(totalCheckTime, "registerValidator", "check")

	if si := response.SuccessfullIndexes(); len(si) > 0 {
		tStore := time.Now()
		request := StoreReq{Items: make([]StoreReqItem, len(si))}
		for nextIter, i := range si {
			p := payload[i]
			request.Items[nextIter] = StoreReqItem{
				Time:    p.Message.Timestamp,
				Payload: p,
			}
		}
		rs.regMngr.SendStore(request)
		m.AppendSince(tStore, "registerValidator", "asyncStore")
	}

	err = response.Error()
	if err == nil {
		logger.
			WithField("processingTimeMs", time.Since(timeStart).Milliseconds()).
			WithField("numberValidators", len(payload)).
			Trace("validator registrations succeeded")
	}

	return err
}

func verifyOther(beacon *structs.BeaconState, tsReg RegistrationManager, i int, sp types.SignedValidatorRegistration) (svresp verify.Resp, ok bool) {
	if sp.Message.Timestamp > uint64(time.Now().Add(10*time.Second).Unix()) {
		return verify.Resp{Commit: false, ID: i, Err: fmt.Errorf("request too far in future for %s", sp.Message.Pubkey.String())}, false
	}

	pk := structs.PubKey{PublicKey: sp.Message.Pubkey}
	known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
	if !known {
		return verify.Resp{Commit: false, ID: i, Err: fmt.Errorf("%s not a known validator", sp.Message.Pubkey.String())}, false
	}

	return verify.Resp{Commit: true, ID: i}, true
}

// GetValidators returns a list of registered block proposers in current and next epoch
func (rs *Register) GetValidators(m *structs.MetricGroup) structs.BuilderGetValidatorsResponseEntrySlice {
	tStart := time.Now()
	defer m.AppendSince(tStart, "getValidators", "all")

	validators := rs.beaconState.Beacon().ValidatorsMap()
	return validators
}
