package RateLimiter

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// testClient returns a Redis client, skipping the test if Redis is unreachable.
func testClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	return c
}

// uniqueIdentity yields a per-test identity so tests don't share buckets.
func uniqueIdentity(t *testing.T) string {
	return fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
}

func TestAllow_BurstThenDeny(t *testing.T) {
	client := testClient(t)
	const capacity, refill = 5, 1
	tb := NewTokenBucket(client, capacity, refill, "rltest")
	id := uniqueIdentity(t)
	t.Cleanup(func() { client.Del(context.Background(), "rltest:tb:"+id) })

	ctx := context.Background()
	allowed := 0
	for i := 0; i < capacity+3; i++ {
		dec, err := tb.Allow(ctx, id)
		if err != nil {
			t.Fatalf("Allow error: %v", err)
		}
		if dec.Allowed {
			allowed++
		}
	}
	if allowed != capacity {
		t.Errorf("allowed=%d, want exactly capacity=%d", allowed, capacity)
	}

	dec, _ := tb.Allow(ctx, id)
	if dec.Allowed {
		t.Fatal("expected denial after capacity exhausted")
	}
	if dec.RetryAfterSec <= 0 {
		t.Errorf("RetryAfterSec=%d, want > 0 on denial", dec.RetryAfterSec)
	}
	if dec.Limit != capacity {
		t.Errorf("Limit=%d, want %d", dec.Limit, capacity)
	}
}

func TestAllow_Refill(t *testing.T) {
	client := testClient(t)
	const capacity, refill = 2, 5 // 5 tokens/sec → ~200ms per token
	tb := NewTokenBucket(client, capacity, refill, "rltest")
	id := uniqueIdentity(t)
	t.Cleanup(func() { client.Del(context.Background(), "rltest:tb:"+id) })

	ctx := context.Background()
	for i := 0; i < capacity; i++ {
		if dec, _ := tb.Allow(ctx, id); !dec.Allowed {
			t.Fatalf("request %d should have been allowed", i)
		}
	}
	if dec, _ := tb.Allow(ctx, id); dec.Allowed {
		t.Fatal("bucket should be empty")
	}

	time.Sleep(400 * time.Millisecond)
	if dec, _ := tb.Allow(ctx, id); !dec.Allowed {
		t.Fatal("expected a refilled token to be available after wait")
	}
}

func TestAllow_ConcurrentNoOverAdmission(t *testing.T) {
	client := testClient(t)
	const capacity, refill = 30, 1
	tb := NewTokenBucket(client, capacity, refill, "rltest")
	id := uniqueIdentity(t)
	t.Cleanup(func() { client.Del(context.Background(), "rltest:tb:"+id) })

	ctx := context.Background()
	const total = capacity * 4
	var allowed int64
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if dec, err := tb.Allow(ctx, id); err == nil && dec.Allowed {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	wg.Wait()

	got := atomic.LoadInt64(&allowed)
	if got < capacity || got > capacity+2 {
		t.Errorf("allowed=%d, want in [%d, %d] (no over-admission)", got, capacity, capacity+2)
	}
}
