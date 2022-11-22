package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"

	relay "github.com/blocknative/dreamboat/pkg"
	"github.com/blocknative/dreamboat/pkg/api"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/pkg/errors"
)

const (
	url = "http://localhost:18550" + api.PathGetPayload
)

var (
	slot      uint64 = 0
	blockHash        = "0"
)

func main() {
	fmt.Print("getting payload... ")
	if err := submitRequest(); err != nil {
		panic(err)
	}
}

func submitRequest() error {
	builderDomain, err := relay.ComputeDomain(
		types.DomainTypeAppBuilder,
		relay.GenesisForkVersionRopsten,
		types.Root{}.String())
	if err != nil {
		return err
	}

	request, err := validGetPayloadRequest(builderDomain)
	if err != nil {
		return err
	}

	b, err := json.Marshal(request)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.WithMessage(fmt.Errorf("invalid return code, expected 200 - received %d", resp.StatusCode), string(body))
	}

	fmt.Println(resp.Status)
	return nil
}

func validGetPayloadRequest(domain types.Domain) (*types.SignedBlindedBeaconBlock, error) {
	submitRequest, err := validSubmitBlockRequest(domain)
	if err != nil {
		return nil, err
	}

	header, err := types.PayloadToPayloadHeader(submitRequest.ExecutionPayload)
	if err != nil {
		return nil, err
	}

	header.BlockHash = submitRequest.Message.BlockHash

	msg := &types.BlindedBeaconBlock{
		Slot:          submitRequest.Message.Slot,
		ProposerIndex: 2,
		ParentRoot:    types.Root{0x03},
		StateRoot:     types.Root{0x04},
		Body: &types.BlindedBeaconBlockBody{
			Eth1Data: &types.Eth1Data{
				DepositRoot:  types.Root{0x05},
				DepositCount: 5,
				BlockHash:    types.Hash{0x06},
			},
			ProposerSlashings:      []*types.ProposerSlashing{},
			AttesterSlashings:      []*types.AttesterSlashing{},
			Attestations:           []*types.Attestation{},
			Deposits:               []*types.Deposit{},
			VoluntaryExits:         []*types.SignedVoluntaryExit{},
			SyncAggregate:          &types.SyncAggregate{types.CommitteeBits{0x07}, types.Signature{0x08}},
			ExecutionPayloadHeader: header,
		},
	}

	return &types.SignedBlindedBeaconBlock{
		Message:   msg,
		Signature: random96Bytes(),
	}, nil
}

func validSubmitBlockRequest(domain types.Domain) (*types.BuilderSubmitBlockRequest, error) {
	sk, pk, err := bls.GenerateNewKeypair()
	if err != nil {
		return nil, err
	}

	var pubKey types.PublicKey
	if err := pubKey.FromSlice(pk.Compress()); err != nil {
		return nil, err
	}

	payload := randomPayload()

	var bh types.Hash
	bh.UnmarshalText([]byte(blockHash))

	msg := &types.BidTrace{
		Slot:                 slot,
		ParentHash:           types.Hash(random32Bytes()),
		BlockHash:            bh,
		BuilderPubkey:        pubKey,
		ProposerPubkey:       types.PublicKey(random48Bytes()),
		ProposerFeeRecipient: types.Address(random20Bytes()),
		Value:                types.IntToU256(rand.Uint64()),
	}

	signature, err := types.SignMessage(msg, domain, sk)
	if err != nil {
		return nil, err
	}

	return &types.BuilderSubmitBlockRequest{
		Signature:        signature,
		Message:          msg,
		ExecutionPayload: payload,
	}, nil
}

func randomPayload() *types.ExecutionPayload {

	return &types.ExecutionPayload{
		ParentHash:    types.Hash(random32Bytes()),
		FeeRecipient:  types.Address(random20Bytes()),
		StateRoot:     types.Hash(random32Bytes()),
		ReceiptsRoot:  types.Hash(random32Bytes()),
		LogsBloom:     types.Bloom(random256Bytes()),
		Random:        random32Bytes(),
		BlockNumber:   rand.Uint64(),
		GasLimit:      rand.Uint64(),
		GasUsed:       rand.Uint64(),
		Timestamp:     rand.Uint64(),
		ExtraData:     types.ExtraData{},
		BaseFeePerGas: types.IntToU256(rand.Uint64()),
		BlockHash:     types.Hash(random32Bytes()),
		Transactions:  randomTransactions(2),
	}
}

func randomTransactions(size int) []hexutil.Bytes {
	txs := make([]hexutil.Bytes, 0, size)
	for i := 0; i < size; i++ {
		tx := make([]byte, rand.Intn(32))
		rand.Read(tx)
		txs = append(txs, tx)
	}
	return txs
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}

func random48Bytes() (b [48]byte) {
	rand.Read(b[:])
	return b
}

func random96Bytes() (b [96]byte) {
	rand.Read(b[:])
	return b
}

func random20Bytes() (b [20]byte) {
	rand.Read(b[:])
	return b
}

func random256Bytes() (b [256]byte) {
	rand.Read(b[:])
	return b
}
