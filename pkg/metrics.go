package relay

import (
	"github.com/blocknative/dreamboat/metrics"
)

type ServiceMetrics struct {
}

func (ds *Service) initMetrics() {
}

func (ds *Service) AttachMetrics(m *metrics.Metrics) {
	ds.Relay.AttachMetrics(m)
}
