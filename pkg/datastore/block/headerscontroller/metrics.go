package headerscontroller

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type HeaderControllerMetrics struct {
	LatestSlot    prometheus.Gauge
	HeadersSize   prometheus.Gauge
	RemovalChecks prometheus.Gauge

	HeadersAdded prometheus.Counter
}

func (r *HeaderController) initMetrics() {
	r.m.LatestSlot = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "datastore",
		Name:      "latestSlot",
		Help:      "Latest slot submitted",
	})

	r.m.HeadersSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "datastore",
		Name:      "headersSize",
		Help:      "Header map sizes",
	})

	r.m.RemovalChecks = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "dreamboat",
		Subsystem: "datastore",
		Name:      "removalChecks",
		Help:      "content removal check",
	})

	r.m.HeadersAdded = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "datastore",
		Name:      "headersAdded",
		Help:      "new headers added",
	})
}

func (r *HeaderController) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.LatestSlot)
	m.Register(r.m.HeadersSize)
	m.Register(r.m.RemovalChecks)
	m.Register(r.m.HeadersAdded)
}
