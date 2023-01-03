package structs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusObserver interface {
	WithLabelValues(lvs ...string) prometheus.Observer
}

type MetricGroup map[string]time.Duration

func (mg MetricGroup) Observe(name string, dur time.Duration) {
	mg[name] = dur
}

func (mg MetricGroup) ObserveSince(name string, t time.Time) {
	mg[name] = time.Since(t)
}

func (mg MetricGroup) Commit(t PrometheusObserver, name string) {
	for metric, dur := range mg {
		t.WithLabelValues(name, metric, "").Observe(dur.Seconds())
	}
}

func (mg MetricGroup) CommitWithError(t PrometheusObserver, name string, err error) {
	for metric, dur := range mg {
		t.WithLabelValues(name, metric, err.Error()).Observe(dur.Seconds())
	}
}
