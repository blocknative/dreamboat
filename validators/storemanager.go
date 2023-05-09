package validators

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
)

type ValidatorStore interface {
	GetRegistration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
	PutNewerRegistration(ctx context.Context, pk types.PublicKey, registration types.SignedValidatorRegistration) error
}

type ValidatorCache interface {
	Add(types.PublicKey, structs.ValidatorCacheEntry) (evicted bool)
	Get(types.PublicKey) (structs.ValidatorCacheEntry, bool)
}

// StoreReqItem is a payload requested to be stored
type StoreReqItem struct {
	Payload types.SignedValidatorRegistration
	Time    uint64
}
type StoreReq struct {
	Items []StoreReqItem
}

type StoreManager struct {
	RegistrationCache ValidatorCache
	writeTTL          time.Duration

	store ValidatorStore

	StoreCh             chan StoreReq
	storeMutex          sync.RWMutex
	storeWorkersCounter sync.WaitGroup
	isClosed            int32

	l log.Logger

	m StoreManagerMetrics
}

func NewStoreManager(l log.Logger, cache ValidatorCache, store ValidatorStore, writeTTL time.Duration, storeSize uint) *StoreManager {
	rm := &StoreManager{
		l:                 l,
		store:             store,
		writeTTL:          writeTTL,
		RegistrationCache: cache,
		StoreCh:           make(chan StoreReq, storeSize),
	}
	rm.initMetrics()
	return rm
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

func (rm *StoreManager) GetRegistration(ctx context.Context, pk types.PublicKey) (types.SignedValidatorRegistration, error) {
	return rm.store.GetRegistration(ctx, pk)
}

func (rm *StoreManager) Check(rvg *types.RegisterValidatorRequestMessage) bool {
	v, ok := rm.RegistrationCache.Get(rvg.Pubkey)
	if !ok {
		return false
	}

	if time.Since(v.Time).Seconds() > (rm.writeTTL.Seconds() - (rm.writeTTL.Seconds() * 5 / 100)) {
		return false
	}

	return v.Entry.Message.FeeRecipient == rvg.FeeRecipient && v.Entry.Message.GasLimit == rvg.GasLimit
}

func (rm *StoreManager) SendStore(request StoreReq) {
	// lock needed for Close()
	rm.storeMutex.RLock()
	defer rm.storeMutex.RUnlock()
	if atomic.LoadInt32(&(rm.isClosed)) == 0 {
		rm.StoreCh <- request
	}
}

func (rm *StoreManager) RunStore(num uint) {
	for i := uint(0); i < num; i++ {
		rm.storeWorkersCounter.Add(1)
		go rm.ParallelStore(rm.store)
	}
}

func (pm *StoreManager) ParallelStore(datas ValidatorStore) {
	defer pm.storeWorkersCounter.Done()

	pm.m.RunningWorkers.WithLabelValues("ParallelStore").Inc()
	defer pm.m.RunningWorkers.WithLabelValues("ParallelStore").Dec()

	ctx := context.Background()

	for payload := range pm.StoreCh {
		pm.m.StoreSize.Observe(float64(len(payload.Items)))
		if err := pm.storeRegistration(ctx, payload); err != nil {
			pm.l.Errorf("error storing registration - %w ", err)
			continue
		}
	}
}

func (pm *StoreManager) storeRegistration(ctx context.Context, payload StoreReq) (err error) {
	for _, i := range payload.Items {
		t := prometheus.NewTimer(pm.m.StoreTiming)
		now := time.Now()
		err := pm.store.PutNewerRegistration(ctx, i.Payload.Message.Pubkey, i.Payload)
		if err != nil {
			pm.m.StoreErrorRate.Inc()
			return err
		}
		pm.RegistrationCache.Add(i.Payload.Message.Pubkey, structs.ValidatorCacheEntry{
			Time:  now,
			Entry: i.Payload,
		})

		t.ObserveDuration()
	}
	return nil
}
