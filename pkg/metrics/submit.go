package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type SubmitMetrics struct {
	VerifyTiming *prometheus.HistogramVec
}
