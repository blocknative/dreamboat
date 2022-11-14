package structs

import (
	"fmt"

	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

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

type Epoch uint64

func (e Epoch) Loggable() map[string]any {
	return map[string]any{
		"epoch": e,
	}
}

type PubKey struct{ types.PublicKey }

func (pk PubKey) Loggable() map[string]any {
	return map[string]any{
		"pubkey": pk,
	}
}

func (pk PubKey) Bytes() []byte {
	return pk.PublicKey[:]
}

func (pk PubKey) ValidatorKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("valdator-%s", pk))
}

func (pk PubKey) RegistrationKey() ds.Key {
	return ds.NewKey(fmt.Sprintf("registration-%s", pk))
}

type TraceQuery struct {
	Slot          Slot
	BlockHash     types.Hash
	BlockNum      uint64
	Pubkey        types.PublicKey
	Cursor, Limit uint64
}

func (q TraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q TraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q TraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q TraceQuery) HasPubkey() bool {
	return q.Pubkey != types.PublicKey{}
}

func (q TraceQuery) HasCursor() bool {
	return q.Cursor != 0
}

func (q TraceQuery) HasLimit() bool {
	return q.Limit != 0
}

type BidTraceExtended struct {
	types.BidTrace
	BlockNumber uint64 `json:"block_number,string"`
	NumTx       uint64 `json:"num_tx,string"`
}

type BidTraceWithTimestamp struct {
	BidTraceExtended
	Timestamp uint64 `json:"timestamp,string"`
}

type BuilderGetValidatorsResponseEntrySlice []types.BuilderGetValidatorsResponseEntry

func (b BuilderGetValidatorsResponseEntrySlice) Loggable() map[string]any {
	return map[string]any{
		"numDuties": len(b),
	}
}

type Query struct {
	Slot      Slot
	BlockHash types.Hash
	BlockNum  uint64
	PubKey    types.PublicKey
}

type PayloadKey struct {
	BlockHash types.Hash
	Proposer  types.PublicKey
	Slot      Slot
}

type DeliveredTrace struct {
	Trace       BidTraceWithTimestamp
	BlockNumber uint64
}

type HeaderAndTrace struct {
	Header *types.ExecutionPayloadHeader
	Trace  *BidTraceWithTimestamp
}

type BlockBidAndTrace struct {
	Trace   *types.SignedBidTrace
	Bid     *types.GetHeaderResponse
	Payload *types.GetPayloadResponse
}

type BeaconState struct {
	DutiesState
	ValidatorsState
	GenesisInfo
}

func (s *BeaconState) KnownValidatorByIndex(index uint64) (types.PubkeyHex, error) {
	pk, ok := s.ValidatorsState.KnownValidatorsByIndex[index]
	if !ok {
		return "", ErrUnknownValue
	}
	return pk, nil
}

func (s *BeaconState) IsKnownValidator(pk types.PubkeyHex) (bool, error) {
	_, ok := s.ValidatorsState.KnownValidators[pk]
	return ok, nil
}

func (s *BeaconState) KnownValidators() map[types.PubkeyHex]struct{} {
	return s.ValidatorsState.KnownValidators
}

func (s *BeaconState) HeadSlot() Slot {
	return s.CurrentSlot
}

func (s *BeaconState) ValidatorsMap() BuilderGetValidatorsResponseEntrySlice {
	return s.ProposerDutiesResponse
}

type DutiesState struct {
	CurrentSlot            Slot
	ProposerDutiesResponse BuilderGetValidatorsResponseEntrySlice
}

type ValidatorsState struct {
	KnownValidatorsByIndex map[uint64]types.PubkeyHex
	KnownValidators        map[types.PubkeyHex]struct{}
}

type GenesisInfo struct {
	GenesisTime           uint64 `json:"genesis_time,string"`
	GenesisValidatorsRoot string `json:"genesis_validators_root"`
	GenesisForkVersion    string `json:"genesis_fork_version"`
}
