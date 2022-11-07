package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flashbots/go-boost-utils/types"
)

type RegisteredManager struct {
	M           map[string]uint64
	acc         sync.RWMutex
	VerifyInput chan SVRReq
}

func NewRegisteredManager(queue int) *RegisteredManager {
	return &RegisteredManager{
		M:           make(map[string]uint64),
		VerifyInput: make(chan SVRReq, queue),
	}
}

func (rm *RegisteredManager) RunWorkers(num int, domain types.Domain) {
	for i := 0; i < num; i++ {
		go VerifyParallel(rm.VerifyInput, domain)
	}
}

func (rm *RegisteredManager) Set(k string, value uint64) {
	rm.acc.Lock()
	defer rm.acc.Unlock()
	rm.M[k] = value
}

func (rm *RegisteredManager) Get(k string) (value uint64, ok bool) {
	rm.acc.RLock()
	defer rm.acc.RUnlock()
	value, ok = rm.M[k]
	return
}

type Setter interface {
	Set(k string, value uint64)
}

type SVRReq struct {
	SignedValidatorRegistration
	Iter     int
	Response chan SVRReqResp
}

type SVRReqResp struct {
	Err    error
	Commit bool
	Iter   int
}

func registerSync(datas Datastore, s Setter, ttl time.Duration, a, b chan SVRReqResp, failure, exit chan struct{}, payload []SignedValidatorRegistration) {
	rcv := make(map[int]struct{})

	var numA, numB int
	var errored bool
	var item SVRReqResp

	var total = len(payload)
SyncLoop:
	for {
		select {
		case item = <-a:
			numA++
			if item.Err != nil {
				if !errored {
					close(failure)
				}
			} else {
				storeIfReady(datas, s, rcv, item.Iter, payload[item.Iter], ttl)
			}

			if numA == total && numB == total {
				break SyncLoop
			}
		case item = <-b:
			numB++
			if item.Err != nil {
				if !errored {
					close(failure)
				}
			} else if item.Commit {
				storeIfReady(datas, s, rcv, item.Iter, payload[item.Iter], ttl)
			}

			if numA == total && numB == total {
				break SyncLoop
			}
		}
	}
	close(a)
	close(b)
	close(exit)
	close(failure)
}

func storeIfReady(datas Datastore, s Setter, rcv map[int]struct{}, iter int, registerRequest SignedValidatorRegistration, ttl time.Duration) error {
	// store only if it has two records from both checks
	_, ok := rcv[iter]
	if !ok {
		rcv[iter] = struct{}{}
		return nil
	}

	if err := datas.PutRegistrationRaw(context.Background(), PubKey{registerRequest.Message.Pubkey}, registerRequest.Raw, ttl); err != nil {
		return fmt.Errorf("failed to store %s", registerRequest.Message.Pubkey.String())
	}
	delete(rcv, iter)
	s.Set(registerRequest.Message.Pubkey.String(), registerRequest.Message.Timestamp)
	return nil
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *DefaultRelay) RegisterValidator2(ctx context.Context, payload []SignedValidatorRegistration, state State) error {

	/* TODO(l): Consider this
	for _, registerRequest := range payload {
		if verifyTimestamp(registerRequest.Message.Timestamp) {
			return fmt.Errorf("request too far in future for %s", registerRequest.Message.Pubkey.String())
		}
	}*/
	retSignature := make(chan SVRReqResp, 20)
	retOther := make(chan SVRReqResp, 200)
	failure := make(chan struct{}, 1)
	exit := make(chan struct{}, 1)

	go registerSync(state.Datastore(), rs.regMngr, rs.config.TTL, retSignature, retOther, failure, exit, payload)
	go checkInMem(state.Beacon(), rs.regMngr, payload, retOther)

	var failed bool
	for i, _ := range payload {
		if failed { // after failure just populate the errors
			retSignature <- SVRReqResp{Iter: i, Err: errors.New("failed")}
			continue
		}
		select {
		case <-failure:
			failed = true
		case rs.regMngr.VerifyInput <- SVRReq{payload[i], i, retSignature}:
		}
	}

	<-exit
	return nil
}

func checkInMem(state BeaconState, rMgr *RegisteredManager, payload []SignedValidatorRegistration, out chan SVRReqResp) {
	for i, sp := range payload {
		if verifyTimestamp(sp.Message.Timestamp) {
			out <- SVRReqResp{Commit: false, Iter: i} //return fmt.Errorf("request too far in future for %s", registerRequest.Message.Pubkey.String())
			continue
		}

		pk := PubKey{sp.Message.Pubkey}
		known, _ := state.IsKnownValidator(pk.PubkeyHex())
		if !known {
			out <- SVRReqResp{Commit: false, Iter: i}
			continue
		}

		previousValidatorTimestamp, ok := rMgr.Get(pk.String())
		out <- SVRReqResp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), Iter: i}
	}
}

func VerifyParallel(vInput <-chan SVRReq, domain types.Domain) {
	for registerRequest := range vInput {
		// TODO(l): Make sure this doesn't Panic!
		_, err := types.VerifySignature(
			registerRequest.Message,
			domain,
			registerRequest.Message.Pubkey[:],
			registerRequest.Signature[:],
		)
		registerRequest.Response <- SVRReqResp{Err: err, Iter: registerRequest.Iter}
	}
}
