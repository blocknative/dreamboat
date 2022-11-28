package relay

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/lthibault/log"
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

	VerifySubmitBlockCh       chan VerifyReq
	VerifyRegisterValidatorCh chan VerifyReq
	VerifyOtherCh             chan VerifyReq

	StoreCh             chan StoreReq
	storeMutex          sync.RWMutex
	storeWorkersCounter sync.WaitGroup
	isClosed            int32

	l log.Logger

	m ProcessManagerMetrics
}

func NewProcessManager(l log.Logger, verifySize, storeSize uint) *ProcessManager {
	rm := &ProcessManager{
		l:           l,
		LastRegTime: make(map[string]uint64),

		VerifySubmitBlockCh:       make(chan VerifyReq, verifySize),
		VerifyRegisterValidatorCh: make(chan VerifyReq, verifySize),
		VerifyOtherCh:             make(chan VerifyReq, verifySize),

		StoreCh: make(chan StoreReq, storeSize),
	}
	rm.initMetrics()
	return rm
}

func (rm *ProcessManager) Close(ctx context.Context) {
	rm.storeMutex.Lock()
	atomic.StoreInt32(&(rm.isClosed), int32(1))
	// close of store channel would initiate range automatic exits
	close(rm.StoreCh)
	rm.storeMutex.Unlock()

	rm.storeWorkersCounter.Wait()
}

func (rm *ProcessManager) RunVerify(num uint) {
	for i := uint(0); i < num; i++ {
		go rm.VerifyParallel()
	}
}

func (rm *ProcessManager) RunStore(store Datastore, ttl time.Duration, num uint) {
	for i := uint(0); i < num; i++ {
		rm.storeWorkersCounter.Add(1)
		go rm.ParallelStore(store, ttl)
	}
}

func (rm *ProcessManager) RunCleanup(checkinterval uint64, cleanupInterval time.Duration) {
	for {
		rm.cleanupCycle(checkinterval)
		rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
		time.Sleep(cleanupInterval)
	}
}

func (rm *ProcessManager) cleanupCycle(checkinterval uint64) {
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
}

func (rm *ProcessManager) LoadAll(m map[string]uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	for k, v := range m {
		rm.LastRegTime[k] = v
	}

	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
}

type StoreReq struct {
	Items []StoreReqItem
}

func (rm *ProcessManager) Set(k string, value uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	rm.LastRegTime[k] = value
	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
}

func (rm *ProcessManager) SendStore(request StoreReq) {
	// lock needed for Close()
	rm.storeMutex.RLock()
	defer rm.storeMutex.RUnlock()
	if atomic.LoadInt32(&(rm.isClosed)) == 0 {
		rm.StoreCh <- request
	}

}

func (rm *ProcessManager) VerifyChan() chan VerifyReq {
	return rm.VerifyOtherCh
}

func (rm *ProcessManager) GetVerifyChan(stack uint) chan VerifyReq {
	switch stack {
	case ResponseQueueSubmit:
		return rm.VerifySubmitBlockCh
	case ResponseQueueRegister:
		return rm.VerifyRegisterValidatorCh
	default: // ResponseQueueOther
		return rm.VerifyOtherCh
	}
}

func (rm *ProcessManager) Get(k string) (value uint64, ok bool) {
	rm.lrtl.RLock()
	defer rm.lrtl.RUnlock()

	value, ok = rm.LastRegTime[k]
	return value, ok
}

func (pm *ProcessManager) ParallelStore(datas Datastore, ttl time.Duration) {
	defer pm.storeWorkersCounter.Done()

	pm.m.RunningWorkers.WithLabelValues("ParallelStore").Inc()
	defer pm.m.RunningWorkers.WithLabelValues("ParallelStore").Dec()

	ctx := context.Background()

	for payload := range pm.StoreCh {
		pm.m.StoreSize.Observe(float64(len(payload.Items)))
		if err := pm.storeRegistration(ctx, datas, ttl, payload); err != nil {
			pm.l.Errorf("error storing registration - %w ", err)
		}
	}
}

func (pm *ProcessManager) storeRegistration(ctx context.Context, datas Datastore, ttl time.Duration, payload StoreReq) (err error) {
	defer func() { // better safe than sorry
		if r := recover(); r != nil {
			var isErr bool
			err, isErr = r.(error)
			if !isErr {
				err = fmt.Errorf("storeRegistration panic: %v", r)
			}
		}
	}()

	pm.lrtl.Lock()
	for _, v := range payload.Items {
		pm.LastRegTime[v.Pubkey.String()] = v.Time
	}
	pm.m.MapSize.Set(float64(len(pm.LastRegTime)))
	pm.lrtl.Unlock()

	for _, i := range payload.Items {
		t := prometheus.NewTimer(pm.m.StoreTiming)

		err := datas.PutRegistrationRaw(ctx, structs.PubKey{PublicKey: i.Pubkey}, i.RawPayload, ttl)
		if err != nil {
			pm.m.StoreErrorRate.Inc()
			return err
		}
		t.ObserveDuration()
	}
	return nil
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
			_ = verifyCheck(timerA, v)
		case v := <-rm.VerifyRegisterValidatorCh:
			_ = verifyCheck(timerB, v)
		case v := <-rm.VerifyOtherCh:
			_ = verifyCheck(timerC, v)
		}
	}
}

func verifyCheck(o prometheus.Observer, v VerifyReq) (err error) {
	defer func() { // better safe than sorry
		if r := recover(); r != nil {
			var isErr bool
			err, isErr = r.(error)
			if !isErr {
				err = fmt.Errorf("verify signature bytes panic: %v", r)
			}
		}
	}()

	if v.Response.IsClosed() {
		return nil
	}
	t := prometheus.NewTimer(o)
	defer t.ObserveDuration()

	v.Response.Send(verifyUnit(v.ID, v.Msg, v.Signature, v.Pubkey))
	return err

}
func verifyUnit(id int, msg [32]byte, sigBytes [96]byte, pkBytes [48]byte) Resp {
	ok, err := VerifySignatureBytes(msg, sigBytes[:], pkBytes[:])
	if err == nil && !ok {
		err = bls.ErrInvalidSignature
	}
	return Resp{Err: err, ID: id, Type: ResponseTypeVerify}
}

type StoreResp struct {
	nonErrors []int64
	numAll    int

	rLock    sync.Mutex
	isClosed int32
	err      error

	done chan error
}

func NewRespC(numAll int) (s *StoreResp) {
	return &StoreResp{
		numAll: numAll,
		done:   make(chan error, 1),
	}
}

func (s *StoreResp) SuccessfullIndexes() []int64 {
	return s.nonErrors
}

func (s *StoreResp) Done() chan error {
	return s.done
}

func (s *StoreResp) IsClosed() bool {
	return atomic.LoadInt32(&(s.isClosed)) != 0
}

func (s *StoreResp) Send(r Resp) {
	s.rLock.Lock()
	defer s.rLock.Unlock()

	if s.IsClosed() {
		return
	}

	if r.Err != nil {
		s.err = r.Err
		s.close()
		return
	}

	s.nonErrors = append(s.nonErrors, int64(r.ID))
	if s.numAll == len(s.nonErrors) {
		s.close()
		return
	}
}

func (s *StoreResp) Error() (err error) {
	return s.err
}

func (s *StoreResp) Close(id int, err error) {
	s.rLock.Lock()
	defer s.rLock.Unlock()

	if err != nil {
		s.err = err
	}
	s.close()
}

func (s *StoreResp) close() {
	if !s.IsClosed() {
		atomic.StoreInt32(&(s.isClosed), int32(1))
		close(s.done)
	}
}
