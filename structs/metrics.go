package structs

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusObserver interface {
	WithLabelValues(lvs ...string) prometheus.Observer
}

type MetricGroup struct {
	mu      sync.Mutex
	metrics []metric
}

type metric struct {
	dur    time.Duration
	labels []string
}

func NewMetricGroup(num int) *MetricGroup {
	return &MetricGroup{metrics: make([]metric, 0, num)}
}

func (mg *MetricGroup) Append(dur time.Duration, labels ...string) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	mg.metrics = append(mg.metrics, metric{dur: dur, labels: labels})
}

func (mg *MetricGroup) AppendSince(t time.Time, labels ...string) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	mg.metrics = append(mg.metrics, metric{dur: time.Since(t), labels: labels})
}

func (mg *MetricGroup) Observe(t PrometheusObserver) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	for _, metric := range mg.metrics {
		t.WithLabelValues(append(metric.labels, "")...).Observe(metric.dur.Seconds())
	}
}

func (mg *MetricGroup) ObserveWithError(t PrometheusObserver, err error) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	for _, metric := range mg.metrics {
		t.WithLabelValues(append(metric.labels, err.Error())...).Observe(metric.dur.Seconds())
	}
}
