package redis

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	ds "github.com/ipfs/go-datastore"
)

type RedisDatastore struct {
	Redis *redis.Client
}

func (r *RedisDatastore) PutWithTTL(ctx context.Context, key ds.Key, b []byte, ttl time.Duration) error {
	return r.Redis.Set(ctx, key.String(), b, ttl).Err()
}

func (r *RedisDatastore) Get(ctx context.Context, key ds.Key) ([]byte, error) {
	cmd := r.Redis.Get(ctx, key.String())
	s, err := cmd.Result()
	return []byte(s), err
}
