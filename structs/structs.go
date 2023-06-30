package structs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/flashbots/go-boost-utils/types"
	ds "github.com/ipfs/go-datastore"
)

var ErrUnknownValue = errors.New("value is unknown")

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

// PayloadTraceQuery structure used to query payloads only
type PayloadTraceQuery struct {
	Slot                          Slot
	BlockHash                     types.Hash
	BlockNum                      uint64
	ProposerPubkey, BuilderPubkey types.PublicKey
	Cursor, Limit                 uint64
	OrderByValue                  int
}

func (q PayloadTraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q PayloadTraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q PayloadTraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q PayloadTraceQuery) HasPubkey() bool {
	return q.ProposerPubkey != types.PublicKey{}
}

func (q PayloadTraceQuery) HasCursor() bool {
	return q.Cursor != 0
}

func (q PayloadTraceQuery) HasLimit() bool {
	return q.Limit != 0
}

type PayloadQuery struct {
	Slot      Slot
	BlockHash types.Hash
	BlockNum  uint64
	PubKey    types.PublicKey
	Limit     uint64
}

// HeaderTraceQuery structure used to query header structure
type HeaderTraceQuery struct {
	Slot      Slot
	BlockHash types.Hash
	BlockNum  uint64
	Limit     uint64
}

func (q HeaderTraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q HeaderTraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q HeaderTraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q HeaderTraceQuery) HasLimit() bool {
	return q.Limit != 0
}

// SubmissionTraceQuery structure used to query header structure
type SubmissionTraceQuery struct {
	Slot          Slot
	BlockHash     types.Hash
	BlockNum      uint64
	Limit         uint64
	BuilderPubkey types.PublicKey
}

func (q SubmissionTraceQuery) HasSlot() bool {
	return q.Slot != Slot(0)
}

func (q SubmissionTraceQuery) HasBlockHash() bool {
	return q.BlockHash != types.Hash{}
}

func (q SubmissionTraceQuery) HasBlockNum() bool {
	return q.BlockNum != 0
}

func (q SubmissionTraceQuery) HasLimit() bool {
	return q.Limit != 0
}

type BidTraceExtended struct {
	types.BidTrace
	BlockNumber uint64 `json:"block_number,string"`
	NumTx       uint64 `json:"num_tx,string"`
}

type BidTraceWithTimestamp struct {
	BidTraceExtended
	Timestamp   uint64 `json:"timestamp,string"`
	TimestampMs uint64 `json:"timestamp_ms,string"`
}

type BuilderGetValidatorsResponseEntrySlice []types.BuilderGetValidatorsResponseEntry

func (b BuilderGetValidatorsResponseEntrySlice) Loggable() map[string]any {
	return map[string]any{
		"numDuties": len(b),
	}
}

type PayloadKey struct {
	BlockHash types.Hash
	Proposer  types.PublicKey
	Slot      Slot
}

func (k PayloadKey) Loggable() map[string]any {
	return map[string]any{
		"slot":       k.Slot,
		"block_hash": k.BlockHash,
		"proposer":   k.Proposer,
	}
}

type DeliveredTrace struct {
	Trace       BidTraceWithTimestamp
	BlockNumber uint64
}

type HeaderAndTrace struct {
	Header ExecutionPayloadHeader
	Trace  BidTraceWithTimestamp
}

type ExecutionPayloadHeader interface {
	GetParentHash() types.Hash
	GetBlockHash() types.Hash
	GetBlockNumber() uint64
}

type BlockAndTraceExtended interface {
	BidValue() types.U256Str
	Slot() uint64
	Proposer() types.PublicKey
	BuilderPubkey() (pub types.PublicKey)

	ExecutionPayload() ExecutionPayload
	ExecutionHeaderHash() (types.Hash, error)
	ToDeliveredTrace(slot uint64) (DeliveredTrace, error)
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

// / extra
type HeaderData struct {
	HeaderAndTrace
	Slot      Slot
	Marshaled []byte `json:"-"`
}

func (hd *HeaderData) UnmarshalJSON(b []byte) error {
	var hnt HeaderAndTrace
	if err := json.Unmarshal(b, &hnt); err != nil {
		return err
	}
	hd.HeaderAndTrace = hnt
	hd.Marshaled = b
	return nil
}

// / That's to be improved in future
type CompleteBlockstruct struct {
	Header  HeaderAndTrace
	Payload BlockAndTraceExtended
}

type ValidatorCacheEntry struct {
	Time  time.Time
	Entry types.SignedValidatorRegistration
}

type WithdrawalsState struct {
	Slot       Slot
	ParentHash types.Hash
	Root       types.Root
}

type RandaoState struct {
	Slot       uint64
	ParentHash types.Hash
	Randao     string
}

type ForkState struct {
	AltairEpoch    uint64
	BellatrixEpoch uint64
	CapellaEpoch   uint64
}

type ForkVersion uint8

const (
	ForkUnknown ForkVersion = iota
	ForkAltair
	ForkBellatrix
	ForkCapella
)

func (v ForkVersion) String() string {
	switch v {
	case ForkAltair:
		return "altair"
	case ForkBellatrix:
		return "bellatrix"
	case ForkCapella:
		return "capella"
	default:
		return "unknown"
	}
}

func (fs ForkState) IsCapella(slot uint64, epoch uint64) bool {
	return fs.CapellaEpoch > 0 && epoch >= fs.CapellaEpoch
}

func (fs ForkState) IsBellatrix(slot uint64, epoch uint64) bool {
	if fs.CapellaEpoch == 0 {
		return fs.BellatrixEpoch > 0 && epoch >= fs.BellatrixEpoch
	}
	return fs.BellatrixEpoch > 0 && epoch >= fs.BellatrixEpoch && epoch < fs.CapellaEpoch
}

func (fs ForkState) IsAltair(slot uint64, epoch uint64) bool {
	return epoch >= fs.AltairEpoch && epoch < fs.BellatrixEpoch
}

func (fs ForkState) Version(slot uint64, epoch uint64) ForkVersion {
	if fs.IsCapella(slot, epoch) {
		return ForkCapella
	} else if fs.IsBellatrix(slot, epoch) {
		return ForkBellatrix
	} else if fs.IsAltair(slot, epoch) {
		return ForkAltair
	}
	return ForkUnknown
}
