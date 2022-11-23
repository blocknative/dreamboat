package fromlogs

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func localOrEnv(local string) string {
	if s := os.Getenv("RELAY_ADDRESS"); s != "" {
		return s
	}
	return local
}

func Test_payoads(t *testing.T) {
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
			networkType: NetworkMainnet,
			domain:      localOrEnv("0.0.0.0:18550"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			o, err := openFile(tt.path)
			defer o.Close()
			require.NoError(t, err)

			parsed, err := parseCSV(o)
			require.NoError(t, err)

			// Just Logs
			usecaseCorrectMaxProfit(t, tt.networkType, parsed)
			usecaseCorrectPayloadDelivered(t, tt.networkType, parsed)
			usecaseNoSubmissionNoBids(t, tt.networkType, parsed)
			usecasePayloadNotFoundOnlyAfterNoBid(t, tt.networkType, parsed)

			//ctx := context.Background()
			//cli := http.Client{}
			/*
				for k, v := range parsed.Bids {
					if k.NetworkType != tt.networkType {
						continue
					}

					toTest := pickPayload(v)
					for _, p := range toTest {
						t.Logf("Querying builder blocks for: %d, block: %s", p.Slot, p.Blockhash)
						preAddress := fmt.Sprintf("http://%s/relay/v1/data/bidtraces/builder_blocks_received?block_hash=%s", tt.domain, p.Blockhash)
						req, err := http.NewRequestWithContext(ctx, http.MethodGet, preAddress, nil)
						require.NoError(t, err)

						resp, err := cli.Do(req)
						require.NoError(t, err)

						bbR := []builderBlocksResponse{}
						dec := json.NewDecoder(resp.Body)
						err = dec.Decode(&bbR)
						resp.Body.Close()

						require.NoError(t, err)
						require.NotEmpty(t, bbR)

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

						require.Equal(t, p.Bid.String(), ghR.Data.Message.Value)
						require.Equal(t, p.Blockhash, ghR.Data.Message.Header.BlockHash)
					}
				}*/

		})
	}
}

