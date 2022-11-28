package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	ResponseTypeVerify = iota
	ResponseTypeOthers
	ResponseTypeStored
)

type TimestampRegistry interface {
	Get(pubkey string) (timestamp uint64, ok bool)
}

// VerifyReq is a request structure used in communication
// between api calls and fixed set of worker goroutines
// it's using return channel pattern, meaning that after
// sent the sender locks on that channel to get the response
type VerifyReq struct {
	Signature [96]byte
	Pubkey    [48]byte
	Msg       [32]byte
	// Unique identifier of payload
	// if needed to be passed back in response
	ID       int
	Response *StoreResp
}

// StoreReqItem is similar to VerifyReq jsut for storing payloads
type StoreReqItem struct {
	RawPayload json.RawMessage
	Time       uint64
	Pubkey     types.PublicKey
}

// Resp respone structure
// - potential candidate for structure pool
// as it's almost constant size
type Resp struct {
	ID     int
	Type   int8
	Commit bool
	Err    error
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Relay) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) (err error) {
	logger := rs.l.WithField("method", "RegisterValidator")

	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "all"))
	defer timer.ObserveDuration()

	be := rs.beaconState.Beacon()
	verifyChan := rs.regMngr.GetVerifyChan(ResponseQueueRegister)

	response := NewRespC(len(payload))

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

		checkTime := time.Now()
		o, ok := verifyOther(be, rs.regMngr, i, p)
		if !ok {
			response.Close(i, o.Err)
			break SendPayloads
		}
		totalCheckTime += time.Since(checkTime)

		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.config.BuilderSigningDomain)
		if err != nil {
			response.Close(i, errors.New("invalid signature"))
			break SendPayloads
		}

		verifyChan <- VerifyReq{
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
	rs.m.Timing.WithLabelValues("registerValidator", "verify").Observe(math.Abs(processTime.Seconds() - totalCheckTime.Seconds()))
	rs.m.Timing.WithLabelValues("registerValidator", "check").Observe(totalCheckTime.Seconds())

	if si := response.SuccessfullIndexes(); len(si) > 0 {
		timerStore := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "asyncStore"))
		request := StoreReq{Items: make([]StoreReqItem, len(si))}
		for nextIter, i := range si {
			p := payload[i]
			request.Items[nextIter] = StoreReqItem{
				Time:       p.Message.Timestamp,
				Pubkey:     p.Message.Pubkey,
				RawPayload: p.Raw,
			}
		}
		rs.regMngr.SendStore(request)
		timerStore.ObserveDuration()
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

func verifyOther(beacon *structs.BeaconState, tsReg TimestampRegistry, i int, sp structs.SignedValidatorRegistration) (svresp Resp, ok bool) {
	if verifyTimestamp(sp.Message.Timestamp) {
		return Resp{Commit: false, ID: i, Err: fmt.Errorf("request too far in future for %s", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}, false
	}

	pk := structs.PubKey{PublicKey: sp.Message.Pubkey}
	known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
	if !known {
		return Resp{Commit: false, ID: i, Err: fmt.Errorf("%s not a known validator", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}, false
	}

	previousValidatorTimestamp, ok := tsReg.Get(pk.String()) // Do not error on this
	return Resp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), ID: i, Type: ResponseTypeOthers}, true
}
