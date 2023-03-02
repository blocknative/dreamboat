package structs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
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
	Slot          Slot
	BlockHash     types.Hash
	BlockNum      uint64
	Pubkey        types.PublicKey
	Cursor, Limit uint64
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
	return q.Pubkey != types.PublicKey{}
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
	Payload BlockBidAndTrace
}

func SignedBlindedBeaconBlockToBeaconBlock(signedBlindedBeaconBlock *types.SignedBlindedBeaconBlock, executionPayload *types.ExecutionPayload) *types.SignedBeaconBlock {
	block := &types.SignedBeaconBlock{
		Signature: signedBlindedBeaconBlock.Signature,
		Message: &types.BeaconBlock{
			Slot:          signedBlindedBeaconBlock.Message.Slot,
			ProposerIndex: signedBlindedBeaconBlock.Message.ProposerIndex,
			ParentRoot:    signedBlindedBeaconBlock.Message.ParentRoot,
			StateRoot:     signedBlindedBeaconBlock.Message.StateRoot,
			Body: &types.BeaconBlockBody{
				RandaoReveal:      signedBlindedBeaconBlock.Message.Body.RandaoReveal,
				Eth1Data:          signedBlindedBeaconBlock.Message.Body.Eth1Data,
				Graffiti:          signedBlindedBeaconBlock.Message.Body.Graffiti,
				ProposerSlashings: signedBlindedBeaconBlock.Message.Body.ProposerSlashings,
				AttesterSlashings: signedBlindedBeaconBlock.Message.Body.AttesterSlashings,
				Attestations:      signedBlindedBeaconBlock.Message.Body.Attestations,
				Deposits:          signedBlindedBeaconBlock.Message.Body.Deposits,
				VoluntaryExits:    signedBlindedBeaconBlock.Message.Body.VoluntaryExits,
				SyncAggregate:     signedBlindedBeaconBlock.Message.Body.SyncAggregate,
				ExecutionPayload:  executionPayload,
			},
		},
	}

	if block.Message.Body.ProposerSlashings == nil {
		block.Message.Body.ProposerSlashings = []*types.ProposerSlashing{}
	}
	if block.Message.Body.AttesterSlashings == nil {
		block.Message.Body.AttesterSlashings = []*types.AttesterSlashing{}
	}
	if block.Message.Body.Attestations == nil {
		block.Message.Body.Attestations = []*types.Attestation{}
	}
	if block.Message.Body.Deposits == nil {
		block.Message.Body.Deposits = []*types.Deposit{}
	}

	if block.Message.Body.VoluntaryExits == nil {
		block.Message.Body.VoluntaryExits = []*types.SignedVoluntaryExit{}
	}

	if block.Message.Body.Eth1Data == nil {
		block.Message.Body.Eth1Data = &types.Eth1Data{}
	}

	if block.Message.Body.SyncAggregate == nil {
		block.Message.Body.SyncAggregate = &types.SyncAggregate{}
	}

	if block.Message.Body.ExecutionPayload == nil {
		block.Message.Body.ExecutionPayload = &types.ExecutionPayload{}
	}

	if block.Message.Body.ExecutionPayload.ExtraData == nil {
		block.Message.Body.ExecutionPayload.ExtraData = types.ExtraData{}
	}

	if block.Message.Body.ExecutionPayload.Transactions == nil {
		block.Message.Body.ExecutionPayload.Transactions = []hexutil.Bytes{}
	}

	return block
}

type ValidatorCacheEntry struct {
	Time  time.Time
	Entry types.SignedValidatorRegistration
}

type ForkState struct {
	BellatrixEpoch Epoch
	CapellaEpoch   Epoch
}

func (fs ForkState) IsCapella(slot Slot) bool {
	return slot.Epoch() >= fs.CapellaEpoch
}

func (fs ForkState) IsBellatrix(slot Slot) bool {
	return slot.Epoch() >= fs.BellatrixEpoch && slot.Epoch() < fs.CapellaEpoch
}
