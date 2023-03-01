package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/blocknative/dreamboat/pkg/structs"
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

	GenesisValidatorsRootMainnet = "0x4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95"
	GenesisValidatorsRootRopsten = "0x44f1e56283ca88b35c789f7f449e52339bc1fefe3a45913a43a6d16edcd33cf1"
	GenesisValidatorsRootSepolia = "0xd8ea171f3c94aea21ebc42a1ed61052acf3f9209c00e4efbaaddac09ed9b8078"
	GenesisValidatorsRootGoerli  = "0x043db0d9a83813551ee2f33450d23797757d430911a9320530ad8a0eabc43efb"
)

// Config provides all available options for the default BeaconClient and Relay
type Config struct {
	builders map[structs.PubKey]*builder

	GenesisForkVersion    string
	GenesisValidatorsRoot string

	BellatrixForkVersion string
	CapellaForkVersion   string
}

func NewConfig() *Config {
	return &Config{
		builders: make(map[structs.PubKey]*builder),
	}
}

func (c *Config) LoadBuilders(builderURLs []string) (err error) {
	var entry *builder
	for _, b := range builderURLs {
		if entry, err = newBuilderEntry(b); err != nil {
			break
		}

		c.builders[entry.PubKey] = entry
	}
	return err
}

func (c *Config) LoadNetwork(network string) {
	switch network {
	case "main", "mainnet":
		c.GenesisForkVersion = GenesisForkVersionMainnet
		c.GenesisValidatorsRoot = GenesisValidatorsRootMainnet
		c.BellatrixForkVersion = BellatrixForkVersionMainnet
		c.CapellaForkVersion = CapellaForkVersionMainnet
	case "ropsten":
		c.GenesisForkVersion = GenesisForkVersionRopsten
		c.GenesisValidatorsRoot = GenesisValidatorsRootRopsten
		c.BellatrixForkVersion = BellatrixForkVersionRopsten
		c.CapellaForkVersion = CapellaForkVersionRopsten
	case "sepolia":
		c.GenesisForkVersion = GenesisForkVersionSepolia
		c.GenesisValidatorsRoot = GenesisValidatorsRootSepolia
		c.BellatrixForkVersion = BellatrixForkVersionSepolia
		c.CapellaForkVersion = CapellaForkVersionSepolia
	case "goerli":
		c.GenesisForkVersion = GenesisForkVersionGoerli
		c.GenesisValidatorsRoot = GenesisValidatorsRootGoerli
		c.BellatrixForkVersion = BellatrixForkVersionGoerli
		c.CapellaForkVersion = CapellaForkVersionGoerli
	}
}

type Network struct {
	GenesisForkVersion    string `json:"GenesisForkVersion"`
	GenesisValidatorsRoot string `json:"GenesisValidatorsRoot"`
	BellatrixForkVersion  string `json:"BellatrixForkVersion"`
	CapellaForkVersion    string `json:"CapellaForkVersion"`
}

func (c *Config) ReadNetworkConfig(datadir, network string) (err error) {
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

	return nil
}

// builder represents a builder that the relay service connects to.
type builder struct {
	PubKey structs.PubKey
	URL    *url.URL
}

func (b builder) Loggable() map[string]any {
	return map[string]any{
		"pubkey": b.PubKey,
		"url":    b.URL,
	}
}

// NewRelayEntry creates a new instance based on an input string
// relayURL can be IP@PORT, PUBKEY@IP:PORT, https://IP, etc.
func newBuilderEntry(relayURL string) (*builder, error) {
	u, err := url.ParseRequestURI(ensureScheme(relayURL))
	if err != nil {
		return nil, err
	}

	// Extract the relay's public key from the parsed URL.
	if !hasPubKey(u) {
		return nil, errors.New("missing relay public key")
	}

	var pk structs.PubKey
	if err = pk.UnmarshalText([]byte(u.User.Username())); err != nil {
		return nil, err
	}

	return &builder{
		PubKey: pk,
		URL:    u,
	}, nil
}

// GetURI returns the full request URI with scheme, host, path and args.
func (b builder) GetURI(path string) string {
	u := *b.URL
	u.User = nil
	u.Path = path
	return u.String()
}

func ensureScheme(url string) string {
	if strings.HasPrefix(url, "http") {
		return url
	}

	return fmt.Sprintf("http://%s", url)
}

func hasPubKey(u *url.URL) bool {
	return u.User.Username() != ""
}
