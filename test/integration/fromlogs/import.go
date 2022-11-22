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

func parseCSV(readr io.ReadCloser) (map[Key][]RecordPayload, map[Key][]RecordBid, error) {
	r := csv.NewReader(readr)
	rPayloads := map[Key][]RecordPayload{}
	rBids := map[Key][]RecordBid{}

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
				return rPayloads, rBids, err
			}
			continue
		}

		switch record[rp.Message] {
		case "bid sent":
			pb, err := parseBid(record, rp)
			if err != nil {
				return nil, nil, err
			}
			v, ok := rBids[Key{pb.Slot, pb.NetworkType}]
			if !ok {
				v = []RecordBid{}
			}
			v = append(v, pb)
			rBids[Key{pb.Slot, pb.NetworkType}] = v
		case "payload sent":
			pp, err := parsePayload(record, rp)
			if err != nil {
				return nil, nil, err
			}

			v, ok := rPayloads[Key{pp.Slot, pp.NetworkType}]
			if !ok {
				v = []RecordPayload{}
			}
			v = append(v, pp)
			rPayloads[Key{pp.Slot, pp.NetworkType}] = v
		default:
			// skip others
		}
	}

	for _, v := range rPayloads {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.After(v[j].Time)
		})
	}

	for _, v := range rBids {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.After(v[j].Time)
		})
	}
	return rPayloads, rBids, nil
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

	a, ok := big.NewInt(0).SetString(strings.ReplaceAll(record[rp.Bid], `"`, ""), 10)
	if !ok {
		return rPay, fmt.Errorf("error parsing bid: %+v", record[rp.Bid])
	}
	return RecordPayload{
		NetworkType: networkType,
		Time:        time,
		Slot:        slot,
		Blockhash:   strings.ReplaceAll(record[rp.Blockhash], `"`, ""),
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

	a, ok := big.NewInt(0).SetString(strings.ReplaceAll(record[rp.BidValue], `"`, ""), 10)
	if !ok {
		return rBid, fmt.Errorf("error parsing bid: %+v", record[rp.BidValue])
	}
	return RecordBid{
		NetworkType: networkType,
		Time:        time,
		Slot:        slot,
		Blockhash:   strings.ReplaceAll(record[rp.Blockhash], `"`, ""),
		Bid:         a,
	}, nil
}
