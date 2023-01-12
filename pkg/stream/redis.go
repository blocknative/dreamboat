package stream

import (
	"context"

	"github.com/go-redis/redis/v8"
	"github.com/lthibault/log"
)

type RedisPubsub struct {
	Redis  *redis.Client
	Logger log.Logger
}

func (r *RedisPubsub) Publish(ctx context.Context, topic string, data []byte) error {
	return r.Redis.Publish(ctx, topic, data).Err()
}

func (r *RedisPubsub) Subscribe(ctx context.Context, topic string) (chan []byte, error) {
	sub := make(chan []byte)

	go func() {
		defer close(sub)
		for ctx.Err() == nil { // restart on failure
			pubsub := r.Redis.Subscribe(ctx, topic)
			r.Logger.Debug("redis subscription started")

			redisSub := pubsub.Channel()
			for data := range redisSub {
				select {
				case sub <- []byte(data.Payload):
				case <-ctx.Done():
					return
				}
			}
			r.Logger.Warn("redis subscription closed")
		}

	}()

	return sub, nil
}
