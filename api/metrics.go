package api

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type APIMetrics struct {
	ApiReqCounter *prometheus.CounterVec
	ApiReqTiming  *prometheus.HistogramVec
	ApiReqElCount *prometheus.HistogramVec

	RelayTiming *prometheus.HistogramVec
}

func (api *API) initMetrics() {
	api.m.ApiReqCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "api",
		Name:      "reqcount",
		Help:      "Number of requests.",
	}, []string{"endpoint", "code", "type"})

	api.m.ApiReqTiming = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "api",
		Name:      "duration",
		Help:      "Duration of requests per endpoint",
	}, []string{"endpoint"})

	api.m.ApiReqElCount = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "api",
		Name:      "reqElCount",
		Help:      "Counts of elements received per endpoint",
	}, []string{"endpoint", "type"})

	api.m.RelayTiming = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type", "error"})
}

func (api *API) AttachMetrics(m *metrics.Metrics) {
	m.Register(api.m.ApiReqCounter)
	m.Register(api.m.ApiReqTiming)
	m.Register(api.m.ApiReqElCount)
	m.Register(api.m.RelayTiming)
}