type GetHeaderResponse struct {
	Data struct {
		Message struct {
			Header struct {
				BlockHash string `json:"block_hash"`
			} `json:"header"`
			Value string `json:"value"`
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

// Check the max profit bid is being sent on GetHeader
func usecaseCorrectMaxProfit(t *testing.T, env int, pr *ParsedResult) {
	// Get all bid sent logs
	// For all logs, get all builder block stored logs for the same slot, which were previous or equal to the bid sent log
	// Calculate a list of candidate max profit blocks for that slot by:
	// - Calculate the what blocks have been max profit in the following time window [header requested, bid sent], including both timestamps
	// Confirm the bid that is sent has been "flaged" as max profit at certain moment in the aforementioned timestamps

	// Calculate the max profit for that slot using the block stored logs
	// Confirm the bid that is sent is the expected max profit

	if len(pr.Bids) == 0 {
		t.Log("[WARN] 'bid sent' not found in the test file")
		return
	}

BIDLOOP:
	for k, bids := range pr.Bids {
		if k.NetworkType != env {
			continue
		}

		bbs, ok := pr.BuilderBlockStored[k]
		if !ok {
			t.Logf("[WARN] no 'builder block stored' logs for slot (%d)", k.Slot)
			// should we fail here
			continue
		}
		hreq, ok := pr.HeaderRequested[k]
		if !ok {
			t.Logf("[WARN] no 'header requested' logs for slot (%d)", k.Slot)
			// should we fail here
			continue
		}

		for _, bid := range bids {
			headerRequested := hreq[0]
			if len(hreq) > 1 {
				for _, h := range hreq {
					if h.Time.Before(bid.Time) {
						headerRequested = h
					}
				}
			}

			var maxCandidatesSent, maxCandidatesRequested []RecordPayload
			for _, v := range bbs {
				m := v
				if v.Time.UnixMicro() <= bid.Time.UnixMicro() {
					maxCandidatesSent = append(maxCandidatesSent, m)
				}

				if v.Time.UnixMicro() <= headerRequested.Time.UnixMicro() {
					maxCandidatesRequested = append(maxCandidatesRequested, m)
				}
			}

			var maxRequested RecordPayload
			maxSent := recreateMax(maxCandidatesSent)

			if bid.Bid.Cmp(maxSent.Bid) != 0 {
				// check if the record was not submitted in log
				// with literally the same time
				if maxSent.Time.UnixMicro() == bid.Time.UnixMicro() {
					continue BIDLOOP
				}

				maxRequested = recreateMax(maxCandidatesRequested)
				if bid.Bid.Cmp(maxRequested.Bid) == 0 {
					continue BIDLOOP
				} else if bid.Time.UnixMicro() >= maxRequested.Time.UnixMicro() &&
					bid.Time.UnixMicro() >= maxSent.Time.UnixMicro() {
					continue BIDLOOP
				}
			}
			var info []struct {
				Value uint64
				Time  int64
			}
			for _, i := range maxCandidatesSent {
				info = append(info, struct {
					Value uint64
					Time  int64
				}{
					i.Bid.Uint64(),
					i.Time.UnixMicro(),
				})
			}
			//t.Errorf("Maximum payload is different for slot (%d): %+v , returned( %+v) , ", k.Slot, max.Bid, bid.Bid)
			t.Errorf("Maximum payload is different for slot (%d) - requested: %+v sent: %+v , returned( %+v) , all (%+v) ", k.Slot, maxRequested, maxSent, bid, info)
		}
	}
}

// Check correct payload is delivered on GetPayload
func usecaseCorrectPayloadDelivered(t *testing.T, env int, pr *ParsedResult) {
	// Get all payload sent logs
	// For each log get the previous payload requested log
	// Confirm the blockHash is the same for both payload sent and payload requested

	if len(pr.Payloads) == 0 {
		t.Log("[WARN] 'payload sent' not found in the test file")
		return
	}

	for k, payload := range pr.Payloads {
		if k.NetworkType != env {
			continue
		}

		pReq := pr.PayloadRequested[k]
		for _, p := range payload {
			var found bool
			for _, req := range pReq {
				if p.Blockhash == req.Blockhash {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Payload send and payload requested is different for slot (%d): %+v , all requested( %+v) ", k.Slot, p, pReq)
				return
			}
		}
	}

}

// Check payload not found error only occurs if previously no bid is sent
func usecasePayloadNotFoundOnlyAfterNoBid(t *testing.T, env int, pr *ParsedResult) {
	// Get all no payload found logs
	// For all logs, confirm there isn't any bid sent log is found with the same blockHash

	if len(pr.NoPayloadFound) == 0 {
		t.Log("[WARN] 'no payloads found' not found in the test file")
		return
	}

	for k, noPayload := range pr.NoPayloadFound {
		if k.NetworkType != env {
			continue
		}

		for _, v := range noPayload {
			blocks := pr.Bids[k]           // should we check all logs or just this slot
			for _, block := range blocks { // should we test for "before" here? like timestamp relation?
				if v.Blockhash != block.Blockhash {
					require.NotEqual(t, v.Blockhash, block.Blockhash)
					return
				}
			}
		}
	}
}

// Check bids not found only occur if there was no submission
func usecaseNoSubmissionNoBids(t *testing.T, env int, pr *ParsedResult) {
	///Get all no builder bid logs
	///For all logs, confirm there is no previous builder block stored log for the same slot
	if len(pr.NoBids) == 0 {
		t.Log("[WARN] 'No bids' not found in the test file")
		return
	}

	for k, _ := range pr.NoBids {
		if k.NetworkType != env {
			continue
		}
		blocks := pr.BuilderBlockStored[k]
		require.Len(t, blocks, 0)
		// should we test for "before" here? like timestamp relation?
	}
}

func recreateMax(r []RecordPayload) RecordPayload {
	submissionsByPubKeys := make(map[string]RecordPayload)

	// sort by timestamp
	sort.Slice(r, func(i, j int) bool {
		return r[i].Time.UnixMicro() > r[j].Time.UnixMicro()
	})

	var maxProfit RecordPayload
	for _, newEl := range r {
		submissionsByPubKeys[newEl.Builder] = newEl

		// we should allow resubmission
		if maxProfit.Builder == newEl.Builder {
			for _, submission := range submissionsByPubKeys {
				if maxProfit.Bid == nil || maxProfit.Bid.Cmp(submission.Bid) <= 0 {
					maxProfit = submission
				}
			}
		} else {
			if maxProfit.Bid == nil ||
				maxProfit.Bid.Cmp(newEl.Bid) < 0 {
				maxProfit = newEl
			}
		}

	}
	return maxProfit
}
