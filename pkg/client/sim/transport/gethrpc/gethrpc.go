package gethrpc

import (
	"context"

	"github.com/blocknative/dreamboat/pkg/client/sim/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type Client struct {
	rawurl    string
	namespace string
	C         *rpc.Client
}

func NewClient(namespace string, rawurl string) *Client {
	return &Client{
		rawurl:    rawurl,
		namespace: namespace,
	}
}

func (c *Client) Dial(ctx context.Context) (err error) {
	c.C, err = rpc.DialContext(ctx, c.rawurl)
	return err
}

func (c *Client) ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (rrr types.RpcRawResponse, err error) {
	var intI error
	if err := c.C.CallContext(ctx, &intI, c.namespace+"_validateBuilderSubmissionV1", block); err != nil {
		return rrr, err
	}
	return types.RpcRawResponse{}, intI
}
