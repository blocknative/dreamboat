package relay

import (
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/bls"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
)

type RelayConfig struct {
	L log.Logger

	BuilderSigningDomain  types.Domain
	ProposerSigningDomain map[structs.ForkVersion]types.Domain
	PubKey                types.PublicKey
	SecretKey             *bls.SecretKey

	GetPayloadResponseDelay    time.Duration
	GetPayloadRequestTimeLimit time.Duration

	PayloadDataTTL       time.Duration
	RegistrationCacheTTL time.Duration

	Distributed, StreamServedBids, PublishBlock bool

	AllowedListedBuilders map[[48]byte]struct{}
}

func (rc *RelayConfig) ParseInitialConfig(keys []string) (err error) {
	rc.AllowedListedBuilders, err = makeKeyMap(keys)
	return err

}

func (rc *RelayConfig) OnConfigChange(c structs.OldNew) (err error) {
	switch c.Name {
	case "PublishBlock":
		if b, ok := c.New.(bool); ok {
			rc.PublishBlock = b
			rc.L.With(log.F{"param": "PublishBlock", "value": b}).Info("config param updated")
		}

	case "StreamServedBids":
		if b, ok := c.New.(bool); ok {
			rc.StreamServedBids = b
			rc.L.With(log.F{"param": "PublishBlock", "value": b}).Info("config param updated")
		}

	case "GetPayloadResponseDelay":
		if dur, ok := c.New.(time.Duration); ok {
			rc.GetPayloadResponseDelay = dur
			rc.L.With(log.F{"param": "GetPayloadResponseDelay", "value": dur}).Info("config param updated")
		}

	case "GetPayloadRequestTimeLimit":
		if dur, ok := c.New.(time.Duration); ok {
			rc.GetPayloadRequestTimeLimit = dur
			rc.L.With(log.F{"param": "GetPayloadRequestTimeLimit", "value": dur}).Info("config param updated")
		}

	case "AllowedBuilders":
		if keys, ok := c.New.([]string); ok {
			ab, err := makeKeyMap(keys)
			if err != nil {
				return err
			}
			rc.AllowedListedBuilders = ab
			rc.L.With(log.F{"param": "AllowedListedBuilders", "value": ab}).Info("config param updated")
		}
	}
	return nil
}

func makeKeyMap(keys []string) (map[[48]byte]struct{}, error) {
	newKeys := make(map[[48]byte]struct{})
	for _, key := range keys {
		var pk types.PublicKey
		if err := pk.UnmarshalText([]byte(key)); err != nil {
			return nil, fmt.Errorf("allowed builder not added - wrong public key: %s  - %w", key, err)
		}
		newKeys[pk] = struct{}{}
	}
	return newKeys, nil
}
