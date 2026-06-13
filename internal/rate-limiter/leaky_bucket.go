package RateLimiter

import (
	"context"
	_ "embed"
	"fmt"
	"math"

	redis "github.com/redis/go-redis/v9"
)

//go:embed leaky_bucket.lua
var leakyBucketScript string

// LeakyBucket enforces a per-identity leaky bucket in Redis.
//
// Algorithm (leaky bucket as a meter):
//
//	Picture a bucket with a small hole in the bottom. Every request pours one
//	unit of water in; the hole drains water at a steady "leak rate". The bucket
//	holds at most `capacity` units. A request is allowed only if its unit fits
//	without overflowing; otherwise it is rejected. Because water always leaves
//	at the same rate, the output is perfectly smooth: short bursts are absorbed
//	by the spare room in the bucket, but the sustained rate can never exceed the
//	leak rate.
//
// Leaky vs token bucket: a token bucket lets you spend a whole burst at once
// (up to capacity) and then refills, so allowed traffic stays bursty. A leaky
// bucket meters the drain so throughput comes out even. Both cap the long-run
// average; they differ in how bursty the allowed traffic is permitted to look.
//
// State and the leak+admit step run entirely server-side (Lua), so it is safe
// across concurrent callers and service replicas sharing one Redis.
type LeakyBucket struct {
	client    *redis.Client
	script    *redis.Script
	capacity  int
	leakRate  int
	keyPrefix string
}

// NewLeakyBucket builds a bucket. capacity is the bucket size (burst room, in
// units); leakRate is units drained per second (the sustained throughput);
// keyPrefix namespaces Redis keys.
func NewLeakyBucket(client *redis.Client, capacity, leakRate int, keyPrefix string) *LeakyBucket {
	return &LeakyBucket{
		client:    client,
		script:    redis.NewScript(leakyBucketScript),
		capacity:  capacity,
		leakRate:  leakRate,
		keyPrefix: keyPrefix,
	}
}

// Allow adds one unit for identity and returns the decision. A non-nil error
// means Redis was unreachable/failed; callers apply their fail mode.
func (l *LeakyBucket) Allow(ctx context.Context, identity string) (Decision, error) {
	key := l.keyPrefix + ":lb:" + identity
	const cost = 1
	res, err := l.script.Run(ctx, l.client, []string{key}, l.capacity, l.leakRate, cost).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(res) < 4 {
		return Decision{}, fmt.Errorf("leaky bucket script returned %d values, want 4", len(res))
	}
	// res = { allowed, remaining, retry_after_ms, reset_after_ms }
	return Decision{
		Allowed:       res[0] == 1,
		Limit:         l.capacity,
		Remaining:     int(res[1]),
		RetryAfterSec: int(math.Ceil(float64(res[2]) / 1000.0)),
		ResetAfterSec: int(math.Ceil(float64(res[3]) / 1000.0)),
	}, nil
}
