package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type ProcessManagerMetrics struct {
	VerifyTiming *prometheus.HistogramVec
	StoreTiming  prometheus.Histogram
	StoreSize    prometheus.Histogram

	MapSize prometheus.Gauge

	StoreErrorRate prometheus.Counter

	RunningWorkers *prometheus.GaugeVec
}

func (rm *ProcessManager) initMetrics() {
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

	rm.m.StoreTiming = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "storeTiming",
		Help:      "Duration of stores",
	})

	rm.m.StoreSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "storeSize",
		Help:      "Duration of stores",
	})

	rm.m.StoreErrorRate = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "storeError",
		Help:      "Duration of stores",
	})

	rm.m.MapSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "mapSize",
		Help:      "Size of internal map",
	})
}

func (rm *ProcessManager) AttachMetrics(m *metrics.Metrics) {
	m.Register(rm.m.StoreTiming)
	m.Register(rm.m.StoreSize)
	m.Register(rm.m.StoreErrorRate)
	m.Register(rm.m.VerifyTiming)
	m.Register(rm.m.RunningWorkers)
	m.Register(rm.m.MapSize)
}

type RelayMetrics struct {
	Timing *prometheus.HistogramVec
}

func (r *Relay) initMetrics() {
	r.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type"})
}

func (r *Relay) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.Timing)
}
