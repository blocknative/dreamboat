package gethws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/blocknative/dreamboat/pkg/client"
	"github.com/blocknative/dreamboat/pkg/client/sim/types"
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
	conn, err := c.nodeConn.Get()
	if err != nil {
		return client.ErrNotFound
	}

	params, err := json.Marshal([]*types.BuilderBlockValidationRequest{block})
	if err != nil {
		return err
	}

	resp, err := conn.RequestRPC(ctx, c.namespace+"_validateBuilderSubmissionV1", params)
	if err != nil {
		return client.ErrConnectionFailure //err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return errors.New(resp.Error.Message)
	}
	return nil
}
