package validators

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type StoreManagerMetrics struct {
	StoreTiming    prometheus.Histogram
	StoreSize      prometheus.Histogram
	StoreErrorRate prometheus.Counter
	RunningWorkers *prometheus.GaugeVec
}

func (rm *StoreManager) initMetrics() {
	rm.m.RunningWorkers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "runningWorkers",
		Help:      "Number of requests.",
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
		Help:      "Size of stored",
	})

	rm.m.StoreErrorRate = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "storeError",
		Help:      "Number of errors",
	})

}

func (rm *StoreManager) AttachMetrics(m *metrics.Metrics) {
	m.Register(rm.m.StoreTiming)
	m.Register(rm.m.StoreSize)
	m.Register(rm.m.StoreErrorRate)
	m.Register(rm.m.RunningWorkers)
}

type RegisterMetrics struct {
	Timing                 *prometheus.HistogramVec
	RegistrationsCacheHits *prometheus.CounterVec
}

func (r *Register) initMetrics() {
	r.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "validators",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type"})

	r.m.RegistrationsCacheHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "validators",
		Name:      "registrationCache",
		Help:      "cache hit/miss",
	}, []string{"result"})

}

func (r *Register) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.Timing)
	m.Register(r.m.RegistrationsCacheHits)
}
