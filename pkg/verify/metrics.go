package verify

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type ProcessManagerMetrics struct {
	VerifyTiming   *prometheus.HistogramVec
	RunningWorkers *prometheus.GaugeVec
}

func (rm *VerificationManager) initMetrics() {
	rm.m.RunningWorkers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "runningWorkers",
		Help:      "Number of requests.",
	}, []string{"type"})

	rm.m.VerifyTiming = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "verifyTiming",
		Help:      "Duration of requests per endpoint",
	}, []string{"type"})

}

func (rm *VerificationManager) AttachMetrics(m *metrics.Metrics) {
	m.Register(rm.m.VerifyTiming)
	m.Register(rm.m.RunningWorkers)
}
