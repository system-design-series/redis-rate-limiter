package RateLimiter

import (
	"context"
	_ "embed"
	"fmt"
	"math"

	redis "github.com/redis/go-redis/v9"
)

//go:embed token_bucket.lua
var tokenBucketScript string

// Decision is the verdict for a single rate-check.
type Decision struct {
	Allowed       bool
	Limit         int // capacity
	Remaining     int // floor(tokens) after this request
	ResetAfterSec int // seconds until the bucket is full again (0 when already full)
	RetryAfterSec int // seconds until >=1 token (meaningful when !Allowed)
}

// TokenBucket enforces a per-identity token bucket in Redis. State and the
// refill+decrement are entirely server-side (Lua), so it is safe across many
// concurrent callers and service replicas sharing one Redis.
type TokenBucket struct {
	client     *redis.Client
	script     *redis.Script
	capacity   int
	refillRate int
	keyPrefix  string
}

// NewTokenBucket builds a bucket. capacity is the burst ceiling; refillRate is
// tokens added per second; keyPrefix namespaces Redis keys.
func NewTokenBucket(client *redis.Client, capacity, refillRate int, keyPrefix string) *TokenBucket {
	return &TokenBucket{
		client:     client,
		script:     redis.NewScript(tokenBucketScript),
		capacity:   capacity,
		refillRate: refillRate,
		keyPrefix:  keyPrefix,
	}
}

// Allow consumes one token for identity and returns the decision. A non-nil
// error means Redis was unreachable/failed; callers apply their fail mode.
func (t *TokenBucket) Allow(ctx context.Context, identity string) (Decision, error) {
	key := t.keyPrefix + ":tb:" + identity
	const cost = 1
	res, err := t.script.Run(ctx, t.client, []string{key}, t.capacity, t.refillRate, cost).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(res) < 4 {
		return Decision{}, fmt.Errorf("token bucket script returned %d values, want 4", len(res))
	}
	// res = { allowed, remaining, retry_after_ms, reset_after_ms }
	return Decision{
		Allowed:       res[0] == 1,
		Limit:         t.capacity,
		Remaining:     int(res[1]),
		RetryAfterSec: int(math.Ceil(float64(res[2]) / 1000.0)),
		ResetAfterSec: int(math.Ceil(float64(res[3]) / 1000.0)),
	}, nil
}
