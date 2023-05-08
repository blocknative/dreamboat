package warehouse

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type WarehouseMetrics struct {
	Writes       *prometheus.CounterVec
	FailedWrites *prometheus.CounterVec
}

func (s *Warehouse) initMetrics() {
	s.m.Writes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "warehouse",
		Name:      "writes",
		Help:      "Number of writes by type of data",
	}, []string{"type"})

	s.m.FailedWrites = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "warehouse",
		Name:      "failedWrites",
		Help:      "Number of failed writes by type of data",
	}, []string{"type"})
}

func (s *Warehouse) AttachMetrics(m *metrics.Metrics) {
	m.Register(s.m.Writes)
	m.Register(s.m.FailedWrites)
}
