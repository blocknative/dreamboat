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
}

func (rm *RegisteredManager) initMetrics() {
	/*rm.m.ApiReqCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "api",
		Name:      "reqcount",
		Help:      "Number of requests.",
	}, []string{"endpoint", "code"})
	*/
	rm.m.VerifyTiming = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredVerifyTiming",
		Help:      "Duration of requests per endpoint",
	})
}

func (rm *RegisteredManager) AttachMetrics(m *metrics.Metrics) {
	m.Register(rm.m.VerifyTiming)
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
