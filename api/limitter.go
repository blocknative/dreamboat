package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	"golang.org/x/time/rate"
)

var ErrTooManyCalls = errors.New("too many calls")

type Cache interface {
	Get([48]byte) (*rate.Limiter, bool)
	Add([48]byte, *rate.Limiter) bool
	Purge()
}
type Limitter struct {
	AllowedBuilders   map[[48]byte]struct{}
	c                 Cache
	RateLimit         rate.Limit
	Burst             int
	LimitterCacheSize int
}

func NewLimitter(ratel int, burst int, c Cache, ab map[[48]byte]struct{}) *Limitter {

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

func (l *Limitter) OnConfigChange(c structs.OldNew) (err error) {
	switch c.Name {
	case "SubmissionLimitRate":
		if i, ok := c.New.(int); ok {
			l.RateLimit = rate.Limit(i)
			l.c.Purge()
		}
	case "SubmissionLimitBurst":
		if i, ok := c.New.(int); ok {
			l.Burst = i
			l.c.Purge()
		}
	case "AllowedBuilders":
		if keys, ok := c.New.([]string); ok {
			newKeys := make(map[[48]byte]struct{})
			for _, key := range keys {
				var pk types.PublicKey
				if err = pk.UnmarshalText([]byte(key)); err != nil {
					return fmt.Errorf("ALLOWED BUILDER NOT ADDED - wrong public key: %s  - %w", key, err)
				}
				newKeys[pk] = struct{}{}
			}
			l.AllowedBuilders = newKeys
		}
	}
	return nil
}
