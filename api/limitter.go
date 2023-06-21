package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/lthibault/log"
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
	l                 log.Logger
}

func NewLimitter(l log.Logger, ratel int, burst int, c Cache) *Limitter {
	return &Limitter{
		c:         c,
		RateLimit: rate.Limit(ratel),
		Burst:     burst,
		l:         l,
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

func (l *Limitter) ParseInitialConfig(keys []string) (err error) {
	l.AllowedBuilders, err = makeKeyMap(keys)
	return err

}

func (l *Limitter) OnConfigChange(c structs.OldNew) (err error) {
	switch c.Name {
	case "SubmissionLimitRate":
		if i, ok := c.New.(int64); ok {
			l.RateLimit = rate.Limit(i)
			l.c.Purge()
			l.l.With(log.F{"param": "SubmissionLimitRate", "value": i}).Info("config param updated")
		}
	case "SubmissionLimitBurst":
		if i, ok := c.New.(int64); ok {
			l.Burst = int(i)
			l.c.Purge()
			l.l.With(log.F{"param": "SubmissionLimitBurst", "value": i}).Info("config param updated")
		}
	case "AllowedBuilders":
		if keys, ok := c.New.([]string); ok {
			ab, err := makeKeyMap(keys)
			if err != nil {
				return err
			}
			l.c.Purge()
			l.AllowedBuilders = ab
			l.l.With(log.F{"param": "AllowedBuilders", "value": ab}).Info("config param updated")
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
