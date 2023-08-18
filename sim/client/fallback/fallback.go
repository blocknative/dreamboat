package fallback

import (
	"context"
	"errors"

	sim "github.com/blocknative/dreamboat/sim/client"
	"github.com/blocknative/dreamboat/sim/client/types"
)

type Fallback struct {
	clientsRPC  []sim.Client
	clientsWS   []sim.Client
	clientsHTTP []sim.Client
	atLeastOne  bool
	m           Metrics
}

func NewFallback() *Fallback {
	f := &Fallback{}
	f.initMetrics()
	return f
}

func (f *Fallback) IsSet() bool {
	return f.atLeastOne
}

func (f *Fallback) AddClient(cli sim.Client) {
	switch cli.Kind() {
	case "ws":
		f.clientsWS = addClient(f.clientsWS, cli)
	case "http":
		f.clientsHTTP = addClient(f.clientsHTTP, cli)
	case "rpc":
		f.clientsRPC = addClient(f.clientsRPC, cli)
	}
	f.atLeastOne = true
}

func (f *Fallback) RemoveClient(kind string, id string) {
	switch kind {
	case "ws":
		f.clientsWS = removeClient(f.clientsWS, id)
	case "http":
		f.clientsHTTP = removeClient(f.clientsHTTP, id)
	case "rpc":
		f.clientsRPC = removeClient(f.clientsRPC, id)
	}
	if len(f.clientsWS) == 0 && len(f.clientsHTTP) == 0 && len(f.clientsRPC) == 0 {
		f.atLeastOne = false
	}
}

func addClient(cSlice []sim.Client, cli sim.Client) []sim.Client {
	for _, c := range cSlice {
		if c.ID() == cli.ID() {
			return cSlice
		}
	}
	return append(cSlice, cli)
}

func removeClient(cSlice []sim.Client, id string) []sim.Client {
	for i, c := range cSlice {
		if c.ID() == id {
			return append(cSlice[:i], cSlice[i+1:]...)
		}
	}
	return cSlice
}

func (f *Fallback) ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error) {
	if !f.atLeastOne {
		f.m.ServedFrom.WithLabelValues("none", "error").Inc()
		return sim.ErrNotFound
	}

	var keepTrying bool
	for _, c := range f.clientsRPC {
		err, keepTrying = f.validateBlock(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	for _, c := range f.clientsWS {
		err, keepTrying = f.validateBlock(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	for _, c := range f.clientsHTTP {
		err, keepTrying = f.validateBlock(ctx, c, block)
		if !keepTrying {
			return err
		}
	}
	f.m.ServedFrom.WithLabelValues("all", "fatal").Inc()
	return err
}

func (f *Fallback) validateBlock(ctx context.Context, c sim.Client, block *types.BuilderBlockValidationRequest) (err error, keepTrying bool) {
	if ctx.Err() != nil {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
		return ctx.Err(), false
	}
	err = c.ValidateBlock(ctx, block)
	if err == nil {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "ok").Inc()
		return
	}

	if !(errors.Is(err, sim.ErrNotFound) || errors.Is(err, sim.ErrConnectionFailure)) {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "error").Inc()
		return err, false
	}

	f.m.ServedFrom.WithLabelValues(c.Kind(), "fallback").Inc()
	return err, true
}

func (f *Fallback) ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error) {
	if !f.atLeastOne {
		f.m.ServedFrom.WithLabelValues("none", "error").Inc()
		return sim.ErrNotFound
	}

	var keepTrying bool
	for _, c := range f.clientsRPC {
		err, keepTrying = f.validateBlockV2(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	for _, c := range f.clientsWS {
		err, keepTrying = f.validateBlockV2(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	for _, c := range f.clientsHTTP {
		err, keepTrying = f.validateBlockV2(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	f.m.ServedFrom.WithLabelValues("all", "fatal").Inc()
	return err
}

func (f *Fallback) validateBlockV2(ctx context.Context, c sim.Client, block *types.BuilderBlockValidationRequestV2) (err error, keepTrying bool) {
	if ctx.Err() != nil {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
		return ctx.Err(), false
	}
	err = c.ValidateBlockV2(ctx, block)
	if err == nil {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "ok").Inc()
		return
	}

	if !(errors.Is(err, sim.ErrNotFound) || errors.Is(err, sim.ErrConnectionFailure)) {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "error").Inc()
		return err, false
	}

	f.m.ServedFrom.WithLabelValues(c.Kind(), "fallback").Inc()
	return err, true
}

func (f *Fallback) ValidateBlockV3(ctx context.Context, block *types.BuilderBlockValidationRequestV3) (err error) {
	if !f.atLeastOne {
		f.m.ServedFrom.WithLabelValues("none", "error").Inc()
		return sim.ErrNotFound
	}

	var keepTrying bool
	for _, c := range f.clientsRPC {
		err, keepTrying = f.validateBlockV3(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	for _, c := range f.clientsWS {
		err, keepTrying = f.validateBlockV3(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	for _, c := range f.clientsHTTP {
		err, keepTrying = f.validateBlockV3(ctx, c, block)
		if !keepTrying {
			return err
		}
	}

	f.m.ServedFrom.WithLabelValues("all", "fatal").Inc()
	return err
}

func (f *Fallback) validateBlockV3(ctx context.Context, c sim.Client, block *types.BuilderBlockValidationRequestV3) (err error, keepTrying bool) {
	if ctx.Err() != nil {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "ctx").Inc()
		return ctx.Err(), false
	}
	err = c.ValidateBlockV3(ctx, block)
	if err == nil {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "ok").Inc()
		return
	}

	if !(errors.Is(err, sim.ErrNotFound) || errors.Is(err, sim.ErrConnectionFailure)) {
		f.m.ServedFrom.WithLabelValues(c.Kind(), "error").Inc()
		return err, false
	}

	f.m.ServedFrom.WithLabelValues(c.Kind(), "fallback").Inc()
	return err, true
}
