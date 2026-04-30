package algorithm

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// SlidingWindow implements the sliding window log algorithm using Redis.
// It tracks the timestamp of every request in a sorted set.
// Requests outside the window are pruned on each check.
type SlidingWindow struct {
	rdb *redis.Client
}

func NewSlidingWindow(rdb *redis.Client) *SlidingWindow {
	return &SlidingWindow{rdb: rdb}
}

// Allow checks whether a request from clientID is permitted.
// capacity = max requests allowed in the window
// windowMs = size of the sliding window in milliseconds
func (sw *SlidingWindow) Allow(ctx context.Context, clientID string, capacity int32, windowMs int64) (allowed bool, remaining int32, retryAfterMs int64, err error) {
	key := fmt.Sprintf("slidingwindow:%s", clientID)
	now := time.Now().UnixMilli()
	windowStart := now - windowMs

	// Lua script runs atomically — prune old entries, count, then conditionally add
	script := redis.NewScript(`
		local key         = KEYS[1]
		local now         = tonumber(ARGV[1])
		local windowStart = tonumber(ARGV[2])
		local capacity    = tonumber(ARGV[3])
		local windowMs    = tonumber(ARGV[4])

		-- remove timestamps outside the current window
		redis.call("ZREMRANGEBYSCORE", key, "-inf", windowStart)

		local count = redis.call("ZCARD", key)

		local allowed   = 0
		local remaining = capacity - count

		if count < capacity then
			-- use now as score and a unique member to allow multiple requests per ms
			redis.call("ZADD", key, now, now .. "-" .. math.random(1, 1000000))
			allowed   = 1
			remaining = remaining - 1
		end

		-- expire the key slightly after the window to clean up Redis memory
		redis.call("PEXPIRE", key, windowMs + 1000)

		return {allowed, remaining}
	`)

	result, err := script.Run(ctx, sw.rdb, []string{key},
		now, windowStart, capacity, windowMs).Int64Slice()
	if err != nil {
		return false, 0, 0, fmt.Errorf("sliding window redis error: %w", err)
	}

	allowed = result[0] == 1
	remaining = int32(result[1])

	if !allowed {
		// oldest entry in the window tells us when a slot opens up
		oldest, zErr := sw.rdb.ZRangeWithScores(ctx, key, 0, 0).Result()
		if zErr == nil && len(oldest) > 0 {
			oldestMs := int64(oldest[0].Score)
			retryAfterMs = (oldestMs + windowMs) - now
			if retryAfterMs < 0 {
				retryAfterMs = 0
			}
		}
	}

	return allowed, remaining, retryAfterMs, nil
}