package relay

import (
	"context"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
)

type RegisteredManager struct {
	M   map[string]uint64
	acc sync.RWMutex

	VerifyInputCh chan SVRReq
	StoreCh       chan SVRReq
}

func NewRegisteredManager(verifySize, storeSize int) *RegisteredManager {
	return &RegisteredManager{
		M:             make(map[string]uint64),
		VerifyInputCh: make(chan SVRReq, verifySize),
		StoreCh:       make(chan SVRReq, storeSize),
	}
}

func (rm *RegisteredManager) RunVerify(num int) {
	for i := 0; i < num; i++ {
		go rm.VerifyParallel()
	}
}

func (rm *RegisteredManager) RunStore(store Datastore, num int) {
	for i := 0; i < num; i++ {
		go rm.ParallelStoreIfReady(store, time.Minute*40)
	}
}

func (rm *RegisteredManager) StoreChan() chan SVRReq {
	return rm.StoreCh
}

func (rm *RegisteredManager) VerifyChan() chan SVRReq {
	return rm.VerifyInputCh
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

func (rm *RegisteredManager) ParallelStoreIfReady(datas Datastore, ttl time.Duration) {
	for i := range rm.StoreCh {
		err := datas.PutRegistrationRaw(context.Background(), structs.PubKey{i.payload.Message.Pubkey}, i.payload.Raw, ttl)
		i.Response <- SVRReqResp{
			Iter: i.Iter,
			Err:  err,
		}
	}
}

func (rm *RegisteredManager) VerifyParallel() {
	for v := range rm.VerifyInputCh {
		ok, err := VerifySignatureBytes(v.Msg, v.payload.Signature[:], v.payload.Message.Pubkey[:])
		if err == nil && !ok {
			err = bls.ErrInvalidSignature
		}
		v.Response <- SVRReqResp{Err: err, Iter: v.Iter}
	}
}
