package validators

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
)

// StoreReqItem is similar to VerifyReq jsut for storing payloads
type StoreReqItem struct {
	Payload types.SignedValidatorRegistration
	Time    uint64
	Pubkey  types.PublicKey

	// additional params
	FeeRecipient types.Address
	GasLimit     uint64
}
type StoreReq struct {
	Items []StoreReqItem
}

type CacheEntry struct {
	Time  time.Time
	Entry types.RegisterValidatorRequestMessage
}

type StoreManager struct {
	RegistrationCache       *lru.Cache[types.PublicKey, CacheEntry]
	storeTTLHalftimeSeconds int

	LastRegTime map[string]uint64 // [pubkey]timestamp
	lrtl        sync.RWMutex      // LastRegTime RWLock

	StoreCh             chan StoreReq
	storeMutex          sync.RWMutex
	storeWorkersCounter sync.WaitGroup
	isClosed            int32

	l log.Logger

	m StoreManagerMetrics
}

func NewStoreManager(l log.Logger, storeTTLHalftimeSeconds int, storeSize uint, registrationCacheSize int) (*StoreManager, error) {
	cache, err := lru.New[types.PublicKey, CacheEntry](registrationCacheSize)
	if err != nil {
		return nil, err
	}

	rm := &StoreManager{
		l:                       l,
		storeTTLHalftimeSeconds: storeTTLHalftimeSeconds,
		LastRegTime:             make(map[string]uint64),
		RegistrationCache:       cache,
		StoreCh:                 make(chan StoreReq, storeSize),
	}
	rm.initMetrics()
	return rm, nil
}

func (pm *StoreManager) Close(ctx context.Context) {
	pm.l.Info("Closing process manager")
	pm.storeMutex.Lock()
	atomic.StoreInt32(&(pm.isClosed), int32(1))
	// close of store channel would initiate range automatic exits
	close(pm.StoreCh)
	pm.storeMutex.Unlock()

	pm.l.Info("Awaiting registration stores to finish")
	pm.storeWorkersCounter.Wait()
	pm.l.Info("All registrations stored")
}

func (rm *StoreManager) RunCleanup(checkinterval uint64, cleanupInterval time.Duration) {
	for {
		rm.cleanupCycle(checkinterval)
		rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
		time.Sleep(cleanupInterval)
	}
}

func (rm *StoreManager) cleanupCycle(checkinterval uint64) {
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

func (rm *StoreManager) LoadAll(m map[string]uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	for k, v := range m {
		rm.LastRegTime[k] = v
	}

	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
}

func (rm *StoreManager) RunStore(store RegistrationStore, ttl time.Duration, num uint) {
	for i := uint(0); i < num; i++ {
		rm.storeWorkersCounter.Add(1)
		go rm.ParallelStore(store, ttl)
	}
}

func (rm *StoreManager) Check(rvg *types.RegisterValidatorRequestMessage) bool {
	v, ok := rm.RegistrationCache.Get(rvg.Pubkey)
	if !ok {
		return false
	}

	if time.Since(v.Time).Seconds() > float64(rm.storeTTLHalftimeSeconds+rand.Intn(rm.storeTTLHalftimeSeconds)-(rm.storeTTLHalftimeSeconds*5/100)) {
		return false
	}

	return v.Entry.FeeRecipient == rvg.FeeRecipient && v.Entry.GasLimit == rvg.GasLimit
}

func (pm *StoreManager) ParallelStore(datas RegistrationStore, ttl time.Duration) {
	defer pm.storeWorkersCounter.Done()

	pm.m.RunningWorkers.WithLabelValues("ParallelStore").Inc()
	defer pm.m.RunningWorkers.WithLabelValues("ParallelStore").Dec()

	ctx := context.Background()

	for payload := range pm.StoreCh {
		pm.m.StoreSize.Observe(float64(len(payload.Items)))
		if err := pm.storeRegistration(ctx, datas, ttl, payload); err != nil {
			pm.l.Errorf("error storing registration - %w ", err)
			continue
		}
	}
}

func (pm *StoreManager) storeRegistration(ctx context.Context, datas RegistrationStore, ttl time.Duration, payload StoreReq) (err error) {
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
		now := time.Now()
		err := datas.PutRegistration(ctx, structs.PubKey{PublicKey: i.Pubkey}, i.Payload, ttl)
		if err != nil {
			pm.m.StoreErrorRate.Inc()
			return err
		}
		pm.RegistrationCache.Add(i.Pubkey, CacheEntry{
			Time: now,
			Entry: types.RegisterValidatorRequestMessage{
				FeeRecipient: i.FeeRecipient,
				Timestamp:    i.Time,
				GasLimit:     i.GasLimit},
		})

		t.ObserveDuration()
	}
	return nil
}

func (rm *StoreManager) Set(k string, value uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	rm.LastRegTime[k] = value
	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
}

func (rm *StoreManager) SendStore(request StoreReq) {
	// lock needed for Close()
	rm.storeMutex.RLock()
	defer rm.storeMutex.RUnlock()
	if atomic.LoadInt32(&(rm.isClosed)) == 0 {
		rm.StoreCh <- request
	}

}

func (rm *StoreManager) Get(k string) (value uint64, ok bool) {
	rm.lrtl.RLock()
	defer rm.lrtl.RUnlock()

	value, ok = rm.LastRegTime[k]
	return value, ok
}
