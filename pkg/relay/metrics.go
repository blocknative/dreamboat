package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type ProcessManagerMetrics struct {
	VerifyTiming *prometheus.HistogramVec

	MapSize prometheus.Gauge

	RunningWorkers *prometheus.GaugeVec
}

func (rm *ProcessManager) initMetrics() {
	rm.m.RunningWorkers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredRunningWorkers",
		Help:      "Number of requests.",
	}, []string{"type"})

	rm.m.VerifyTiming = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredVerifyTiming",
		Help:      "Duration of requests per endpoint",
	}, []string{"type"})

	rm.m.MapSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "registeredMapSize",
		Help:      "Duration of requests per endpoint",
	})
}

func (rm *ProcessManager) AttachMetrics(m *metrics.Metrics) {
	m.Register(rm.m.VerifyTiming)
	m.Register(rm.m.RunningWorkers)
	m.Register(rm.m.MapSize)
}

type RelayMetrics struct {
	VerifyTiming prometheus.Histogram
	OtherTiming  prometheus.Histogram
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
