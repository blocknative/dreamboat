package gethws

import (
	"context"

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

func (c *Client) ValidateBlock(ctx context.Context, params []byte) (rrr types.RpcRawResponse, err error) {

	conn, err := c.nodeConn.Get()
	if err != nil {
		return rrr, client.ErrNotFound
	}

	resp, err := conn.RequestRPC(ctx, c.namespace+"_validateBuilderSubmissionV1", params)
	if err != nil {
		return rrr, err
	}
	return resp, nil
}
