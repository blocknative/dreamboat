package dspostgres

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type PostgresMetrics struct {
	ErrorsCount *prometheus.CounterVec
}

func (s *Datastore) initMetrics() {
	s.m.ErrorsCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "datastore.postgres",
		Name:      "errors",
		Help:      "Number of errors while processing requests, that will not be propgated up the stack",
	}, []string{"method", "error"})
}

func (s *Datastore) AttachMetrics(m *metrics.Metrics) {
	m.Register(s.m.ErrorsCount)
}
