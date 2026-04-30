package algorithm

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type TokenBucket struct {
	rdb *redis.Client
}

func NewTokenBucket(rdb *redis.Client) *TokenBucket {
	return &TokenBucket{rdb: rdb}
}

func (tb *TokenBucket) Allow(ctx context.Context, clientID string, capacity, rate int32) (allowed bool, remaining int32, retryAfterMs int64, err error) {
	key := fmt.Sprintf("tokenbucket:%s", clientID)
	lastKey := fmt.Sprintf("tokenbucket:%s:last", clientID)
	now := time.Now().UnixMilli()

	script := redis.NewScript(`
local key      = KEYS[1]
local lastKey  = KEYS[2]
local now      = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local rate     = tonumber(ARGV[3])
local last     = tonumber(redis.call("GET", lastKey) or now)
local tokens   = tonumber(redis.call("GET", key) or capacity)
local elapsed  = math.max(0, now - last)
local refill   = math.floor(elapsed * rate / 1000)
tokens = math.min(capacity, tokens + refill)
local allowed   = 0
local remaining = tokens
if tokens >= 1 then
  tokens    = tokens - 1
  allowed   = 1
  remaining = tokens
end
redis.call("SET", key,     tokens, "PX", 60000)
redis.call("SET", lastKey, now,    "PX", 60000)
return {allowed, remaining}
`)

	result, err := script.Run(ctx, tb.rdb, []string{key, lastKey},
		now, capacity, rate).Int64Slice()
	if err != nil {
		return false, 0, 0, fmt.Errorf("token bucket redis error: %w", err)
	}

	allowed = result[0] == 1
	remaining = int32(result[1])
	if !allowed {
		retryAfterMs = int64(1000 / rate)
	}
	return allowed, remaining, retryAfterMs, nil
}
