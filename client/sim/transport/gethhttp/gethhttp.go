package gethhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/blocknative/dreamboat/client/sim/types"
	"github.com/lthibault/log"
)

type Client struct {
	namespace string
	client    *http.Client
	address   string
	l         log.Logger
}

func NewClient(address string, namespace string, l log.Logger) *Client {
	return &Client{
		namespace: namespace,
		address:   address,
		l:         l,
		client:    &http.Client{},
	}
}

func (c *Client) Kind() string {
	return "http"
}

func (f *Client) IsSet() bool {
	return f.namespace != "" && f.address != ""
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
	buff := new(bytes.Buffer)
	enc := json.NewEncoder(buff)
	if err := enc.Encode(
		types.RpcRequest{
			VersionTag: "2.0",
			ID:         1,
			Method:     c.namespace + "_" + method,
			Params:     []interface{}{block},
		}); err != nil {
		return err
	}
	resp, err := justsend(ctx, c.client, c.address, buff)
	if err != nil {
		return err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return errors.New(resp.Error.Message)
	}
	return nil
}

func justsend(ctx context.Context, client *http.Client, url string, body io.Reader) (rrr types.RpcRawResponse, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return rrr, err
	}
	req.Header.Add("Content-Type", "application/json; charset=utf-8")

	res, err := client.Do(req)
	if err != nil {
		return rrr, err
	}
	defer res.Body.Close()

	rrr = types.RpcRawResponse{}
	dec := json.NewDecoder(res.Body)
	err = dec.Decode(&rrr)
	return rrr, err
}
