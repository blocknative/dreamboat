package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/go-redis/redis/v8"
)

type RedisDatastore struct {
	Redis *redis.Client
}

func (r *RedisDatastore) GetPayload(ctx context.Context, key structs.PayloadKey) (*structs.BlockAndTrace, bool, error) {
	cmd := r.Redis.Get(ctx, payloadKeyToRedisKey(key))
	redisPayload, err := cmd.Result()
	if err != nil {
		return nil, false, fmt.Errorf("fail to get payload from Redis: %w", err)
	}

	var block structs.BlockAndTrace
	return &block, false, json.Unmarshal([]byte(redisPayload), &block)
}

func (r *RedisDatastore) PutPayload(ctx context.Context, key structs.PayloadKey, payload *structs.BlockAndTrace, ttl time.Duration) error {
	redisPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("fail to encode payload: %w", err)
	}

	return r.Redis.Set(ctx, payloadKeyToRedisKey(key), redisPayload, ttl).Err()
}

func payloadKeyToRedisKey(key structs.PayloadKey) string {
	return fmt.Sprintf("p-%d-%s-%s", key.Slot, key.Proposer, key.BlockHash) // TODO: optimize key size
}
