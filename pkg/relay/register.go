package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	ResponseTypeVerify = iota
	ResponseTypeOthers
	ResponseTypeStored
)

type Setter interface {
	Set(k string, value uint64)
}

type Getter interface {
	Get(k string) (value uint64, ok bool)
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
	Response chan Resp
}

// StoreReq is similar to VerifyReq jsut for storing payloads
type StoreReq struct {
	RawPayload json.RawMessage
	Pubkey     types.PublicKey
	ID         int
	Response   chan Resp
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
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "all"))
	defer timer.ObserveDuration()

	var respCh chan Resp

	// This function is limitted to the size of the buffer to prevent deadlocks
	if rs.config.RegisterValidatorMaxNum > uint64(len(payload)) {
		respCh = rs.retChannPool.Get().(chan Resp)
		defer rs.retChannPool.Put(respCh)
	}

	fc := NewFlowControl(respCh, len(payload))
	defer fc.Close()

	// check other parameters in separate goroutine
	go checkOthers(rs.beaconState, rs.regMngr, fc, payload)

	// synchronize responses from:
	//	- verification of signatures
	//	- other checks of payload validity
	//	- successfull store
	go synchronizeResponses(rs.regMngr, fc, payload)

	verifyChan := rs.regMngr.GetVerifyChan(ResponseQueueRegister)
SendPayloads:
	for i, p := range payload {
		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.config.BuilderSigningDomain)
		if err != nil {
			fc.SentVerificationInc()
			fc.RespCh <- Resp{ID: i, Err: errors.New("invalid signature"), Type: ResponseTypeVerify}
			break SendPayloads
		}

		select {
		case <-fc.FailureCh:
			break SendPayloads
		case verifyChan <- VerifyReq{
			Signature: p.Signature,
			Pubkey:    p.Message.Pubkey,
			Msg:       msg,
			ID:        i,
			Response:  fc.RespCh}:
			fc.SentVerificationInc()
		}
	}
	return <-fc.ExitCh
}

func (rs *Relay) RegisterValidatorSingular(ctx context.Context, payload structs.SignedValidatorRegistration) error {
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidatorSingular", "all"))
	defer timer.ObserveDuration()

	otherChecks, _ := checkOther(rs.beaconState.Beacon(), rs.regMngr, 0, payload)
	if otherChecks.Err != nil {
		return otherChecks.Err
	}
	if !otherChecks.Commit { // when timestamp is wrong so we don't commit
		return nil
	}

	timer2 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidatorSingular", "verify"))
	msg, err := types.ComputeSigningRoot(payload.Message, rs.config.BuilderSigningDomain)
	if err != nil {
		return errors.New("invalid signature")
	}

	respCh := rs.singleRetChannPool.Get().(chan Resp)
	defer rs.singleRetChannPool.Put(respCh)

	rs.regMngr.GetVerifyChan(ResponseQueueRegister) <- VerifyReq{
		Signature: payload.Signature,
		Pubkey:    payload.Message.Pubkey,
		Msg:       msg,
		Response:  respCh,
	}
	r := <-respCh
	timer2.ObserveDuration()

	if r.Err != nil {
		return errors.New("invalid signature")
	}

	timer3 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidatorSingular", "store"))
	storeCh := rs.regMngr.GetStoreChan()
	rs.regMngr.Set(payload.Message.Pubkey.String(), payload.Message.Timestamp)
	storeCh <- StoreReq{
		Pubkey:     payload.Message.Pubkey,
		RawPayload: payload.Raw,
		Response:   respCh}

	r = <-respCh
	timer3.ObserveDuration()

	return r.Err
}

func synchronizeResponses(s RegistrationManager, fc *FlowControl, payload []structs.SignedValidatorRegistration) {
	var numVerify, numOthers, stored, sentToStore uint32
	var total = uint32(len(payload))

	storeCh := s.GetStoreChan()
	syncMap := make(map[int]struct{})

	var (
		item Resp
		err  error
	)

	for item = range fc.RespCh {
		switch item.Type {
		case ResponseTypeVerify:
			numVerify++
		case ResponseTypeOthers:
			numOthers++
		case ResponseTypeStored:
			stored++
		}

		if item.Err != nil {
			if err == nil { // if there wasn't any error before
				if item.Type == ResponseTypeOthers {
					numOthers = total // this is terminator, so no event will come from here after
				}
				err = item.Err
				fc.Fail()
			}
		} else if checkStoragePossibility(syncMap, item.ID) {
			p := payload[item.ID]
			s.Set(p.Message.Pubkey.String(), p.Message.Timestamp)
			storeCh <- StoreReq{
				Pubkey:     p.Message.Pubkey,
				ID:         item.ID,
				RawPayload: p.Raw,
				Response:   fc.RespCh}
			sentToStore++
		}

		if numVerify == fc.SentVerifications() && numOthers == total && sentToStore == stored {
			break
		}
	}

	fc.Exit(err)
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

func checkOthers(state State, getter Getter, fc *FlowControl, payload []structs.SignedValidatorRegistration) {
	beacon := state.Beacon()
	for i, sp := range payload {
		p, ok := checkOther(beacon, getter, i, sp)
		fc.RespCh <- p
		if !ok {
			return
		}
	}
}

func checkOther(beacon *structs.BeaconState, getter Getter, i int, sp structs.SignedValidatorRegistration) (svresp Resp, ok bool) {
	if verifyTimestamp(sp.Message.Timestamp) {
		return Resp{Commit: false, ID: i, Err: fmt.Errorf("request too far in future for %s", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}, false
	}

	pk := structs.PubKey{PublicKey: sp.Message.Pubkey}
	known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
	if !known {
		return Resp{Commit: false, ID: i, Err: fmt.Errorf("%s not a known validator", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}, false
	}

	previousValidatorTimestamp, ok := getter.Get(pk.String()) // Do not error on this
	return Resp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), ID: i, Type: ResponseTypeOthers}, true
}

type FlowControl struct {
	RespCh   chan Resp
	isPooled bool

	FailureCh chan struct{}
	ExitCh    chan error

	sentToVerification uint32
	failed             bool
}

func NewFlowControl(respCh chan Resp, numElements int) (fc *FlowControl) {

	if respCh == nil {
		respCh = make(chan Resp, numElements*3)
		fc.isPooled = true
	}

	return &FlowControl{
		RespCh:             respCh,
		FailureCh:          make(chan struct{}),
		ExitCh:             make(chan error),
		sentToVerification: 0,
	}
}

func (fc *FlowControl) Close() {
	if !fc.isPooled {
		close(fc.RespCh)
	}
}

func (fc *FlowControl) SentVerificationInc() {
	atomic.AddUint32(&fc.sentToVerification, 1)
}

func (fc *FlowControl) SentVerifications() uint32 {
	return atomic.LoadUint32(&fc.sentToVerification)
}

func (fc *FlowControl) Fail() {
	if !fc.failed {
		close(fc.FailureCh)
	}
}

func (fc *FlowControl) Exit(err error) {
	if !fc.failed {
		close(fc.FailureCh)
	}

	fc.ExitCh <- err
	close(fc.ExitCh)
}
