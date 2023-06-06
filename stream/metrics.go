package stream

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type StreamMetrics struct {
	RecvCounter *prometheus.CounterVec
	Timing      *prometheus.HistogramVec
	PublishSize *prometheus.HistogramVec
	PublishCounter *prometheus.CounterVec
}

func (s *Client) initMetrics() {
	s.m.RecvCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "recvcount",
		Help:      "Number of blocks received from stream.",
	}, []string{"type"})

	s.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type"})

	s.m.PublishSize = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "publishSize",
		Help:      "Size of the publications per function",
	}, []string{"function"})

	s.m.PublishCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "stream",
		Name:      "publishCounter",
		Help:      "Number of publications per function",
	}, []string{"function"})
}

func (s *Client) AttachMetrics(m *metrics.Metrics) {
	m.Register(s.m.RecvCounter)
	m.Register(s.m.Timing)
	m.Register(s.m.PublishSize)
	m.Register(s.m.PublishCounter)
}
