package api

import (
	"context"
	"errors"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"
)

var ErrTooManyCalls = errors.New("too many calls")

type Limitter struct {
	AllowedBuilders map[[48]byte]struct{}
	c               *lru.Cache[[48]byte, *rate.Limiter]
}

func NewLimitter(ab map[[48]byte]struct{}) *Limitter {
	c, _ := lru.New[[48]byte, *rate.Limiter](100)
	return &Limitter{
		AllowedBuilders: ab,
		c:               c,
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
		lim := rate.NewLimiter(rate.Every(time.Second), 1)
		l.c.Add(pubkey, lim)
	}

	if !lim.Allow() {
		return ErrTooManyCalls
	}

	return nil

}
