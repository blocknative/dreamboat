package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type RelayMetrics struct {
	MissHeaderCount *prometheus.CounterVec
	CacheHitCount   *prometheus.CounterVec
	RetryCount      *prometheus.CounterVec
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

	r.m.RetryCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "retry",
		Help:      "Number of retries/duplicate requests by endpoint (getPayload, getHeader)",
	}, []string{"endpoint"})
}

func (r *Relay) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.MissHeaderCount)
	m.Register(r.m.CacheHitCount)
	m.Register(r.m.RetryCount)
}
