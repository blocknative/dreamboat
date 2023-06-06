package stream

import (
	"context"

	"github.com/lthibault/log"
	"github.com/redis/go-redis/v9"
)

type Pubsub struct {
	Redis  *redis.Client
	Logger log.Logger
}

func (r *Pubsub) Publish(ctx context.Context, topic string, data []byte) error {
	return r.Redis.Publish(ctx, topic, data).Err()
}

func (r *Pubsub) Subscribe(ctx context.Context, topic string) chan []byte {
	logger := r.Logger.WithField("topic", topic)

	sub := make(chan []byte)
	go func() {
		defer close(sub)
		for ctx.Err() == nil { // restart on failure
			pubsub := r.Redis.Subscribe(ctx, topic)
			logger.Debug("redis subscription started")

			redisSub := pubsub.Channel()
			for data := range redisSub {
				select {
				case sub <- []byte(data.Payload):
				case <-ctx.Done():
					return
				}
			}
			logger.Warn("redis subscription closed")
		}

	}()

	return sub
}
