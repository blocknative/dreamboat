package relay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flashbots/go-boost-utils/types"
)

var (
	ALLInput   = make(chan SVRReq, 500)
	OtherInput = make(chan SVRReq, 500)
	RMAnag     = &RegistredManager{M: make(map[string]uint64)}
)

type RegistredManager struct {
	M   map[string]uint64
	acc sync.RWMutex
}

func (rm *RegistredManager) Set(k string, value uint64) {
	rm.acc.Lock()
	defer rm.acc.Unlock()
	rm.M[k] = value
}

func (rm *RegistredManager) Get(k string) (value uint64, ok bool) {
	rm.acc.RLock()
	defer rm.acc.RUnlock()
	value, ok = rm.M[k]
	return
}

type Setter interface {
	Set(k string, value uint64)
}

type SVRReq struct {
	types.SignedValidatorRegistration
	Iter     int
	Response chan SVRReqResp
}
type SVRReqResp struct {
	Err    error
	Commit bool
	Iter   int
}

func registerSync(datas Datastore, s Setter, ttl time.Duration, a, b chan SVRReqResp, failure, exit chan struct{}, payload []types.SignedValidatorRegistration) {
	rcv := make(map[int]struct{})

	var numA, numB int
	var errored bool

SyncLoop:
	for {
		select {
		case item := <-a:
			numA++
			if item.Err != nil {
				if !errored {
					close(failure)
				}
			} else {
				storeIfReady(datas, s, rcv, item.Iter, payload[item.Iter], ttl)
			}

			if numA == len(payload) && numB == len(payload) {
				break SyncLoop
			}
		case item := <-b:
			numB++
			if item.Err != nil {
				if !errored {
					close(failure)
				}
			} else if item.Commit {
				storeIfReady(datas, s, rcv, item.Iter, payload[item.Iter], ttl)
			}

			if numA == len(payload) && numB == len(payload) {
				break SyncLoop
			}
		}
	}
	close(a)
	close(b)
	close(exit)

}

func storeIfReady(datas Datastore, s Setter, rcv map[int]struct{}, iter int, registerRequest types.SignedValidatorRegistration, ttl time.Duration) error {
	_, ok := rcv[iter]
	if !ok {
		rcv[iter] = struct{}{}
		return nil
	}

	// store if it has two records from both checks
	if err := datas.PutRegistration(context.Background(), PubKey{registerRequest.Message.Pubkey}, registerRequest, ttl); err != nil {
		return fmt.Errorf("failed to store %s", registerRequest.Message.Pubkey.String())
	}
	delete(rcv, iter)
	s.Set(registerRequest.Message.Pubkey.String(), registerRequest.Message.Timestamp)
	return nil
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *DefaultRelay) RegisterValidator2(ctx context.Context, payload []types.SignedValidatorRegistration, state State) error {
	for _, registerRequest := range payload {
		if verifyTimestamp(registerRequest.Message.Timestamp) {
			return fmt.Errorf("request too far in future for %s", registerRequest.Message.Pubkey.String())
		}
	}
	retSignature := make(chan SVRReqResp, 20)
	retOther := make(chan SVRReqResp, 200)
	failure := make(chan struct{}, 1)
	exit := make(chan struct{}, 1)

	go registerSync(state.Datastore(), RMAnag, rs.config.TTL, retSignature, retOther, failure, exit, payload)
	go checkInMem(state.Beacon(), payload, retOther)

	var failed bool
	for i, _ := range payload {
		if failed { // after failure just populate the errors
			retSignature <- SVRReqResp{Iter: i, Err: errors.New("failed")}
			continue
		}
		select {
		case <-failure:
			failed = true
		case ALLInput <- SVRReq{payload[i], i, retSignature}:
		}
	}

	<-exit

	return nil
}

// func verifyParallel(in chan SVRReq, domain types.Domain) {
func VerifyParallel(domain types.Domain) {
	for registerRequest := range ALLInput {
		_, err := types.VerifySignature(
			registerRequest.Message,
			domain,
			registerRequest.Message.Pubkey[:],
			registerRequest.Signature[:],
		)
		registerRequest.Response <- SVRReqResp{Err: err, Iter: registerRequest.Iter}
	}
}

func checkInMem(state BeaconState, payload []types.SignedValidatorRegistration, out chan SVRReqResp) {
	for i, sp := range payload {
		pk := PubKey{sp.Message.Pubkey}
		known, _ := state.IsKnownValidator(pk.PubkeyHex())
		if !known {
			out <- SVRReqResp{Commit: false, Iter: i}
			continue
		}

		previousValidatorTimestamp, ok := RMAnag.Get(pk.String())
		out <- SVRReqResp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), Iter: i}
	}
}
