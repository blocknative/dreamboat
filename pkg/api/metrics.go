package api

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type APIMetrics struct {
	ApiReqCounter *prometheus.CounterVec
	ApiReqTiming  *prometheus.HistogramVec
}

func (api *API) initMetrics() {
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
}

func (api *API) AttachMetrics(m *metrics.Metrics) {
	m.Register(api.m.ApiReqCounter)
	m.Register(api.m.ApiReqTiming)
}
