package relay

import (
	"context"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/prometheus/client_golang/prometheus"
)

type ProcessManager struct {
	M   map[string]uint64
	acc sync.RWMutex

	VerifyInputChStackA chan SVRReq
	VerifyInputChStackB chan SVRReq
	VerifyInputChStackC chan SVRReq

	StoreCh chan SVRStoreReq

	m ProcessManagerMetrics
}

func NewProcessManager(verifySize, storeSize uint) *ProcessManager {
	rm := &ProcessManager{
		M: make(map[string]uint64),

		VerifyInputChStackA: make(chan SVRReq, verifySize),
		VerifyInputChStackB: make(chan SVRReq, verifySize),
		VerifyInputChStackC: make(chan SVRReq, verifySize),

		StoreCh: make(chan SVRStoreReq, storeSize),
	}
	rm.initMetrics()
	return rm
}

func (rm *ProcessManager) RunVerify(num uint) {
	for i := uint(0); i < num; i++ {
		go rm.VerifyParallel()
	}
}

func (rm *ProcessManager) RunStore(store Datastore, ttl time.Duration, num uint) {
	for i := uint(0); i < num; i++ {
		go rm.ParallelStoreIfReady(store, ttl)
	}
}

func (rm *ProcessManager) RunCleanup(checkinterval uint64, cleanupInterval time.Duration) {
	for {
		now := uint64(time.Now().Unix())
		var keys []string
		rm.acc.RLock()
		for k, v := range rm.M {
			if checkinterval < now-v {
				keys = append(keys, k)
			}
		}
		rm.acc.RUnlock()

		rm.acc.Lock()
		for _, k := range keys {
			delete(rm.M, k)
		}
		rm.acc.Unlock()
		rm.m.MapSize.Set(float64(len(rm.M)))

		time.Sleep(cleanupInterval)
	}
}

func (rm *ProcessManager) LoadAll(m map[string]uint64) {
	rm.acc.Lock()
	defer rm.acc.Unlock()

	for k, v := range m {
		rm.M[k] = v
	}

	rm.m.MapSize.Set(float64(len(rm.M)))
}

func (rm *ProcessManager) StoreChan() chan SVRStoreReq {
	return rm.StoreCh
}

func (rm *ProcessManager) VerifyChan() chan SVRReq {
	return rm.VerifyInputChStackC
}

func (rm *ProcessManager) VerifyChanStacks(stack uint) chan SVRReq {
	switch stack {
	case 0:
		return rm.VerifyInputChStackA
	case 1:
		return rm.VerifyInputChStackB
	case 2:
		return rm.VerifyInputChStackC
	}
	return rm.VerifyInputChStackC
}

func (rm *ProcessManager) Set(k string, value uint64) {
	rm.acc.Lock()
	defer rm.acc.Unlock()
	defer rm.m.MapSize.Set(float64(len(rm.M)))
	rm.M[k] = value
}

func (rm *ProcessManager) Get(k string) (value uint64, ok bool) {
	rm.acc.RLock()
	defer rm.acc.RUnlock()
	value, ok = rm.M[k]
	return
}

func (rm *ProcessManager) ParallelStoreIfReady(datas Datastore, ttl time.Duration) {
	rm.m.RunningWorkers.WithLabelValues("ParallelStoreIfReady").Inc()
	defer rm.m.RunningWorkers.WithLabelValues("ParallelStoreIfReady").Dec()
	ctx := context.Background()

	for i := range rm.StoreCh {
		i.Response <- SVRReqResp{
			Type: ResponseTypeStored,
			Iter: i.Iter,
			Err:  datas.PutRegistrationRaw(ctx, structs.PubKey{PublicKey: i.Pubkey}, i.RawPayload, ttl),
		}
	}
}

func (rm *ProcessManager) VerifyParallel() {
	rm.m.RunningWorkers.WithLabelValues("VerifyParallel").Inc()
	defer rm.m.RunningWorkers.WithLabelValues("VerifyParallel").Dec()

	timerA := rm.m.VerifyTiming.WithLabelValues("submitBlock")
	timerB := rm.m.VerifyTiming.WithLabelValues("registerValidator")
	timerC := rm.m.VerifyTiming.WithLabelValues("other")

	// Few words of explanation:
	// Because Relay can only process a limitted number of signatures, it would operate on 3 different channels.
	// This way we'll randomly pick next from different queues and huge number of request
	// would not bring other from being processed.
	//
	// The 3 stacks are meant for (in order of signifficance, however processed IN RANDOM ORDER):
	// submitting blocks, registering validators and others.
	for {
		select {
		case v := <-rm.VerifyInputChStackA:
			t := prometheus.NewTimer(timerA)
			v.Response <- verifyUnit(v.Iter, v.Msg, v.Signature, v.Pubkey)
			t.ObserveDuration()
		case v := <-rm.VerifyInputChStackB:
			t := prometheus.NewTimer(timerB)
			v.Response <- verifyUnit(v.Iter, v.Msg, v.Signature, v.Pubkey)
			t.ObserveDuration()
		case v := <-rm.VerifyInputChStackC:
			t := prometheus.NewTimer(timerC)
			v.Response <- verifyUnit(v.Iter, v.Msg, v.Signature, v.Pubkey)
			t.ObserveDuration()
		}
	}
}

func verifyUnit(iter int, msg [32]byte, sigBytes [96]byte, pkBytes [48]byte) SVRReqResp {
	ok, err := VerifySignatureBytes(msg, sigBytes[:], pkBytes[:])
	if err == nil && !ok {
		err = bls.ErrInvalidSignature
	}
	return SVRReqResp{Err: err, Iter: iter, Type: ResponseTypeVerify}
}
