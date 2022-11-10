package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type SubmitMetrics struct {
	VerifyTiming *prometheus.HistogramVec
}

type RegisteredManagerMetrics struct {
	VerifyTiming prometheus.Histogram

	RunningWorkers *prometheus.GaugeVec
}

func (rm *RegisteredManager) initMetrics() {
	/*rm.m.ApiReqCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "api",
		Name:      "reqcount",
		Help:      "Number of requests.",
	}, []string{"endpoint", "code"})
	*/

	rm.m.RunningWorkers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredRunningWorkers",
		Help:      "Number of requests.",
	}, []string{"type"})

	rm.m.VerifyTiming = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredVerifyTiming",
		Help:      "Duration of requests per endpoint",
	})
}

func (rm *RegisteredManager) AttachMetrics(m *metrics.Metrics) {
	m.Register(rm.m.VerifyTiming)
	m.Register(rm.m.RunningWorkers)
}

func (r *Relay) initMetrics() {
	/*rm.m.ApiReqCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "api",
		Name:      "reqcount",
		Help:      "Number of requests.",
	}, []string{"endpoint", "code"})
	*/
	/*rm.m.VerifyTiming = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredVerifyTiming",
		Help:      "Duration of requests per endpoint",
	})*/
}

func (r *Relay) AttachMetrics(m *metrics.Metrics) {

}
