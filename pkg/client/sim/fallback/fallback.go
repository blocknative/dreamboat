package fallback

import (
	"context"
	"errors"

	"github.com/blocknative/dreamboat/pkg/client"
	"github.com/blocknative/dreamboat/pkg/client/sim/types"

	fbtypes "github.com/flashbots/go-boost-utils/types"
)

type Client interface {
	ValidateBlock(ctx context.Context, block *fbtypes.BuilderSubmitBlockRequest) (rrr types.RpcRawResponse, err error)
}

type Fallback struct {
	clients []Client
}

func NewFallback() *Fallback {
	return &Fallback{}
}

func (f *Fallback) AddClient(cli Client) {
	f.clients = append(f.clients, cli)
}

func (f *Fallback) Len() int {
	return len(f.clients)
}

func (f *Fallback) ValidateBlock(ctx context.Context, block *fbtypes.BuilderSubmitBlockRequest) (rrr types.RpcRawResponse, err error) {
	if len(f.clients) == 0 {
		return rrr, client.ErrNotFound
	}

	for _, c := range f.clients {
		rrr, err = c.ValidateBlock(ctx, block)
		if err == nil {
			if !(errors.Is(err, client.ErrNotFound) && errors.Is(err, client.ErrConnectionFailure)) {
				return rrr, err
			}
		}
	}
	return rrr, err
}
