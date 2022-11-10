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

	m RegisteredManagerMetrics
}

func NewRegisteredManager(verifySize, storeSize int) *RegisteredManager {
	rm := &RegisteredManager{
		M:             make(map[string]uint64),
		VerifyInputCh: make(chan SVRReq, verifySize),
		StoreCh:       make(chan SVRReq, storeSize),
	}
	rm.initMetrics()
	return rm
}

func (rm *RegisteredManager) RunVerify(num int) {
	for i := 0; i < num; i++ {
		go rm.VerifyParallel()
	}
}

func (rm *RegisteredManager) RunStore(store Datastore, ttl time.Duration, num int) {
	for i := 0; i < num; i++ {
		go rm.ParallelStoreIfReady(store, ttl)
	}
}

func (rm *RegisteredManager) RunCleanup(checkinterval uint64, cleanupInterval time.Duration) {
	for {
		time.Sleep(cleanupInterval)

		now := uint64(time.Now().Unix())
		var keys []string
		rm.acc.RLock()
		for k, v := range rm.M {
			if now-v < checkinterval {
				keys = append(keys, k)
			}
		}
		rm.acc.RUnlock()

		rm.acc.Lock()
		for _, k := range keys {
			delete(rm.M, k)
		}
		rm.m.MapSize.Set(float64(len(rm.M)))
		rm.acc.Unlock()

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
	defer rm.m.MapSize.Inc()
	rm.M[k] = value
}

func (rm *RegisteredManager) Get(k string) (value uint64, ok bool) {
	rm.acc.RLock()
	defer rm.acc.RUnlock()
	value, ok = rm.M[k]
	return
}

func (rm *RegisteredManager) ParallelStoreIfReady(datas Datastore, ttl time.Duration) {
	rm.m.RunningWorkers.WithLabelValues("ParallelStoreIfReady").Inc()
	defer rm.m.RunningWorkers.WithLabelValues("ParallelStoreIfReady").Dec()
	ctx := context.Background()

	for i := range rm.StoreCh {
		i.Response <- SVRReqResp{
			Type: ResponseTypeStored,
			Iter: i.Iter,
			Err:  datas.PutRegistrationRaw(ctx, structs.PubKey{PublicKey: i.payload.Message.Pubkey}, i.payload.Raw, ttl),
		}
	}
}

func (rm *RegisteredManager) VerifyParallel() {
	rm.m.RunningWorkers.WithLabelValues("VerifyParallel").Inc()
	defer rm.m.RunningWorkers.WithLabelValues("VerifyParallel").Dec()

	for v := range rm.VerifyInputCh {
		ok, err := VerifySignatureBytes(v.Msg, v.payload.Signature[:], v.payload.Message.Pubkey[:])
		if err == nil && !ok {
			err = bls.ErrInvalidSignature
		}
		v.Response <- SVRReqResp{Err: err, Iter: v.Iter, Type: ResponseTypeVerify}
	}
}
