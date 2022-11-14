package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"

	blst "github.com/supranational/blst/bindings/go"
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
			respCh <- SVRReqResp{Iter: i, Err: errors.New("failed"), Type: ResponseTypeVerify}
			failed = true
			break
		}

		//sig :=
		//pub := [:]
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
		if verifyTimestamp(sp.Message.Timestamp) {
			out <- SVRReqResp{Commit: false, Iter: i, Err: fmt.Errorf("request too far in future for %s", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}
			return
		}

		pk := structs.PubKey{PublicKey: sp.Message.Pubkey}
		known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
		if !known {
			out <- SVRReqResp{Commit: false, Iter: i, Err: fmt.Errorf("%s not a known validator", sp.Message.Pubkey.String()), Type: ResponseTypeOthers}
			return
		}

		previousValidatorTimestamp, ok := getter.Get(pk.String()) // Do not error on this
		out <- SVRReqResp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), Iter: i, Type: ResponseTypeOthers}
	}
}

var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

const BLST_SUCCESS = 0x0

func VerifySignatureBytes(msg [32]byte, sigBytes, pkBytes []byte) (ok bool, err error) {
	defer func() { // better safe than sorry
		if r := recover(); r != nil {
			var isErr bool
			err, isErr = r.(error)
			if !isErr {
				err = fmt.Errorf("pkg: %v", r)
			}
		}
	}()

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

	if blst.CoreVerifyPkInG1(pk, sig, true, msg[:], dst, nil) != BLST_SUCCESS {
		return false, bls.ErrInvalidSignature
	}

	return true, nil
}

func VerifySignature(obj types.HashTreeRoot, d types.Domain, pkBytes, sigBytes []byte) (bool, error) {
	msg, err := types.ComputeSigningRoot(obj, d)
	if err != nil {
		return false, err
	}

	return VerifySignatureBytes(msg, sigBytes, pkBytes)
}
