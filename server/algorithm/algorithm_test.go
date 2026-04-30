package algorithm

import (
	"context"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available at localhost:6379, skipping: %v", err)
	}
	return rdb
}

func TestTokenBucket_AllowsUpToCapacity(t *testing.T) {
	rdb := newTestRedis(t)
	tb := NewTokenBucket(rdb)
	ctx := context.Background()

	clientID := fmt.Sprintf("test-tb-%d", uniqueID())
	capacity := int32(5)
	rate := int32(1)

	for i := 0; i < int(capacity); i++ {
		allowed, _, _, err := tb.Allow(ctx, clientID, capacity, rate)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("request %d: expected allowed, got denied", i+1)
		}
	}

	allowed, _, retryAfter, err := tb.Allow(ctx, clientID, capacity, rate)
	if err != nil {
		t.Fatalf("request 6: unexpected error: %v", err)
	}
	if allowed {
		t.Error("request 6: expected denied, got allowed")
	}
	if retryAfter <= 0 {
		t.Error("expected positive retry_after_ms when denied")
	}
}

func TestSlidingWindow_AllowsUpToCapacity(t *testing.T) {
	rdb := newTestRedis(t)
	sw := NewSlidingWindow(rdb)
	ctx := context.Background()

	clientID := fmt.Sprintf("test-sw-%d", uniqueID())
	capacity := int32(5)
	windowMs := int64(5000)

	for i := 0; i < int(capacity); i++ {
		allowed, _, _, err := sw.Allow(ctx, clientID, capacity, windowMs)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("request %d: expected allowed, got denied", i+1)
		}
	}

	allowed, _, retryAfter, err := sw.Allow(ctx, clientID, capacity, windowMs)
	if err != nil {
		t.Fatalf("request 6: unexpected error: %v", err)
	}
	if allowed {
		t.Error("request 6: expected denied, got allowed")
	}
	if retryAfter <= 0 {
		t.Error("expected positive retry_after_ms when denied")
	}
}

var idCounter int64

func uniqueID() int64 {
	idCounter++
	return idCounter
}
