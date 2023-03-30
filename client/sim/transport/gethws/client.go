package gethws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/blocknative/dreamboat/client"
	"github.com/blocknative/dreamboat/client/sim/types"
	"github.com/lthibault/log"
)

type Connectionner interface {
	Get() (*Conn, error)
}

type Client struct {
	nodeConn  Connectionner
	namespace string
	l         log.Logger
}

func NewClient(nodeConn Connectionner, namespace string, l log.Logger) *Client {
	return &Client{
		nodeConn:  nodeConn,
		namespace: namespace,
		l:         l,
	}
}

func (f *Client) IsSet() bool {
	return f.namespace != "" && f.nodeConn != nil
}

func (c *Client) Kind() string {
	return "ws"
}

func (c *Client) ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error) {
	return c.validateBlock(ctx, "validateBuilderSubmissionV1", block)
}
func (c *Client) ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error) {
	return c.validateBlock(ctx, "validateBuilderSubmissionV2", block)
}

func (c *Client) validateBlock(ctx context.Context, method string, block any) (err error) {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	conn, err := c.nodeConn.Get()
	if err != nil {
		return client.ErrNotFound
	}

	params, err := json.Marshal([]any{block})
	if err != nil {
		return err
	}

	resp, err := conn.RequestRPC(ctx, c.namespace+"_"+method, params)
	if err != nil {
		return client.ErrConnectionFailure //err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return errors.New(resp.Error.Message)
	}
	return nil
}
