package relay

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
	"golang.org/x/exp/constraints"
)

type UserAgent string

// ComputeDomain computes the signing domain
func ComputeDomain(domainType types.DomainType, forkVersionHex string, genesisValidatorsRootHex string) (domain types.Domain, err error) {
	genesisValidatorsRoot := types.Root(common.HexToHash(genesisValidatorsRootHex))
	forkVersionBytes, err := hexutil.Decode(forkVersionHex)
	if err != nil || len(forkVersionBytes) > 4 {
		err = errors.New("invalid fork version passed")
		return domain, err
	}
	var forkVersion [4]byte
	copy(forkVersion[:], forkVersionBytes[:4])
	return types.ComputeDomain(domainType, forkVersion, genesisValidatorsRoot), nil
}

type BuilderGetValidatorsResponseEntrySlice []types.BuilderGetValidatorsResponseEntry

type GetValidatorRelayResponse []struct {
	Slot  uint64 `json:"slot,string"`
	Entry struct {
		Message struct {
			FeeRecipient string `json:"fee_recipient"`
			GasLimit     uint64 `json:"gas_limit,string"`
			Timestamp    uint64 `json:"timestamp,string"`
			PubKey       string `json:"pubkey"`
		} `json:"message"`
		Signature string `json:"signature"`
	} `json:"entry"`
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

type Slot uint64

func (s Slot) Loggable() map[string]any {
	return map[string]any{
		"slot":  s,
		"epoch": s.Epoch(),
	}
}

func (s Slot) Epoch() Epoch {
	return Epoch(s / SlotsPerEpoch)
}

func (s Slot) HeaderKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("header-%d", s))
}

func (s Slot) PayloadKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("payload-%d", s))
}

func (pk PubKey) ValidatorKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk))
}

func (pk PubKey) RegistrationKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("registration-%s", pk))
}

func SubmitBlockRequestToBlockBidAndTrace(signedBuilderBid *types.SignedBuilderBid, submitBlockRequest *types.BuilderSubmitBlockRequest) BlockBidAndTrace {
	getHeaderResponse := types.GetHeaderResponse{
		Version: "bellatrix",
		Data:    signedBuilderBid,
	}

	getPayloadResponse := types.GetPayloadResponse{
		Version: "bellatrix",
		Data:    submitBlockRequest.ExecutionPayload,
	}

	signedBidTrace := types.SignedBidTrace{
		Message:   submitBlockRequest.Message,
		Signature: submitBlockRequest.Signature,
	}

	return BlockBidAndTrace{
		Trace:   &signedBidTrace,
		Bid:     &getHeaderResponse,
		Payload: &getPayloadResponse,
	}
}

type Epoch uint64

func (e Epoch) Loggable() map[string]any {
	return map[string]any{
		"epoch": e,
	}
}

func (b BuilderGetValidatorsResponseEntrySlice) Loggable() map[string]any {
	return map[string]any{
		"numDuties": len(b),
	}
}

type ErrBadProposer struct {
	Want, Got PubKey
}

func (ErrBadProposer) Error() string { return "peer is not proposer" }

func Max[T constraints.Ordered](args ...T) T {
	if len(args) == 0 {
		return *new(T) // zero value of T
	}

	if isNan(args[0]) {
		return args[0]
	}

	max := args[0]
	for _, arg := range args[1:] {

		if isNan(arg) {
			return arg
		}

		if arg > max {
			max = arg
		}
	}
	return max
}

func isNan[T constraints.Ordered](arg T) bool {
	return arg != arg
}
