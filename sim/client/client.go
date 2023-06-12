package client

import (
	"context"

	"github.com/blocknative/dreamboat/sim/client/types"
)

type Client interface {
	ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error)
	ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error)
	Kind() string
	ID() string
}
