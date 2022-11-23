package fromlogs

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	NetworkMainnet = iota + 1
	NetworkGoerli
	NetworkRopsten
	NetworkDevnet
)

type RecordPayload struct {
	NetworkType int
	Time        time.Time
	Bid         *big.Int
	Slot        uint64
	Blockhash   string
	Builder     string
}

type RecordPosition struct {
	Date      int
	Host      int
	Service   int
	Bid       int
	BidValue  int
	Blockhash int
	Slot      int
	Message   int
	Builder   int
}

type RecordBid struct {
	NetworkType int
	Time        time.Time
	Bid         *big.Int
	Slot        uint64
	Blockhash   string
}

func openFile(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	return file, err
}

func parseHeader(headers []string) (rp RecordPosition, err error) {
	for i, v := range headers {
		switch v {
		case "Date":
			rp.Date = i
		case "Host":
			rp.Host = i
		case "Service":
			rp.Service = i
		case "bid":
			rp.Bid = i
		case "@bidValue":
			rp.BidValue = i
		case "@blockHash":
			rp.Blockhash = i
		case "@slot":
			rp.Slot = i
		case "@builder":
			rp.Builder = i
		case "Message":
			rp.Message = i
		}
	}

	if rp.Bid == 0 || rp.BidValue == 0 || rp.Blockhash == 0 || rp.Slot == 0 || rp.Message == 0 {
		return rp, fmt.Errorf("not all fields included in headers: %v ", rp)
	}
	return rp, nil
}

type Key struct {
	Slot        uint64
	NetworkType int
}

type ParsedResult struct {
	// "bid sent":
	Bids map[Key][]RecordBid
	// "no builder bid":
	NoBids map[Key][]RecordBid
	// "header requested":
	HeaderRequested map[Key][]RecordBid
	// "payload sent":
	Payloads map[Key][]RecordPayload
	// "builder block stored":
	BuilderBlockStored map[Key][]RecordPayload
	// "payload requested":
	PayloadRequested map[Key][]RecordPayload
	// "no payload found":
	NoPayloadFound map[Key][]RecordPayload
}

func NewParsedResult() *ParsedResult {
	return &ParsedResult{
		Bids:               make(map[Key][]RecordBid),
		NoBids:             make(map[Key][]RecordBid),
		HeaderRequested:    make(map[Key][]RecordBid),
		Payloads:           make(map[Key][]RecordPayload),
		BuilderBlockStored: make(map[Key][]RecordPayload),
		PayloadRequested:   make(map[Key][]RecordPayload),
		NoPayloadFound:     make(map[Key][]RecordPayload),
	}
}

// Query : `Service:relay AND ("payload sent" OR "bid sent" OR "builder block stored" OR "payload requested" OR "no builder bid" OR "no payload found" OR "header requested")
func parseCSV(readr io.ReadCloser) (*ParsedResult, error) {
	r := csv.NewReader(readr)
	pR := NewParsedResult()

	var i uint
	var rp RecordPosition
	for {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		i++
		if i == 1 {
			rp, err = parseHeader(record)
			if err != nil {
				return nil, err
			}
			continue
		}

		switch record[rp.Message] {

		case "bid sent":
			pb, err := parseBid(record, rp)
			if err != nil {
				return nil, err
			}
			v, ok := pR.Bids[Key{pb.Slot, pb.NetworkType}]
			if !ok {
				v = []RecordBid{}
			}
			v = append(v, pb)
			pR.Bids[Key{pb.Slot, pb.NetworkType}] = v

		case "no builder bid":
			pb, err := parseBid(record, rp)
			if err != nil {
				return nil, err
			}
			v, ok := pR.NoBids[Key{pb.Slot, pb.NetworkType}]
			if !ok {
				v = []RecordBid{}
			}
			v = append(v, pb)
			pR.NoBids[Key{pb.Slot, pb.NetworkType}] = v
		case "header requested":
			pb, err := parseBid(record, rp)
			if err != nil {
				return nil, err
			}
			v, ok := pR.HeaderRequested[Key{pb.Slot, pb.NetworkType}]
			if !ok {
				v = []RecordBid{}
			}
			v = append(v, pb)
			pR.HeaderRequested[Key{pb.Slot, pb.NetworkType}] = v
		case "builder block stored":
			pb, err := parsePayload(record, rp)
			if err != nil {
				return nil, err
			}
			v, ok := pR.BuilderBlockStored[Key{pb.Slot, pb.NetworkType}]
			if !ok {
				v = []RecordPayload{}
			}
			v = append(v, pb)
			pR.BuilderBlockStored[Key{pb.Slot, pb.NetworkType}] = v
		case "payload sent":
			pp, err := parsePayload(record, rp)
			if err != nil {
				return nil, err
			}

			v, ok := pR.Payloads[Key{pp.Slot, pp.NetworkType}]
			if !ok {
				v = []RecordPayload{}
			}
			v = append(v, pp)
			pR.Payloads[Key{pp.Slot, pp.NetworkType}] = v

		case "no payload found":
			pp, err := parsePayload(record, rp)
			if err != nil {
				return nil, err
			}

			v, ok := pR.NoPayloadFound[Key{pp.Slot, pp.NetworkType}]
			if !ok {
				v = []RecordPayload{}
			}
			v = append(v, pp)
			pR.NoPayloadFound[Key{pp.Slot, pp.NetworkType}] = v

		case "payload requested":
			pp, err := parsePayload(record, rp)
			if err != nil {
				return nil, err
			}

			v, ok := pR.PayloadRequested[Key{pp.Slot, pp.NetworkType}]
			if !ok {
				v = []RecordPayload{}
			}
			v = append(v, pp)
			pR.PayloadRequested[Key{pp.Slot, pp.NetworkType}] = v
		default:
			// skip others
		}
	}

	sortPayloads(pR.BuilderBlockStored)
	sortPayloads(pR.Payloads)
	sortPayloads(pR.NoPayloadFound)
	sortPayloads(pR.PayloadRequested)

	sortBids(pR.Bids)
	sortBids(pR.NoBids)

	return pR, nil
}

