package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type APIMetrics struct {
	ApiReqCounter *prometheus.CounterVec
	ApiReqTiming  *prometheus.HistogramVec
}
