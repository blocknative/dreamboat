package fallback

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	ServedFrom *prometheus.CounterVec
}

func (fb *Fallback) initMetrics() {
	fb.m.ServedFrom = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "simclient",
		Name:      "requestSource",
		Help:      "Number of calls being done by transport type",
	}, []string{"kind", "node", "result"})
}

func (fb *Fallback) AttachMetrics(m *metrics.Metrics) {
	m.Register(fb.m.ServedFrom)
}
