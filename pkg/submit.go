package relay

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"

	blst "github.com/supranational/blst/bindings/go"
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

func (rm *RegisteredManager) RunWorkers(num int) {
	for i := 0; i < num; i++ {
		go VerifyParallel(rm.VerifyInput)
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

type ReadyTable struct {
	RT  map[int]struct{}
	acc sync.RWMutex
}

func NewReadyTable() *ReadyTable {
	return &ReadyTable{
		RT: make(map[int]struct{}),
	}
}

func (rt *ReadyTable) Set(k int) {
	rt.acc.Lock()
	defer rt.acc.Unlock()
	rt.RT[k] = struct{}{}
}

func (rt *ReadyTable) Has(k int) bool {
	rt.acc.RLock()
	defer rt.acc.RUnlock()
	_, ok := rt.RT[k]
	return ok
}

type Setter interface {
	Set(k string, value uint64)
}

type SVRReq struct {
	payload  SignedValidatorRegistration
	Msg      [32]byte
	Iter     int
	Response chan SVRReqResp
}

type SVRReqResp struct {
	Err    error
	Commit bool
	Iter   int
}

/*
func registerSync(datas Datastore, s Setter, ttl time.Duration, a, b chan SVRReqResp, failure, exit chan struct{}, payload []SignedValidatorRegistration) {
	rcv := make(map[int]struct{})

	saveCh := make(chan int, 5)
	go storeIfReady(datas, s, saveCh, payload, ttl)
	go storeIfReady(datas, s, saveCh, payload, ttl)
	go storeIfReady(datas, s, saveCh, payload, ttl)
	defer close(saveCh)

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
				_, ok := rcv[item.Iter]
				if !ok {
					rcv[item.Iter] = struct{}{}
				} else {
					saveCh <- item.Iter
					delete(rcv, item.Iter)
				}
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
				// store only if it has two records from both checks
				_, ok := rcv[item.Iter]
				if !ok {
					rcv[item.Iter] = struct{}{}
				} else {
					saveCh <- item.Iter
					delete(rcv, item.Iter)
				}
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

func storeIfReady(datas Datastore, s Setter, in chan int, payload []SignedValidatorRegistration, ttl time.Duration) {
	for i := range in {
		registerRequest := &payload[i]
		if err := datas.PutRegistrationRaw(context.Background(), PubKey{registerRequest.Message.Pubkey}, registerRequest.Raw, ttl); err != nil {
			return fmt.Errorf("failed to store %s", registerRequest.Message.Pubkey.String())
		}

		s.Set(registerRequest.Message.Pubkey.String(), registerRequest.Message.Timestamp)
	}
}*/

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
				log.Println("item.Err", item.Err)
				if !errored {
					errored = true
					close(failure)
				}
			} else {
				err := storeIfReady(datas, s, rcv, item.Iter, payload[item.Iter], ttl)
				if err != nil {
					log.Println("err", err)
				}
			}

			if numA == total && numB == total {
				break SyncLoop
			}
		case item = <-b:
			numB++
			if item.Err != nil {
				log.Println("item.Err", item.Err)
				if !errored {
					errored = true
					close(failure)
				}
			} else if item.Commit {
				err := storeIfReady(datas, s, rcv, item.Iter, payload[item.Iter], ttl)
				if err != nil {
					log.Println("err", err)
				}
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
	t := time.Now()
	retSignature := make(chan SVRReqResp, 3000)
	retOther := make(chan SVRReqResp, 3000)
	failure := make(chan struct{}, 1)
	exit := make(chan struct{}, 1)

	go registerSync(state.Datastore(), rs.regMngr, rs.config.TTL, retSignature, retOther, failure, exit, payload)
	go checkInMem(state.Beacon(), rs.regMngr, payload, retOther)
	// This gives additional speedup but it's futile for now

	//	VerifyInput := make(chan SVRReq, 20000)
	var failed bool
	/*
		for i := 0; i < 100; i++ {
			go VerifyParallel(VerifyInput)
		}
	*/

	for i := range payload {
		if failed { // after failure just populate the errors
			retSignature <- SVRReqResp{Iter: i, Err: errors.New("failed")}
			continue
		}

		msg, _ := types.ComputeSigningRoot(payload[i].Message, rs.builderSigningDomain)
		/*if err != nil {
			return false, err
		}*/

		select {
		case <-failure:
			failed = true
		case rs.regMngr.VerifyInput <- SVRReq{payload[i], msg, i, retSignature}:
		}
	}
	//	log.Println("SentAll ", time.Since(t))

	<-exit
	log.Println("RegisterValidator2 ", time.Since(t))
	return nil
}

func checkInMem(state BeaconState, rMgr *RegisteredManager, payload []SignedValidatorRegistration, out chan SVRReqResp) {
	//checkTime := time.Now()
	//	log.Println("check begin", time.Since(checkTime))
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
	// log.Println("checkTime", time.Since(checkTime))
}

func VerifyParallel(vInput <-chan SVRReq) {
	for v := range vInput {
		// TODO(l): Make sure this doesn't Panic!
		ok, err := VerifySignatureBytes(v.Msg, v.payload.Signature[:], v.payload.Message.Pubkey[:])
		if err == nil && !ok {
			err = bls.ErrInvalidSignature
		}
		v.Response <- SVRReqResp{Err: err, Iter: v.Iter}
	}
}

var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

const BLST_SUCCESS = 0x0

func VerifySignatureBytes(msg [32]byte, sigBytes, pkBytes []byte) (bool, error) {
	sig, err := bls.SignatureFromBytes(sigBytes)
	if err != nil {
		return false, err
	}

	pk, err := bls.PublicKeyFromBytes(pkBytes)
	if err != nil {
		return false, err
	}

	if pk == nil || len(msg) == 0 || sig == nil {
		return false, nil
	}

	return (blst.CoreVerifyPkInG1(pk, sig, true, msg[:], dst, nil) == BLST_SUCCESS), nil
}
