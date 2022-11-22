package fromlogs

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"
	"testing"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/stretchr/testify/require"
)

func Test_bids(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		networkType int
		wantErr     bool
	}{
		{
			name:        "one",
			path:        "./test.csv",
			networkType: NetworkDevnet,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			o, err := openFile(tt.path)
			defer o.Close()
			require.NoError(t, err)

			cli := http.Client{}
			anyPublic := types.PublicKey(random48Bytes())
			payload, _, err := parseCSV(o)
			require.NoError(t, err)
			for k, v := range payload {
				if k.NetworkType != tt.networkType {
					continue
				}

				toTest := pickPayload(v)
				for _, p := range toTest {
					address := fmt.Sprintf("http://relay-eth-devnet-0:18550/eth/v1/builder/header/%d/%s/%s", p.Slot, p.Blockhash, anyPublic)
					t.Logf("Testing: %d - address: %s", p.Slot, address)
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
					require.NoError(t, err)

					resp, err := cli.Do(req)
					require.NoError(t, err)
					b, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					log.Println("toTest", string(b))
					break
				}
				break
				//log.Println("toTest", toTest)
			}

		})
	}
}

func pickBid(in []RecordBid) []RecordBid {
	return in
}

func pickPayload(in []RecordPayload) []RecordPayload {
	return in
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}
