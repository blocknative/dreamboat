package relay

import (
	"github.com/blocknative/dreamboat/metrics"
)

type ServiceMetrics struct {
	/*ApiReqCounter *prometheus.CounterVec
	ApiReqTiming  *prometheus.HistogramVec
	*/
}

func (ds *DefaultService) initMetrics() {
	/*
		api.m.ApiReqCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "dreamboat",
			Subsystem: "api",
			Name:      "reqcount",
			Help:      "Number of requests.",
		}, []string{"endpoint", "code"})

		api.m.ApiReqTiming = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "dreamboat",
			Subsystem: "api",
			Name:      "duration",
			Help:      "Duration of requests per endpoint",
		}, []string{"endpoint"})
	*/
}

func (ds *DefaultService) AttachMetrics(m *metrics.Metrics) {
	ds.Relay.AttachMetrics(m)
	// m.Register(api.m.ApiReqCounter)
	// m.Register(api.m.ApiReqTiming)
}
