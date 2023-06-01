package redis

import (
	"context"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/redis/go-redis/v9"
)

type RedisDatastore struct {
	Read, Write *redis.Client
}

func (r *RedisDatastore) PutWithTTL(ctx context.Context, key ds.Key, b []byte, ttl time.Duration) error {
	return r.Write.Set(ctx, key.String(), b, ttl).Err()
}

func (r *RedisDatastore) Get(ctx context.Context, key ds.Key) ([]byte, error) {
	cmd := r.Read.Get(ctx, key.String())
	s, err := cmd.Result()
	return []byte(s), err
}
