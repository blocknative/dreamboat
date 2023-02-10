package fallback

import (
	"context"
	"errors"

	"github.com/blocknative/dreamboat/pkg/client"
	"github.com/blocknative/dreamboat/pkg/client/sim/types"
)

type Client interface {
	ValidateBlock(ctx context.Context, params []byte) (rrr types.RpcRawResponse, err error)
}

type Fallback struct {
	primary   Client
	secondary Client
}

func NewFallback(primary Client, secondary Client) *Fallback {
	return &Fallback{primary, secondary}
}

func (f *Fallback) ValidateBlock(ctx context.Context, params []byte) (rrr types.RpcRawResponse, err error) {
	if f.primary != nil {
		rrr, err = f.primary.ValidateBlock(ctx, params)
	}

	if f.secondary != nil && err != nil {
		if errors.Is(err, client.ErrNotFound) || errors.Is(err, client.ErrConnectionFailure) { // separate just for readability
			rrr, err = f.secondary.ValidateBlock(ctx, params)
		}
	}

	return rrr, err
}
