package redis_stream

import (
	"context"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/stream/transport"
	"github.com/google/uuid"
	"github.com/jpillora/backoff"
	"github.com/lthibault/log"
	"github.com/redis/go-redis/v9"
)

type Topic struct {
	Redis     *redis.Client
	Logger    log.Logger
	Name      string
	LocalNode uuid.UUID
}

func (t *Topic) String() string {
	return t.Name
}

func (t *Topic) Publish(ctx context.Context, m transport.Message) error {
	m.Source = t.LocalNode

	b, err := transport.Encode(m)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	return t.Redis.Publish(ctx, t.Name, b).Err()
}

func (t *Topic) Subscribe(ctx context.Context) transport.Subscription {
	return Subscription{
		Logger:    t.Logger,
		PubSub:    t.Redis.Subscribe(ctx, t.Name),
		LocalNode: t.LocalNode,
	}
}

type Subscription struct {
	*redis.PubSub
	Logger    log.Logger
	LocalNode uuid.UUID
}

func (s Subscription) Next(ctx context.Context) (transport.Message, error) {
	var b = backoff.Backoff{
		Min:    time.Millisecond,
		Max:    time.Millisecond * 100,
		Jitter: true,
	}

	for {
		rawMsg, err := s.ReceiveMessage(ctx)
		switch err {
		case redis.ErrClosed, context.Canceled:
			return transport.Message{}, err
		}

		if err != nil {
			s.Logger.
				WithError(err).
				WithField("attempt", b.Attempt()).
				WithField("backoff", b.ForAttempt(b.Attempt())).
				Warn("failed to get subscription message from redis")
			select {
			case <-time.After(b.Duration()):
			case <-ctx.Done():
				return transport.Message{}, ctx.Err()
			}

			continue
		}

		b.Reset()

		msg, err := transport.Decode([]byte(rawMsg.Payload))
		if err != nil {
			s.Logger.
				WithError(err).
				Error("failed to decode subscription message from redis")
			continue
		}

		// skip the message if we originally published it
		if msg.Source != s.LocalNode {
			return msg, nil
		}
	}
}
