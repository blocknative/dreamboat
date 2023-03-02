package forks

import (
	"github.com/flashbots/go-boost-utils/types"
)

// BlindedBeaconBlockBody https://github.com/ethereum/beacon-APIs/blob/master/types/bellatrix/block.yaml#L65
type BlindedBeaconBlockBody struct {
	RandaoReveal      types.Signature              `json:"randao_reveal" ssz-size:"96"`
	Eth1Data          *types.Eth1Data              `json:"eth1_data"`
	Graffiti          types.Hash                   `json:"graffiti" ssz-size:"32"`
	ProposerSlashings []*types.ProposerSlashing    `json:"proposer_slashings" ssz-max:"16"`
	AttesterSlashings []*types.AttesterSlashing    `json:"attester_slashings" ssz-max:"2"`
	Attestations      []*types.Attestation         `json:"attestations" ssz-max:"128"`
	Deposits          []*types.Deposit             `json:"deposits" ssz-max:"16"`
	VoluntaryExits    []*types.SignedVoluntaryExit `json:"voluntary_exits" ssz-max:"16"`
	SyncAggregate     *types.SyncAggregate         `json:"sync_aggregate"`
	// ExecutionPayloadHeader to be implemented in forks
}
