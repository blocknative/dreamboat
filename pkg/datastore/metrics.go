package datastore

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func InitDatastoreMetrics(m *metrics.Metrics) error {
	return m.RegisterExpvar(map[string]*prometheus.Desc{
		"badger_v2_blocked_puts_total":   prometheus.NewDesc("badger_blocked_puts_total", "Blocked Puts", nil, nil),
		"badger_v2_disk_reads_total":     prometheus.NewDesc("badger_disk_reads_total", "Disk Reads", nil, nil),
		"badger_v2_disk_writes_total":    prometheus.NewDesc("badger_disk_writes_total", "Disk Writes", nil, nil),
		"badger_v2_gets_total":           prometheus.NewDesc("badger_gets_total", "Gets", nil, nil),
		"badger_v2_puts_total":           prometheus.NewDesc("badger_puts_total", "Puts", nil, nil),
		"badger_v2_memtable_gets_total":  prometheus.NewDesc("badger_memtable_gets_total", "Memtable gets", nil, nil),
		"badger_v2_lsm_size_bytes":       prometheus.NewDesc("badger_lsm_size_bytes", "LSM Size in bytes", []string{"database"}, nil),
		"badger_v2_vlog_size_bytes":      prometheus.NewDesc("badger_vlog_size_bytes", "Value Log Size in bytes", []string{"database"}, nil),
		"badger_v2_pending_writes_total": prometheus.NewDesc("badger_pending_writes_total", "Pending Writes", []string{"database"}, nil),
		"badger_v2_read_bytes":           prometheus.NewDesc("badger_read_bytes", "Read bytes", nil, nil),
		"badger_v2_written_bytes":        prometheus.NewDesc("badger_written_bytes", "Written bytes", nil, nil),
		"badger_v2_lsm_bloom_hits_total": prometheus.NewDesc("badger_lsm_bloom_hits_total", "LSM Bloom Hits", []string{"level"}, nil),
		"badger_v2_lsm_level_gets_total": prometheus.NewDesc("badger_lsm_level_gets_total", "LSM Level Gets", []string{"level"}, nil),
	})
}

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
