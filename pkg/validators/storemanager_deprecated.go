package validators

/*
   DEPRECATED
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

func (rm *StoreManager) Get(k string) (value uint64, ok bool) {
	rm.lrtl.RLock()
	defer rm.lrtl.RUnlock()

	value, ok = rm.LastRegTime[k]
	return value, ok
}


func (rm *StoreManager) Set(k string, value uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	rm.LastRegTime[k] = value
	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
}


func (rm *StoreManager) LoadAll(m map[string]uint64) {
	rm.lrtl.Lock()
	defer rm.lrtl.Unlock()

	for k, v := range m {
		rm.LastRegTime[k] = v
	}

	rm.m.MapSize.Set(float64(len(rm.LastRegTime)))
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

*/
