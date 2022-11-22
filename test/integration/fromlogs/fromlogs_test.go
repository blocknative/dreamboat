package fromlogs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func localOrEnv(local string) string {
	if s := os.Getenv("RELAY_ADDRESS"); s != "" {
		return s
	}
	return local
}
func Test_bids(t *testing.T) {
	tests := []struct {
		name        string
		domain      string
		path        string
		networkType int
		wantErr     bool
	}{
		{
			name:        "one",
			path:        "./test.csv",
			networkType: NetworkDevnet,
			domain:      localOrEnv("0.0.0.0:18550"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			o, err := openFile(tt.path)
			defer o.Close()
			require.NoError(t, err)

			cli := http.Client{}
			payload, _, err := parseCSV(o)
			require.NoError(t, err)
			for k, v := range payload {
				if k.NetworkType != tt.networkType {
					continue
				}

				toTest := pickPayload(v)
				for _, p := range toTest {
					preAddress := fmt.Sprintf("http://%s/relay/v1/data/bidtraces/builder_blocks_received?slot=%d", tt.domain, p.Slot)
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, preAddress, nil)
					require.NoError(t, err)

					resp, err := cli.Do(req)
					require.NoError(t, err)

					bbR := []builderBlocksResponse{}
					dec := json.NewDecoder(resp.Body)
					err = dec.Decode(&bbR)
					resp.Body.Close()
					//b, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.NotEmpty(t, bbR)

					// relay-eth-devnet-0
					address := fmt.Sprintf("http://%s/eth/v1/builder/header/%d/%s/%s", tt.domain, p.Slot, bbR[0].ParentHash, bbR[0].ProposerPubkey)
					t.Logf("Testing: %d - address: %s", p.Slot, address)
					req, err = http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
					require.NoError(t, err)

					resp, err = cli.Do(req)
					require.NoError(t, err)

					ghR := GetHeaderResponse{}
					dec = json.NewDecoder(resp.Body)
					err = dec.Decode(&ghR)
					require.NoError(t, err)
					resp.Body.Close()
					log.Println("ghR", ghR)
					//f, err := io.ReadAll(resp.Body)
					//require.NoError(t, err)

					//log.Println("toTest", string(f))
					break
				}
				break
				//log.Println("toTest", toTest)
			}

		})
	}
}

type GetHeaderResponse struct {
	Data struct {
		Message struct {
			Header struct {
				BlockHash string `json:"block_hash"`
				Value     string `json:"value"`
			} `json:"header"`
		} `json:"message"`
	} `json:"data"`
}

type builderBlocksResponse struct {
	Slot           string `json:"slot"`
	ParentHash     string `json:"parent_hash"`
	ProposerPubkey string `json:"proposer_pubkey"`
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
