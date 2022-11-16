package relay

import (
	"context"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	ResponseQueueSubmit = iota
	ResponseQueueRegister
	ResponseQueueOther
)

type ProcessManager struct {
	LastRegTime map[string]uint64 // [pubkey]timestamp
	lrtl        sync.RWMutex      // LastRegTime RWLock

	VerifySubmitBlockCh       chan VSReq
	VerifyRegisterValidatorCh chan VSReq
	VerifyOtherCh             chan VSReq

	StoreCh chan StoreReq

	m ProcessManagerMetrics
}

func NewProcessManager(verifySize, storeSize uint) *ProcessManager {
	rm := &ProcessManager{
		LastRegTime: make(map[string]uint64),

		VerifySubmitBlockCh:       make(chan VSReq, verifySize),
		VerifyRegisterValidatorCh: make(chan VSReq, verifySize),
		VerifyOtherCh:             make(chan VSReq, verifySize),

		StoreCh: make(chan StoreReq, storeSize),
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
		rm.lrtl.RLock()
		for k, v := range rm.LastRegTime {
			if checkinterval < now-v {
				keys = append(keys, k)
			}
		}
		rm.lrtl.RUnlock()

		rm.lrtl.Lock()
		for _, k := range keys {
			delete(rm.LastRegTime, k)
		}
		rm.lrtl.Unlock()
		rm.m.MapSize.Set(float64(len(rm.LastRegTime)))

		time.Sleep(cleanupInterval)
	}
}

func (rm *ProcessManager) LoadAll(m map[string]uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	for k, v := range m {
		rm.LastRegTime[k] = v
	}

	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
}

func (rm *ProcessManager) GetStoreChan() chan StoreReq {
	return rm.StoreCh
}

func (rm *ProcessManager) VerifyChan() chan VSReq {
	return rm.VerifyOtherCh
}

func (rm *ProcessManager) GetVerifyChan(stack uint) chan VSReq {
	switch stack {
	case ResponseQueueSubmit:
		return rm.VerifySubmitBlockCh
	case ResponseQueueRegister:
		return rm.VerifyRegisterValidatorCh
	default: // ResponseQueueOther
		return rm.VerifyOtherCh
	}
}

func (rm *ProcessManager) Set(k string, value uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()
	defer rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
	rm.LastRegTime[k] = value
}

func (rm *ProcessManager) Get(k string) (value uint64, ok bool) {
	rm.lrtl.RLock()
	defer rm.lrtl.RUnlock()
	value, ok = rm.LastRegTime[k]
	return
}

func (rm *ProcessManager) ParallelStoreIfReady(datas Datastore, ttl time.Duration) {
	rm.m.RunningWorkers.WithLabelValues("ParallelStoreIfReady").Inc()
	defer rm.m.RunningWorkers.WithLabelValues("ParallelStoreIfReady").Dec()
	ctx := context.Background()

	for i := range rm.StoreCh {
		i.Response <- Resp{
			Type: ResponseTypeStored,
			ID:   i.ID,
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
		case v := <-rm.VerifySubmitBlockCh:
			t := prometheus.NewTimer(timerA)
			v.Response <- verifyUnit(v.ID, v.Msg, v.Signature, v.Pubkey)
			t.ObserveDuration()
		case v := <-rm.VerifyRegisterValidatorCh:
			t := prometheus.NewTimer(timerB)
			v.Response <- verifyUnit(v.ID, v.Msg, v.Signature, v.Pubkey)
			t.ObserveDuration()
		case v := <-rm.VerifyOtherCh:
			t := prometheus.NewTimer(timerC)
			v.Response <- verifyUnit(v.ID, v.Msg, v.Signature, v.Pubkey)
			t.ObserveDuration()
		}
	}
}

func verifyUnit(id int, msg [32]byte, sigBytes [96]byte, pkBytes [48]byte) Resp {
	ok, err := VerifySignatureBytes(msg, sigBytes[:], pkBytes[:])
	if err == nil && !ok {
		err = bls.ErrInvalidSignature
	}
	return Resp{Err: err, ID: id, Type: ResponseTypeVerify}
}
