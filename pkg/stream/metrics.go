package stream

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type StreamMetrics struct {
	StreamRecvCounter       *prometheus.CounterVec
	StreamPayloadHitCounter *prometheus.CounterVec
	Timing                  *prometheus.HistogramVec
}

func (s *StreamDatastore) initMetrics() {
	s.m.StreamRecvCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "recvcount",
		Help:      "Number of blocks received from stream.",
	}, []string{"type"})

	s.m.StreamPayloadHitCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "payloadHit",
		Help:      "Number of payloads hit",
	}, []string{"source","type"})

	s.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type", "error"})
}

func (s *StreamDatastore) AttachMetrics(m *metrics.Metrics) {
	m.Register(s.m.StreamRecvCounter)
	m.Register(s.m.StreamPayloadHitCounter)
	m.Register(s.m.Timing)
}
