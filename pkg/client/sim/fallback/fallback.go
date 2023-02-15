package fallback

import (
	"context"
	"errors"

	"github.com/blocknative/dreamboat/pkg/client"
	"github.com/blocknative/dreamboat/pkg/client/sim/types"
)

type Client interface {
	ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error)
	Kind() string
}

type Fallback struct {
	clients []Client
	m       Metrics
}

func NewFallback() *Fallback {
	f := &Fallback{}
	f.initMetrics()
	return f
}

func (f *Fallback) AddClient(cli Client) {
	f.clients = append(f.clients, cli)
}

func (f *Fallback) Len() int {
	return len(f.clients)
}

func (f *Fallback) ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error) {
	if len(f.clients) == 0 {
		f.m.ServedFrom.WithLabelValues("none", "error").Inc()
		return client.ErrNotFound
	}

	for _, c := range f.clients {
		err = c.ValidateBlock(ctx, block)
		if err == nil {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ok").Inc()
			return
		}

		if !(errors.Is(err, client.ErrNotFound) && errors.Is(err, client.ErrConnectionFailure)) {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "error").Inc()
			return err
		}

		f.m.ServedFrom.WithLabelValues(c.Kind(), "fallback").Inc()
	}
	return err
}
