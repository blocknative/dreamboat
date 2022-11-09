package relay

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"

	blst "github.com/supranational/blst/bindings/go"
)

type Setter interface {
	Set(k string, value uint64)
}

type Getter interface {
	Get(k string) (value uint64, ok bool)
}

type SVRReq struct {
	payload  structs.SignedValidatorRegistration
	Msg      [32]byte
	Iter     int
	Response chan SVRReqResp
}

type SVRReqResp struct {
	Err    error
	Commit bool
	Iter   int
}

func registerSync(s RegistrationManager, ttl time.Duration, a, b chan SVRReqResp, failure, exit chan struct{}, payload []structs.SignedValidatorRegistration) {
	rcv := make(map[int]struct{})

	var numA, numB, stored int
	var errored bool
	var item SVRReqResp

	saved := make(chan SVRReqResp, len(payload))
	var sentToStore int
	var total = len(payload)

	storeCh := s.StoreChan()
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
				ok := storeIfReady(s, rcv, item.Iter, payload[item.Iter])
				if ok {
					select {
					case <-saved:
						stored++
					case storeCh <- SVRReq{
						payload:  payload[item.Iter],
						Iter:     item.Iter,
						Response: saved,
					}:
						sentToStore++
					}
				}
			}

			if numA == total && numB == total && sentToStore == stored {
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
				ok := storeIfReady(s, rcv, item.Iter, payload[item.Iter])
				if ok {
					select {
					case <-saved:
						stored++
					case storeCh <- SVRReq{
						payload:  payload[item.Iter],
						Iter:     item.Iter,
						Response: saved,
					}:
						sentToStore++
					}
				}
			}
			if numA == total && numB == total && sentToStore == stored {
				break SyncLoop
			}
		case <-saved:
			stored++
			if numA == total && numB == total && sentToStore == stored {
				break SyncLoop
			}
		}
	}
	close(a)
	close(b)
	close(exit)
	close(failure)
}

func storeIfReady(s Setter, rcv map[int]struct{}, iter int, registerRequest structs.SignedValidatorRegistration) bool {
	// store only if it has two records from both checks
	_, ok := rcv[iter]
	if !ok {
		rcv[iter] = struct{}{}
		return false
	}
	//if err := datas.PutRegistrationRaw(context.Background(), PubKey{registerRequest.Message.Pubkey}, registerRequest.Raw, ttl); err != nil {
	//	return fmt.Errorf("failed to store %s", registerRequest.Message.Pubkey.String())
	//}
	delete(rcv, iter)
	s.Set(registerRequest.Message.Pubkey.String(), registerRequest.Message.Timestamp)
	return true
}

// ***** Builder Domain *****
// RegisterValidator is called is called by validators communicating through mev-boost who would like to receive a block from us when their slot is scheduled
func (rs *Relay) RegisterValidator(ctx context.Context, payload []structs.SignedValidatorRegistration) error {
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

	go registerSync(rs.regMngr, rs.config.TTL, retSignature, retOther, failure, exit, payload)
	go checkInMem(rs.beaconState, rs.regMngr, payload, retOther)

	// This gives additional speedup but it's futile for now
	var failed bool

	VInp := rs.regMngr.VerifyChan()
	for i := range payload {
		if failed { // after failure just populate the errors
			retSignature <- SVRReqResp{Iter: i, Err: errors.New("failed")}
			continue
		}

		msg, err := types.ComputeSigningRoot(payload[i].Message, rs.config.BuilderSigningDomain)
		if err != nil {
			retSignature <- SVRReqResp{Iter: i, Err: errors.New("failed")}
			failed = true
		}

		select {
		case <-failure:
			failed = true
		case VInp <- SVRReq{payload[i], msg, i, retSignature}:
		}
	}
	log.Println("SentAll ", time.Since(t))

	<-exit
	log.Println("RegisterValidator2 ", time.Since(t))
	return nil
}

func checkInMem(state State, getter Getter, payload []structs.SignedValidatorRegistration, out chan SVRReqResp) {
	checkTime := time.Now()
	//	log.Println("check begin", time.Since(checkTime))

	beacon := state.Beacon()
	for i, sp := range payload {
		if verifyTimestamp(sp.Message.Timestamp) {
			out <- SVRReqResp{Commit: false, Iter: i} //return fmt.Errorf("request too far in future for %s", registerRequest.Message.Pubkey.String())
			continue
		}

		pk := structs.PubKey{sp.Message.Pubkey}
		known, _ := beacon.IsKnownValidator(pk.PubkeyHex())
		if !known {
			out <- SVRReqResp{Commit: false, Iter: i} // return fmt.Errorf("%s not a known validator", registerRequest.Message.Pubkey.String())
			continue
		}

		previousValidatorTimestamp, ok := getter.Get(pk.String())
		out <- SVRReqResp{Commit: (!ok || sp.Message.Timestamp < previousValidatorTimestamp), Iter: i}
	}
	log.Println("checkTime", time.Since(checkTime))
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

	return (blst.CoreVerifyPkInG1(pk, sig, true, msg[:], dst, nil) == BLST_SUCCESS), nil
}

func VerifySignature(obj types.HashTreeRoot, d types.Domain, pkBytes, sigBytes []byte) (bool, error) {
	msg, err := types.ComputeSigningRoot(obj, d)
	if err != nil {
		return false, err
	}

	return VerifySignatureBytes(msg, sigBytes, pkBytes)
}
