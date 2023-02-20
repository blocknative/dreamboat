package datastore

/*



func (s *Datastore) GetDelivered(ctx context.Context, query structs.PayloadQuery) (structs.BidTraceWithTimestamp, error) {
	key, err := s.queryToDeliveredKey(ctx, query)
	if err != nil {
		return structs.BidTraceWithTimestamp{}, err
	}
	return s.getDelivered(ctx, key)
}

func (s *Datastore) getDelivered(ctx context.Context, key ds.Key) (structs.BidTraceWithTimestamp, error) {
	data, err := s.TTLStorage.Get(ctx, key)
	if err != nil {
		return structs.BidTraceWithTimestamp{}, err
	}

	var trace structs.BidTraceWithTimestamp
	err = json.Unmarshal(data, &trace)
	return trace, err
}

func (s *Datastore) GetDeliveredBatch(ctx context.Context, queries []structs.PayloadQuery) ([]structs.BidTraceWithTimestamp, error) {
	keys := make([]ds.Key, 0, len(queries))
	for _, query := range queries {
		key, err := s.queryToDeliveredKey(ctx, query)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	batch, err := s.TTLStorage.GetBatch(ctx, keys)
	if err != nil {
		return nil, err
	}

	traceBatch := make([]structs.BidTraceWithTimestamp, 0, len(batch))
	for _, data := range batch {
		var trace structs.BidTraceWithTimestamp
		if err = json.Unmarshal(data, &trace); err != nil {
			return nil, err
		}
		traceBatch = append(traceBatch, trace)
	}

	return traceBatch, err
}

func (s *Datastore) PutDelivered(ctx context.Context, slot structs.Slot, trace structs.DeliveredTrace, ttl time.Duration) error {
	data, err := json.Marshal(trace.Trace)
	if err != nil {
		return err
	}

	txn := s.Badger.NewTransaction(true)
	defer txn.Discard()
	if err := txn.SetEntry(badger.NewEntry(DeliveredHashKey(trace.Trace.BlockHash).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredNumKey(trace.BlockNumber).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredPubkeyKey(trace.Trace.ProposerPubkey).Bytes(), DeliveredKey(slot).Bytes()).WithTTL(ttl)); err != nil {
		return err
	}
	if err := txn.SetEntry(badger.NewEntry(DeliveredKey(slot).Bytes(), data).WithTTL(ttl)); err != nil {
		return err
	}

	return txn.Commit()
}

func (s *Datastore) queryToDeliveredKey(ctx context.Context, query structs.PayloadQuery) (ds.Key, error) {
	var (
		rawKey []byte
		err    error
	)

	if (query.BlockHash != types.Hash{}) {
		rawKey, err = s.TTLStorage.Get(ctx, DeliveredHashKey(query.BlockHash))
	} else if query.BlockNum != 0 {
		rawKey, err = s.TTLStorage.Get(ctx, DeliveredNumKey(query.BlockNum))
	} else if (query.PubKey != types.PublicKey{}) {
		rawKey, err = s.TTLStorage.Get(ctx, DeliveredPubkeyKey(query.PubKey))
	} else {
		rawKey = DeliveredKey(query.Slot).Bytes()
	}

	if err != nil {
		return ds.Key{}, err
	}
	return ds.NewKey(string(rawKey)), nil
}


func DeliveredKey(slot structs.Slot) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-%d", slot))
}

func DeliveredHashKey(bh types.Hash) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-hash-%s", bh.String()))
}

func DeliveredNumKey(bn uint64) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-num-%d", bn))
}

func DeliveredPubkeyKey(pk types.PublicKey) ds.Key {
	return ds.NewKey(fmt.Sprintf("delivered-pk-%s", pk.String()))
}


func (s *Datastore) CheckSlotDelivered(ctx context.Context, slot uint64) (bool, error) {
	tx := s.Badger.NewTransaction(false)
	defer tx.Discard()

	_, err := tx.Get(DeliveredKey(structs.Slot(slot)).Bytes())
	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	return (err == nil), err
}
*/

/*

func (s *Datastore) GetHeadersBySlot(ctx context.Context, slot uint64) ([]structs.HeaderAndTrace, error) {
	el, _ := s.hc.GetHeaders(slot, slot, 1)
	if el != nil {
		return el, nil
	}

	data, err := s.TTLStorage.Get(ctx, HeaderKey(slot))
	if err != nil && errors.Is(err, ds.ErrNotFound) {
		return el, err
	}

	el = []structs.HeaderAndTrace{}
	if err = json.Unmarshal(data, &el); err != nil {
		return el, err
	}

	return el, err
}

func (s *Datastore) GetHeadersByBlockNum(ctx context.Context, blockNumber uint64) ([]structs.HeaderAndTrace, error) {
	slot, err := s.TTLStorage.Get(ctx, HeaderNumKey(blockNumber))
	if err != nil {
		return nil, err
	}
	return s.GetHeadersBySlot(ctx, binary.LittleEndian.Uint64(slot))
}

func (s *Datastore) GetHeadersByBlockHash(ctx context.Context, hash types.Hash) ([]structs.HeaderAndTrace, error) {
	slot, err := s.TTLStorage.Get(ctx, HeaderHashKey(hash))
	if err != nil {
		return nil, err
	}

	newContent, err := s.TTLStorage.Get(ctx, HeaderKeyContent(binary.LittleEndian.Uint64(slot), hash.String()))
	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) { // do not fail on not found try others
			return nil, err
		}
		// old code fallback - to be removed
		if true {
			newContent, err = s.TTLStorage.Get(ctx, HeaderKey(binary.LittleEndian.Uint64(slot)))
			if err != nil {
				return nil, err
			}

			el := []structs.HeaderAndTrace{}
			if err = json.Unmarshal(newContent, &el); err != nil {
				return el, err
			}

			newEl := []structs.HeaderAndTrace{}
			for _, v := range el {
				if v.Header.BlockHash == hash {
					elem := v
					newEl = append(newEl, elem)
				}
			}
			return newEl, nil
		}

	}

	el := structs.HeaderAndTrace{}
	if err = json.Unmarshal(newContent, &el); err != nil {
		return nil, err
	}
	return []structs.HeaderAndTrace{el}, nil
}

func (s *Datastore) GetLatestHeaders(ctx context.Context, limit uint64, stopLag uint64) ([]structs.HeaderAndTrace, error) {
	ls := s.hc.GetLatestSlot()
	stop := ls - stopLag
	el, lastSlot := s.hc.GetHeaders(ls, stop, int(limit))

	if el == nil {
		el = []structs.HeaderAndTrace{}
	}

	// all from memory
	if len(el) >= int(limit) {
		return el[:limit], nil
	}

	initialSlot := lastSlot
	readr := bytes.NewReader(nil)
	dec := json.NewDecoder(readr)
	for {
		data, err := s.TTLStorage.Get(ctx, HeaderKey(initialSlot))
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				return el, nil
			}
			return el, err
		}
		readr.Reset(data)
		hnt := []structs.HeaderAndTrace{}
		if err := dec.Decode(&hnt); err != nil {
			return nil, err
		}

		el = append(el, hnt...)
		initialSlot--
		// introduce limit?
	}
}
*/
