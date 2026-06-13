package RateLimiter

import (
	"context"
	_ "embed"
	"fmt"
	"math"

	redis "github.com/redis/go-redis/v9"
)

//go:embed sliding_window_counter.lua
var slidingWindowCounterScript string

// SlidingWindowCounter enforces a per-identity sliding-window-counter limit in
// Redis.
//
// Algorithm (sliding window counter):
//
//	A plain fixed window ("max N per 60s") is cheap but has an edge problem: a
//	caller can send N at 0:59 and another N at 1:00 (2N in two seconds) because
//	the counter resets hard at the boundary. The sliding window counter smooths
//	that by blending the current fixed window with the previous one.
//
//	It keeps a count for the current window and the previous one, then estimates
//	the rolling count as:
//
//	    estimate = current_count + previous_count * (overlap fraction)
//
//	where the overlap fraction is how much of the previous window still falls
//	inside a rolling window that ends "now". As time moves through the current
//	window that fraction shrinks from 1 toward 0, so the previous window's
//	weight fades out smoothly instead of dropping off a cliff. A request is
//	allowed when `estimate + cost <= limit`.
//
//	This is an approximation (it assumes the previous window's traffic was
//	evenly spread), but it is O(1) in time and memory and removes almost all of
//	the fixed-window boundary burst, which is why it's a common production
//	choice.
//
// State and the estimate+admit step run entirely server-side (Lua), so it is
// safe across concurrent callers and service replicas sharing one Redis.
type SlidingWindowCounter struct {
	client    *redis.Client
	script    *redis.Script
	limit     int
	windowSec int
	keyPrefix string
}

// NewSlidingWindowCounter builds a limiter. limit is the max requests per
// window; windowSec is the window length in seconds; keyPrefix namespaces Redis
// keys.
func NewSlidingWindowCounter(client *redis.Client, limit, windowSec int, keyPrefix string) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		client:    client,
		script:    redis.NewScript(slidingWindowCounterScript),
		limit:     limit,
		windowSec: windowSec,
		keyPrefix: keyPrefix,
	}
}

// Allow counts one request for identity and returns the decision. A non-nil
// error means Redis was unreachable/failed; callers apply their fail mode.
func (s *SlidingWindowCounter) Allow(ctx context.Context, identity string) (Decision, error) {
	key := s.keyPrefix + ":swc:" + identity
	const cost = 1
	res, err := s.script.Run(ctx, s.client, []string{key}, s.limit, s.windowSec, cost).Int64Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(res) < 4 {
		return Decision{}, fmt.Errorf("sliding window counter script returned %d values, want 4", len(res))
	}
	// res = { allowed, remaining, retry_after_ms, reset_after_ms }
	return Decision{
		Allowed:       res[0] == 1,
		Limit:         s.limit,
		Remaining:     int(res[1]),
		RetryAfterSec: int(math.Ceil(float64(res[2]) / 1000.0)),
		ResetAfterSec: int(math.Ceil(float64(res[3]) / 1000.0)),
	}, nil
}
