package api

import (
	"context"
	"errors"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

const LimitterCacheSize = 400

var ErrTooManyCalls = errors.New("too many calls")

type Limitter struct {
	AllowedBuilders   map[[48]byte]struct{}
	c                 *lru.Cache[[48]byte, *rate.Limiter]
	RateLimit         rate.Limit
	Burst             int
	LimitterCacheSize int
}

func NewLimitter(ratel int, burst int, ab map[[48]byte]struct{}) *Limitter {
	c, _ := lru.New[[48]byte, *rate.Limiter](LimitterCacheSize)
	return &Limitter{
		AllowedBuilders: ab,
		c:               c,
		RateLimit:       rate.Limit(ratel),
		Burst:           burst,
	}
}

func (l *Limitter) Allow(ctx context.Context, pubkey [48]byte) error {
	if l.AllowedBuilders != nil {
		if _, ok := l.AllowedBuilders[pubkey]; ok {
			return nil
		}
	}
	lim, ok := l.c.Get(pubkey)
	if !ok {
		lim = rate.NewLimiter(l.RateLimit, l.Burst)
		l.c.Add(pubkey, lim)
	}

	if !lim.Allow() {
		return ErrTooManyCalls
	}

	return nil

}

func (l *Limitter) OnConfigChange(c structs.OldNew) {
	switch c.Name {
	case "AllowedBuilders":
		if keys, ok := c.New.([]string); ok {
			newKeys := make(map[[48]byte]struct{})
			for _, key := range keys {
				var pk types.PublicKey
				if err := pk.UnmarshalText([]byte(key)); err != nil {
					//logger.WithError(err).With(log.F{"key": k}).Error("ALLOWED BUILDER NOT ADDED - wrong public key")
					continue
				}
				newKeys[pk] = struct{}{}
			}

			//rc.AllowedListedBuilders = newKeys
		}
	}
}
