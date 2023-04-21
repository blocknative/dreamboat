package api

import (
	"context"
	"errors"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

const LimitterCacheSize = 100

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

//OnConfigChange(change structs.OldNew)
