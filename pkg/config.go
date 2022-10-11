package relay

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
)

const (
	GenesisForkVersionMainnet = "0x00000000"
	GenesisForkVersionKiln    = "0x70000069" // https://github.com/eth-clients/merge-testnets/blob/main/kiln/config.yaml#L10
	GenesisForkVersionRopsten = "0x80000069"
	GenesisForkVersionSepolia = "0x90000069"
	GenesisForkVersionGoerli  = "0x00001020" // https://github.com/eth-clients/merge-testnets/blob/main/goerli-shadow-fork-5/config.yaml#L11

	GenesisValidatorsRootMainnet = "0x4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95"
	GenesisValidatorsRootKiln    = "0x99b09fcd43e5905236c370f184056bec6e6638cfc31a323b304fc4aa789cb4ad"
	GenesisValidatorsRootRopsten = "0x44f1e56283ca88b35c789f7f449e52339bc1fefe3a45913a43a6d16edcd33cf1"
	GenesisValidatorsRootSepolia = "0xd8ea171f3c94aea21ebc42a1ed61052acf3f9209c00e4efbaaddac09ed9b8078"
	GenesisValidatorsRootGoerli  = "0x043db0d9a83813551ee2f33450d23797757d430911a9320530ad8a0eabc43efb"

	BellatrixForkVersionMainnet = "0x02000000"
	BellatrixForkVersionKiln    = "0x70000071"
	BellatrixForkVersionRopsten = "0x80000071"
	BellatrixForkVersionSepolia = "0x90000071"
	BellatrixForkVersionGoerli  = "0x02001020"
)

type PubKey struct{ types.PublicKey }

func (pk PubKey) Loggable() map[string]any {
	return map[string]any{
		"pubkey": pk,
	}
}

func (pk PubKey) Bytes() []byte {
	return pk.PublicKey[:]
}

// Config provides all available options for the default BeaconClient and Relay
type Config struct {
	Log                 log.Logger
	BuilderURLs         []string
	Network             string
	RelayRequestTimeout time.Duration
	BuilderCheck        bool
	BeaconEndpoints     []string
	SimulationEndpoint  string
	PubKey              types.PublicKey
	SecretKey           *bls.SecretKey
	Datadir             string
	TTL                 time.Duration
	CheckKnownValidator bool

	// private fields; populated during validation
	builders              map[PubKey]*builder
	genesisForkVersion    string
	genesisValidatorsRoot string
	bellatrixForkVersion  string
}

func (c *Config) validate() error {
	c.builders = make(map[PubKey]*builder)

	if err := c.validateNetwork(); err != nil {
		return err
	}

	return c.validateBuilders()
}

func (c *Config) validateNetwork() error {
	switch c.Network {
	case "main", "mainnet":
		c.genesisForkVersion = GenesisForkVersionMainnet
		c.genesisValidatorsRoot = GenesisValidatorsRootMainnet
		c.bellatrixForkVersion = BellatrixForkVersionMainnet
	case "kiln":
		c.genesisForkVersion = GenesisForkVersionKiln
		c.genesisValidatorsRoot = GenesisValidatorsRootKiln
		c.bellatrixForkVersion = BellatrixForkVersionKiln
	case "ropsten":
		c.genesisForkVersion = GenesisForkVersionRopsten
		c.genesisValidatorsRoot = GenesisValidatorsRootRopsten
		c.bellatrixForkVersion = BellatrixForkVersionRopsten
	case "sepolia":
		c.genesisForkVersion = GenesisForkVersionSepolia
		c.genesisValidatorsRoot = GenesisValidatorsRootSepolia
		c.bellatrixForkVersion = BellatrixForkVersionSepolia
	case "goerli":
		c.genesisForkVersion = GenesisForkVersionGoerli
		c.genesisValidatorsRoot = GenesisValidatorsRootGoerli
		c.bellatrixForkVersion = BellatrixForkVersionGoerli
	default:
		return errors.New("unknown network")
	}
	return nil
}

func (c *Config) validateBuilders() (err error) {
	var entry *builder
	for _, b := range c.BuilderURLs {
		if entry, err = newBuilderEntry(b); err != nil {
			break
		}

		c.builders[entry.PubKey] = entry
	}

	return
}

// builder represents a builder that the relay service connects to.
type builder struct {
	PubKey PubKey
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

	var pk PubKey
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
