package fallbackstore

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type LRDatastoreMetrics struct {
	StreamPayloadHitCounter *prometheus.CounterVec
	Timing                  *prometheus.HistogramVec
}

func (s *LocalRemoteDatastore) initMetrics() {
	s.m.StreamPayloadHitCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "payloadHit",
		Help:      "Number of payloads hit",
	}, []string{"source", "type"})

	s.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type"})
}

func (s *LocalRemoteDatastore) AttachMetrics(m *metrics.Metrics) {
	m.Register(s.m.StreamPayloadHitCounter)
	m.Register(s.m.Timing)
}
