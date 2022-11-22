package relay

import (
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flashbots/go-boost-utils/types"
)

func TestComputeDomain(t *testing.T) {
	type args struct {
		domainType               types.DomainType
		forkVersionHex           string
		genesisValidatorsRootHex string
	}
	tests := []struct {
		name       string
		args       args
		wantDomain string
		wantErr    bool
	}{
		{
			name: "domainBuilder",
			args: args{
				domainType:               types.DomainTypeAppBuilder,
				forkVersionHex:           "0x00000000",
				genesisValidatorsRootHex: types.Root{}.String(),
			},
			wantDomain: "0x00000001f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a9",
		},
		{
			name: "domainBeaconProposer",
			args: args{
				domainType:               types.DomainTypeBeaconProposer,
				forkVersionHex:           "0x02000000",
				genesisValidatorsRootHex: "0x4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95",
			},
			wantDomain: "0x000000004a26c58b08add8089b75caa540848881a8d4f0af0be83417a85c0f45",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDomain, err := ComputeDomain(tt.args.domainType, tt.args.forkVersionHex, tt.args.genesisValidatorsRootHex)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComputeDomain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(hexutil.Encode(gotDomain[:]), tt.wantDomain) {
				t.Errorf("ComputeDomain() = %v, want %v", gotDomain, tt.wantDomain)
			}
		})
	}
}
