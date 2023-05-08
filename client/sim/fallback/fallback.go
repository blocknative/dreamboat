package fallback

import (
	"context"
	"errors"

	"github.com/blocknative/dreamboat/client"
	"github.com/blocknative/dreamboat/client/sim/types"
)

type Client interface {
	ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error)
	ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error)
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

func (f *Fallback) IsSet() bool {
	return len(f.clients) > 0
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
		if ctx.Err() != nil {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
			return ctx.Err()
		}
		err = c.ValidateBlock(ctx, block)
		if err == nil {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ok").Inc()
			return
		}

		if !(errors.Is(err, client.ErrNotFound) || errors.Is(err, client.ErrConnectionFailure)) {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "error").Inc()
			return err
		}

		f.m.ServedFrom.WithLabelValues(c.Kind(), "fallback").Inc()
	}
	return err
}

func (f *Fallback) ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error) {
	if len(f.clients) == 0 {
		f.m.ServedFrom.WithLabelValues("none", "error").Inc()
		return client.ErrNotFound
	}

	for _, c := range f.clients {
		if ctx.Err() != nil {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
			return ctx.Err()
		}
		err = c.ValidateBlockV2(ctx, block)
		if err == nil {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ok").Inc()
			return
		}

		if ctx.Err() != nil {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
			return err
		}

		if !(errors.Is(err, client.ErrNotFound) || errors.Is(err, client.ErrConnectionFailure)) {
			f.m.ServedFrom.WithLabelValues(c.Kind(), "error").Inc()
			return err
		}

		f.m.ServedFrom.WithLabelValues(c.Kind(), "fallback").Inc()
	}
	return err
}
