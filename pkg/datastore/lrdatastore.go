package datastore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blocknative/dreamboat/pkg/structs"
)

type LocalDatastore interface {
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockAndTrace, time.Duration) error
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockAndTrace, bool, error)
	CacheBlock(ctx context.Context, block *structs.CompleteBlockstruct) error
}

type RemoteDatastore interface {
	GetPayload(context.Context, structs.PayloadKey) (*structs.BlockAndTrace, bool, error)
	PutPayload(context.Context, structs.PayloadKey, *structs.BlockAndTrace, time.Duration) error
}

type LocalRemoteDatastore struct {
	Local  LocalDatastore
	Remote RemoteDatastore

	Logger log.Logger

	m LRDatastoreMetrics
}

func NewLocalRemoteDatastore(local LocalDatastore, remote RemoteDatastore, l log.Logger) *LocalRemoteDatastore {
	s := LocalRemoteDatastore{
		Local:  local,
		Remote: remote,
		Logger: l.WithField("relay-service", "stream-datastore"),
	}

	s.initMetrics()

	return &s
}

type getPayloadResponse struct {
	id        string
	block     *structs.BlockAndTrace
	isLocal   bool
	fromCache bool
	err       error
}

var chanPool = sync.Pool{
	New: func() any {
		return make(chan getPayloadResponse)
	},
}

func (s *LocalRemoteDatastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockAndTrace, bool, error) {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("getPayload", "all"))
	defer timer0.ObserveDuration()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses := chanPool.Get().(chan getPayloadResponse)
	defer chanPool.Put(responses)

	id := uuid.NewString()

	go func(ctx context.Context, resp chan getPayloadResponse, id string) {
		timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("getPayload", "local"))
		block, fromCache, err := s.Local.GetPayload(ctx, key)
		timer1.ObserveDuration()
		select {
		case responses <- getPayloadResponse{id: id, block: block, isLocal: true, fromCache: fromCache, err: err}:
		case <-ctx.Done():
		}
	}(ctx, responses, id)

	go func(ctx context.Context, resp chan getPayloadResponse, id string) {
		timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("getPayload", "remote"))
		block, isCache, err := s.Remote.GetPayload(ctx, key)
		timer1.ObserveDuration()
		select {
		case responses <- getPayloadResponse{id: id, block: block, isLocal: false, fromCache: isCache, err: err}:
		case <-ctx.Done():
		}
	}(ctx, responses, id)

	for i := 0; i < 2; i++ {
		var resp getPayloadResponse
		select {
		case resp = <-responses:
		case <-ctx.Done():
			return &structs.BlockAndTrace{}, false, ctx.Err()
		}

		if resp.id != id { // to avoid wrong responses from previous requests
			i -= 1
			continue
		}

		if resp.block != nil && resp.err == nil {
			if resp.isLocal {
				s.m.StreamPayloadHitCounter.WithLabelValues("local", "hit").Inc()
			} else {
				s.m.StreamPayloadHitCounter.WithLabelValues("remote", "hit").Inc()
			}
			return resp.block, resp.fromCache, resp.err
		}

		s.Logger.With(key).WithField("isLocal", resp.isLocal).WithError(resp.err).Debug("payload not found")
		if resp.isLocal {
			s.m.StreamPayloadHitCounter.WithLabelValues("local", "miss").Inc()
		} else {
			s.m.StreamPayloadHitCounter.WithLabelValues("remote", "miss").Inc()
		}
	}

	return nil, false, fmt.Errorf("payload not found")
}

func (s *LocalRemoteDatastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload *structs.BlockAndTrace, ttl time.Duration) error {
	timer0 := prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "all"))
	defer timer0.ObserveDuration()

	timer1 := prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "remoteStore"))
	if err := s.Remote.PutPayload(ctx, key, payload, ttl); err != nil {
		return err
	}
	timer1.ObserveDuration()

	timer1 = prometheus.NewTimer(s.m.Timing.WithLabelValues("putPayload", "localStore"))
	defer timer1.ObserveDuration()

	return s.Local.PutPayload(ctx, key, payload, ttl)
}
