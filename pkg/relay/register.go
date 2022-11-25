package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Response *RespC
}

// StoreReq is similar to VerifyReq jsut for storing payloads
type StoreReq struct {
	RawPayload json.RawMessage

	Time   uint64
	Pubkey types.PublicKey
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
func (rs *Relay) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) error {
	logger := rs.l.WithField("method", "RegisterValidator")
	timeStart := time.Now()

	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "all"))
	defer timer.ObserveDuration()

	be := rs.beaconState.Beacon()
	verifyChan := rs.regMngr.GetVerifyChan(ResponseQueueRegister)

	respChA := NewRespC(len(payload))
SendPayloads:
	for i, p := range payload {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			respChA.Close(0, err)
			return err
		default:
		}
		o, ok := verifyOther(be, rs.regMngr, i, p)
		if !ok {
			respChA.Close(i, o.Err)
			break SendPayloads
		}

		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.config.BuilderSigningDomain)
		if err != nil {
			respChA.Close(i, errors.New("invalid signature"))
			break SendPayloads
		}

		verifyChan <- VerifyReq{
			Signature: p.Signature,
			Pubkey:    p.Message.Pubkey,
			Msg:       msg,
			ID:        i,
			Response:  respChA}
	}
	var err error
	select {
	case err = <-respChA.Done():
	case <-ctx.Done():
		err := ctx.Err()
		respChA.Close(0, err)
		return err
	}

	if si := respChA.SuccessfullIndexes(); len(si) > 0 {
		sReq := SReq{ReqS: make([]StoreReq, len(si))}
		for _, i := range si {
			p := payload[i]
			sReq.ReqS = append(sReq.ReqS, StoreReq{
				Time:       p.Message.Timestamp,
				Pubkey:     p.Message.Pubkey,
				RawPayload: p.Raw,
			})
		}
		rs.regMngr.SendStore(sReq)
	}

	if err == nil {
		logger.
			WithField("processingTimeMs", time.Since(timeStart).Milliseconds()).
			WithField("numberValidators", len(payload)).
			Trace("validator registrations succeeded")
	}

	return err
}

func checkStoragePossibility(syncMap map[int]struct{}, id int) bool {
	// it's possible only if it has two records from both checks
	_, ok := syncMap[id]
	if !ok {
		syncMap[id] = struct{}{}
		return false
	}
	delete(syncMap, id)
	return true
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

/*
func verifyOthers(state State, tsReg TimestampRegistry, fc *FlowControl, payload []structs.SignedValidatorRegistration) {
	beacon := state.Beacon()
	for i, sp := range payload {
		p, ok := verifyOther(beacon, tsReg, i, sp)
		fc.RespCh <- p
		if !ok {
			return
		}
	}
}*/
