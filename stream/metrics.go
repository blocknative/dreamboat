package stream

import (
	"github.com/prometheus/client_golang/prometheus"
)

type streamMetrics struct {
	RecvCounter    *prometheus.CounterVec
	Timing         *prometheus.HistogramVec
	PublishSize    *prometheus.HistogramVec
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

	s.Metrics.Register(s.m.RecvCounter)
	s.Metrics.Register(s.m.Timing)
	s.Metrics.Register(s.m.PublishSize)
	s.Metrics.Register(s.m.PublishCounter)
}
