package gethws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/blocknative/dreamboat/sim/client"
	"github.com/blocknative/dreamboat/sim/client/types"
	"github.com/lthibault/log"
)

type Connectionner interface {
	Get() (*Conn, uint32, error)
	TryOtherThan(uint32) (*Conn, error)
}

type Client struct {
	nodeConn           Connectionner
	namespace          string
	tryOtherConnection bool
	l                  log.Logger
}

func NewClient(nodeConn Connectionner, namespace string, try bool, l log.Logger) *Client {
	return &Client{
		nodeConn:           nodeConn,
		namespace:          namespace,
		tryOtherConnection: try,
		l:                  l,
	}
}

func (c *Client) ID() string {
	return ""
}

func (f *Client) IsSet() bool {
	return f.namespace != "" && f.nodeConn != nil
}

func (c *Client) Kind() string {
	return "ws"
}

func (c *Client) ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (node string, err error) {
	return c.validateBlock(ctx, "validateBuilderSubmissionV1", block)
}
func (c *Client) ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (node string, err error) {
	return c.validateBlock(ctx, "validateBuilderSubmissionV2", block)
}

func (c *Client) validateBlock(ctx context.Context, method string, block any) (node string, err error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	conn, n, err := c.nodeConn.Get()
	if err != nil {
		return "", client.ErrNotFound
	}

	params, err := json.Marshal([]any{block})
	if err != nil {
		return "", err
	}

	node, err = c.trySend(ctx, conn, method, params)
	if c.tryOtherConnection && err == client.ErrConnectionFailure {
		tConn, iErr := c.nodeConn.TryOtherThan(n)
		if iErr != nil {
			return node, err
		}
		node, err = c.trySend(ctx, tConn, method, params)
	}

	return node, err
}

func (c *Client) trySend(ctx context.Context, conn *Conn, method string, params []byte) (node string, err error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	resp, err := conn.RequestRPC(ctx, c.namespace+"_"+method, params)
	if err != nil {
		return resp.Node, client.ErrConnectionFailure //err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return resp.Node, errors.New(resp.Error.Message)
	}
	return resp.Node, nil
}
