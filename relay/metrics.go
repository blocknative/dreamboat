package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type RelayMetrics struct {
	MissHeaderCount *prometheus.CounterVec
	CacheHitCount   *prometheus.CounterVec
}

func (r *Relay) initMetrics() {
	r.m.MissHeaderCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "missHeader",
		Help:      "Number of missed headers by reason (oldSlot, noSubmission)",
	}, []string{"reason"})
	r.m.CacheHitCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "cacheHit",
		Help:      "Number of cache hits/failures per function",
	}, []string{"function", "hit"})
}

func (r *Relay) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.MissHeaderCount)
	m.Register(r.m.CacheHitCount)
}
