package metrics

import (
	met "github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func InitDatastoreMetrics(m *met.Metrics) error {
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