func sortPayloads(p map[Key][]RecordPayload) {
	for _, v := range p {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.UnixMicro() > v[j].Time.UnixMicro()
		})
	}
}
func sortBids(p map[Key][]RecordBid) {
	for _, v := range p {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.UnixMicro() > v[j].Time.UnixMicro()
		})
	}
}

func parsePayload(record []string, rp RecordPosition) (rPay RecordPayload, err error) {
	var networkType int
	h := record[rp.Host]
	if strings.Contains(h, "mainnet") {
		networkType = NetworkMainnet
	} else if strings.Contains(h, "goerli") {
		networkType = NetworkGoerli
	} else if strings.Contains(h, "ropsten") {
		networkType = NetworkRopsten
	} else if strings.Contains(h, "devnet") {
		networkType = NetworkDevnet
	} else {
		return rPay, fmt.Errorf("error parsing host - missing env: %+v", record[rp.Host])
	}

	time, err := time.Parse(time.RFC3339, record[rp.Date])
	if err != nil {
		return rPay, fmt.Errorf("error parsing date: %+v", record[rp.Date])
	}

	slot, err := strconv.ParseUint(record[rp.Slot], 10, 64)
	if err != nil {
		return rPay, fmt.Errorf("error parsing slot: %+v", record[rp.Slot])
	}

	var a *big.Int
	if record[rp.Bid] != "" {
		var ok bool
		a, ok = big.NewInt(0).SetString(strings.ReplaceAll(record[rp.Bid], `"`, ""), 10)
		if !ok {
			return rPay, fmt.Errorf("error parsing bid: %+v", record[rp.Bid])
		}
	}

	var b string
	if record[rp.Builder] != "" {
		b = strings.ReplaceAll(record[rp.Builder], `"`, "")
	}

	var blockhash string
	if record[rp.Blockhash] != "" {
		blockhash = strings.ReplaceAll(record[rp.Blockhash], `"`, "")
	}

	return RecordPayload{
		NetworkType: networkType,
		Time:        time,
		Slot:        slot,
		Builder:     b,
		Blockhash:   blockhash,
		Bid:         a,
	}, nil
}

func parseBid(record []string, rp RecordPosition) (rBid RecordBid, err error) {
	var networkType int
	h := record[rp.Host]
	if strings.Contains(h, "mainnet") {
		networkType = NetworkMainnet
	} else if strings.Contains(h, "goerli") {
		networkType = NetworkGoerli
	} else if strings.Contains(h, "ropsten") {
		networkType = NetworkRopsten
	} else if strings.Contains(h, "devnet") {
		networkType = NetworkDevnet
	} else {
		return rBid, fmt.Errorf("error parsing host - missing env: %+v", record[rp.Host])
	}

	time, err := time.Parse(time.RFC3339, record[rp.Date])
	if err != nil {
		return rBid, fmt.Errorf("error parsing date: %+v", record[rp.Date])
	}

	slot, err := strconv.ParseUint(record[rp.Slot], 10, 64)
	if err != nil {
		return rBid, fmt.Errorf("error parsing slot: %+v", record[rp.Slot])
	}

	var blockhash string
	if record[rp.Blockhash] != "" {
		blockhash = strings.ReplaceAll(record[rp.Blockhash], `"`, "")
	}

	var a *big.Int
	if record[rp.BidValue] != "" {
		var ok bool
		a, ok = big.NewInt(0).SetString(strings.ReplaceAll(record[rp.BidValue], `"`, ""), 10)
		if !ok {
			return rBid, fmt.Errorf("error parsing bid: %+v", record[rp.BidValue])
		}
	} else {
		a = big.NewInt(0)
	}

	return RecordBid{
		NetworkType: networkType,
		Time:        time,
		Slot:        slot,
		Blockhash:   blockhash,
		Bid:         a,
	}, nil
}
