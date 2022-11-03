package metrics

import (
	"database/sql"
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "expvar"
)

type Metrics struct {
	registry  *prometheus.Registry
	gatherers prometheus.Gatherers
}

func NewMetrics() (m *Metrics) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.Register(collectors.NewGoCollector())

	return &Metrics{registry: reg, gatherers: prometheus.Gatherers{reg}}
}

func (m *Metrics) RegisterDB(db *sql.DB, dbName string) error {
	return m.registry.Register(collectors.NewDBStatsCollector(db, dbName))
}

func (m *Metrics) RegisterExpvar(exports map[string]*prometheus.Desc) error {
	return m.registry.Register(collectors.NewExpvarCollector(exports))
}

func (m *Metrics) Register(cs prometheus.Collector) error {
	return m.registry.Register(cs)
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(
		m.gatherers[0],
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
}

func AttachProfiler(sm *http.ServeMux) {
	sm.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	sm.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	sm.HandleFunc("/debug/pprof/trace", pprof.Trace)
	sm.HandleFunc("/debug/pprof/profile", pprof.Profile)
	sm.HandleFunc("/debug/pprof/", pprof.Index)
}
