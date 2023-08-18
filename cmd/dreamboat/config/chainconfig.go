package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	GenesisForkVersionMainnet = "0x00000000"
	GenesisForkVersionRopsten = "0x80000069"
	GenesisForkVersionSepolia = "0x90000069"
	GenesisForkVersionGoerli  = "0x00001020" // https://github.com/eth-clients/merge-testnets/blob/main/goerli-shadow-fork-5/config.yaml#L11

	BellatrixForkVersionMainnet = "0x02000000"
	BellatrixForkVersionRopsten = "0x80000071"
	BellatrixForkVersionSepolia = "0x90000071"
	BellatrixForkVersionGoerli  = "0x02001020"

	CapellaForkVersionRopsten = "0x03001020"
	CapellaForkVersionSepolia = "0x90000072"
	CapellaForkVersionGoerli  = "0x03001020"
	CapellaForkVersionMainnet = "0x03000000"

	DenebForkVersionRopsten = "0x00000000"
	DenebForkVersionSepolia = "0x00000000"
	DenebForkVersionGoerli  = "0x00000000"
	DenebForkVersionMainnet = "0x00000000"

	GenesisValidatorsRootMainnet = "0x4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95"
	GenesisValidatorsRootRopsten = "0x44f1e56283ca88b35c789f7f449e52339bc1fefe3a45913a43a6d16edcd33cf1"
	GenesisValidatorsRootSepolia = "0xd8ea171f3c94aea21ebc42a1ed61052acf3f9209c00e4efbaaddac09ed9b8078"
	GenesisValidatorsRootGoerli  = "0x043db0d9a83813551ee2f33450d23797757d430911a9320530ad8a0eabc43efb"
)

type Network struct {
	GenesisForkVersion    string `json:"GenesisForkVersion"`
	GenesisValidatorsRoot string `json:"GenesisValidatorsRoot"`
	BellatrixForkVersion  string `json:"BellatrixForkVersion"`
	CapellaForkVersion    string `json:"CapellaForkVersion"`
	DenebForkVersion      string `json:"DenebForkVersion"`
}

// ChainConfig provides all available options for the default BeaconClient and Relay
type ChainConfig struct {
	GenesisForkVersion    string
	BellatrixForkVersion  string
	CapellaForkVersion    string
	GenesisValidatorsRoot string
	DenebForkVersion      string
}

func NewChainConfig() *ChainConfig {
	return &ChainConfig{}
}

func (c *ChainConfig) LoadNetwork(network string) {
	switch network {
	case "main", "mainnet":
		c.GenesisForkVersion = GenesisForkVersionMainnet
		c.GenesisValidatorsRoot = GenesisValidatorsRootMainnet
		c.BellatrixForkVersion = BellatrixForkVersionMainnet
		c.CapellaForkVersion = CapellaForkVersionMainnet
		c.DenebForkVersion = DenebForkVersionMainnet
	case "ropsten":
		c.GenesisForkVersion = GenesisForkVersionRopsten
		c.GenesisValidatorsRoot = GenesisValidatorsRootRopsten
		c.BellatrixForkVersion = BellatrixForkVersionRopsten
		c.CapellaForkVersion = CapellaForkVersionRopsten
		c.DenebForkVersion = DenebForkVersionRopsten
	case "sepolia":
		c.GenesisForkVersion = GenesisForkVersionSepolia
		c.GenesisValidatorsRoot = GenesisValidatorsRootSepolia
		c.BellatrixForkVersion = BellatrixForkVersionSepolia
		c.CapellaForkVersion = CapellaForkVersionSepolia
		c.DenebForkVersion = DenebForkVersionSepolia
	case "goerli":
		c.GenesisForkVersion = GenesisForkVersionGoerli
		c.GenesisValidatorsRoot = GenesisValidatorsRootGoerli
		c.BellatrixForkVersion = BellatrixForkVersionGoerli
		c.CapellaForkVersion = CapellaForkVersionGoerli
		c.DenebForkVersion = DenebForkVersionGoerli
	}
}

func (c *ChainConfig) ReadNetworkConfig(datadir, network string) (err error) {
	jsonFile, err := os.Open(datadir + "/networks.json")
	if err != nil {
		return fmt.Errorf("unknown network: %s: %w", network, err)
	}

	var networks map[string]Network
	if err := json.NewDecoder(jsonFile).Decode(&networks); err != nil {
		return err
	}

	config, ok := networks[network]
	if !ok {
		return fmt.Errorf("not found in config file: %s", datadir+"/networks.json")
	}

	c.GenesisForkVersion = config.GenesisForkVersion
	c.GenesisValidatorsRoot = config.GenesisValidatorsRoot
	c.BellatrixForkVersion = config.BellatrixForkVersion
	c.CapellaForkVersion = config.CapellaForkVersion
	c.DenebForkVersion = config.DenebForkVersion

	return nil
}
