package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestAPI_MultiTxQueries(t *testing.T) {

	tests := []struct {
		name        string
		address     string
		expectedErr ExpectedResult
		methodName  []string
		d           []DecodeStringRequest
	}{
		{
			name:        "decode-db-abi",
			address:     "http://0.0.0.0:10000/decoder",
			expectedErr: ExpectedResult{200, ""},
			methodName:  []string{"swapExactETHForTokens", "swapExactETHForTokens"},
			d: []DecodeStringRequest{{
				To:    "0xd9e1ce17f2641f24ae83637ab66a2cca9c378b9e", // broken
				Input: "0x7ff36ab500000000000000000000000000000000000000000000001511114de9f41790350000000000000000000000000000000000000000000000000000000000000080000000000000000000000000aa2a95ee108eacb02904876bc6d8e66321ff25c500000000000000000000000000000000000000000000000000000000625479ed0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000c02aaa39b223fe8d0a0e5c4f27ead9083c756cc20000000000000000000000003155ba85d5f96b2d030a4966af206230e46849cb",
			}, {
				To:    "0xd9e1ce17f2641f24ae83637ab66a2cca9c378b9f",
				Input: "0x7ff36ab500000000000000000000000000000000000000000000001511114de9f41790350000000000000000000000000000000000000000000000000000000000000080000000000000000000000000aa2a95ee108eacb02904876bc6d8e66321ff25c500000000000000000000000000000000000000000000000000000000625479ed0000000000000000000000000000000000000000000000000000000000000002000000000000000000000000c02aaa39b223fe8d0a0e5c4f27ead9083c756cc20000000000000000000000003155ba85d5f96b2d030a4966af206230e46849cb",
			}},
		},
	}

	client := http.DefaultClient
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bytes.NewBuffer(nil)
			enc := json.NewEncoder(b)
			err := enc.Encode(tt.d)
			if err != nil {
				t.Errorf("http request encode error = %v", err)
				return
			}

			req, err := http.NewRequest(http.MethodGet, tt.address, b)
			if err != nil {
				t.Errorf("http request error = %v", err)
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("http request error = %v", err)
				return
			}

			if resp.StatusCode != 200 {
				t.Errorf("http request error = %v", err)
				return
			}

			content, _ := io.ReadAll(resp.Body)
			dec := json.NewDecoder(bytes.NewReader(content))
			qresp := []QResp{}
			if err = dec.Decode(&qresp); err != nil {
				t.Errorf("decode error = %v, (%s)", err, string(content))
				return
			}

			if qresp[0].Error != "error decoding body:  decode.capnp:ContractCallDecoder.decode: method not found" {
				t.Errorf("response error = expected error, content:  %s", string(content))
			}
			if qresp[1].MethodName != tt.methodName[1] {
				t.Errorf("response error = expected method name  %s, got %s content:  %s", tt.methodName[0], qresp[0].MethodName, string(content))
			}
		})
	}
}
