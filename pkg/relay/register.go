package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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

const (
	ResponseQueueSubmit = iota
	ResponseQueueRegister
	ResponseQueueOther
)

// returnChannelSize is the size of the buffer
// describing the queue of results before it would be processed by registerSync
const returnChannelSize = 150_000

var retChannPool = sync.Pool{
	New: func() any {
		return make(chan SVRReqResp, returnChannelSize)
	},
}

var singleRetChannPool = sync.Pool{
	New: func() any {
		return make(chan SVRReqResp, 1)
	},
}

type Setter interface {
	Set(k string, value uint64)
}

type Getter interface {
	Get(k string) (value uint64, ok bool)
}

type SVRReq struct {
	Signature [96]byte
	Pubkey    [48]byte

	Msg      [32]byte
	Iter     int
	Response chan SVRReqResp
}
type SVRStoreReq struct {
	RawPayload json.RawMessage
	Pubkey     types.PublicKey

	Iter     int
	Response chan SVRReqResp
}

type SVRReqResp struct {
	Type   int8
	Err    error
	Commit bool
	Iter   int
}

func registerSync(s RegistrationManager, in chan SVRReqResp, failure chan struct{}, exit chan error, payload []structs.SignedValidatorRegistration, sentVerified *uint32) {
	var numVerify, numOthers, stored, sentToStore uint32
	var total = uint32(len(payload))

	storeCh := s.StoreChan()
	rcv := make(map[int]struct{})

	var (
		item SVRReqResp
		err  error
	)

	for item = range in {
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
				close(failure)
			}
		} else if storeIfReady(s, rcv, item.Iter) {
			p := payload[item.Iter]
			s.Set(p.Message.Pubkey.String(), p.Message.Timestamp)
			storeCh <- SVRStoreReq{
				Pubkey:     p.Message.Pubkey,
				Iter:       item.Iter,
				RawPayload: p.Raw,
				Response:   in}

			sentToStore++
		}
		if numVerify == atomic.LoadUint32(sentVerified) && numOthers == total && sentToStore == stored {
			break
		}
	}

	if err == nil {
		close(failure)
	}
	exit <- err
	close(exit)
}

func storeIfReady(s Setter, rcv map[int]struct{}, iter int) bool {
	// store only if it has two records from both checks
	_, ok := rcv[iter]
	if !ok {
		rcv[iter] = struct{}{}
		return false
	}
	delete(rcv, iter)
	return true
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Relay) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) error {
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidator", "all"))
	defer timer.ObserveDuration()

	// This function is limitted to the size of the buffer to prevent deadlocks
	if len(payload) >= returnChannelSize/3-1 {
		return fmt.Errorf("total number of validators exceeded: %d ", returnChannelSize/3-1)
	}

	respCh := retChannPool.Get().(chan SVRReqResp)
	defer retChannPool.Put(respCh)

	failure := make(chan struct{}, 1)
	exit := make(chan error, 1)

	sentVerified := uint32(0)
	go registerSync(rs.regMngr, respCh, failure, exit, payload, &sentVerified)
	go checkInMem(rs.beaconState, rs.regMngr, payload, respCh)
	var failed bool
	VInp := rs.regMngr.VerifyChanStacks(ResponseQueueRegister)
	for i, p := range payload {
		if failed { // after failure just populate the errors
			atomic.AddUint32(&sentVerified, 1)
			respCh <- SVRReqResp{Iter: i, Err: errors.New("failed"), Type: ResponseTypeVerify}
			break
		}

		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.config.BuilderSigningDomain)
		if err != nil {
			atomic.AddUint32(&sentVerified, 1)
			respCh <- SVRReqResp{Iter: i, Err: errors.New("invalid signature"), Type: ResponseTypeVerify}
			failed = true
			break
		}

		select {
		case <-failure:
			failed = true
		case VInp <- SVRReq{
			Signature: p.Signature,
			Pubkey:    p.Message.Pubkey,
			Msg:       msg,
			Iter:      i,
			Response:  respCh}:
			atomic.AddUint32(&sentVerified, 1)
		}
	}

	return <-exit
}

func checkInMem(state State, getter Getter, payload []structs.SignedValidatorRegistration, out chan SVRReqResp) {
	beacon := state.Beacon()
	for i, sp := range payload {
		p, ok := checkOneInMem(beacon, getter, i, sp)
		out <- p
		if !ok {
			return
		}
	}
}

func checkOneInMem(beacon *structs.BeaconState, getter Getter, i int, sp structs.SignedValidatorRegistration) (svresp SVRReqResp, ok bool) {
	if verifyTimestamp(sp.Message.Timestamp) {
		return SVRReqResp{Commit: false, Iter: i, Err: fmt.Errorf("request too far in future for %s", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}, false
	}

	pk := structs.PubKey{PublicKey: sp.Message.Pubkey}
	known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
	if !known {
		return SVRReqResp{Commit: false, Iter: i, Err: fmt.Errorf("%s not a known validator", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}, false
	}

	previousValidatorTimestamp, ok := getter.Get(pk.String()) // Do not error on this
	return SVRReqResp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), Iter: i, Type: ResponseTypeOthers}, true
}

func (rs *Relay) RegisterValidatorSingular(ctx context.Context, payload structs.SignedValidatorRegistration) error {
	timer := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidatorSingular", "all"))
	defer timer.ObserveDuration()

	otherChecks, _ := checkOneInMem(rs.beaconState.Beacon(), rs.regMngr, 0, payload)
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

	respCh := singleRetChannPool.Get().(chan SVRReqResp)
	defer singleRetChannPool.Put(respCh)

	rs.regMngr.VerifyChanStacks(ResponseQueueRegister) <- SVRReq{
		Signature: payload.Signature,
		Pubkey:    payload.Message.Pubkey,
		Msg:       msg,
		Iter:      0,
		Response:  respCh,
	}
	r := <-respCh
	timer2.ObserveDuration()

	if r.Err != nil {
		return errors.New("invalid signature")
	}

	timer3 := prometheus.NewTimer(rs.m.Timing.WithLabelValues("registerValidatorSingular", "store"))
	storeCh := rs.regMngr.StoreChan()
	rs.regMngr.Set(payload.Message.Pubkey.String(), payload.Message.Timestamp)
	storeCh <- SVRStoreReq{
		Pubkey:     payload.Message.Pubkey,
		Iter:       0,
		RawPayload: payload.Raw,
		Response:   respCh}

	r = <-respCh
	timer3.ObserveDuration()

	return r.Err
}
