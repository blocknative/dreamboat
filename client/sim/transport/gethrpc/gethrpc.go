package gethrpc

import (
	"context"

	"github.com/blocknative/dreamboat/client/sim/types"
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

func (f *Client) IsSet() bool {
	return f.namespace != "" && f.rawurl != ""
}

func (c *Client) Kind() string {
	return "rpc"
}

func (c *Client) Dial(ctx context.Context) (err error) {
	c.C, err = rpc.DialContext(ctx, c.rawurl)
	return err
}

func (c *Client) ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error) {
	return c.validateBlock(ctx, "validateBuilderSubmissionV1", block)
}
func (c *Client) ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error) {
	return c.validateBlock(ctx, "validateBuilderSubmissionV2", block)
}

func (c *Client) validateBlock(ctx context.Context, method string, block any) (err error) {
	var intI error
	if err := c.C.CallContext(ctx, &intI, c.namespace+"_"+method, block); err != nil {
		return err
	}
	return intI
}
