package client

import (
	"context"

	"github.com/blocknative/dreamboat/sim/client/types"
)

type Client interface {
	ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (node string, err error)
	ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (node string, err error)
	Kind() string
	ID() string
}
