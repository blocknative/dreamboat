package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type RelayMetrics struct {
	MissHeaderCount *prometheus.CounterVec
}

func (r *Relay) initMetrics() {
	r.m.MissHeaderCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "missHeader",
		Help:      "Number of missed headers by reason (oldSlot, noSubmission)",
	}, []string{"reason"})
}

func (r *Relay) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.MissHeaderCount)
}
