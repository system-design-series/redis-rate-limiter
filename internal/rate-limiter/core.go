package RateLimiter

import (
	"context"
	"net/http"
	"strconv"

	"RateLimiter/internal/config"

	"github.com/labstack/echo/v5"
)

// Bucket is the rate-limiting decision source. TokenBucket implements it;
// tests provide a stub.
type Bucket interface {
	Allow(ctx context.Context, identity string) (Decision, error)
}

// Limiter adapts a Bucket to the HTTP rate-check endpoint and applies the
// configured fail mode when the bucket errors.
type Limiter struct {
	bucket   Bucket
	failMode config.FailMode
}

func New(bucket Bucket, failMode config.FailMode) *Limiter {
	return &Limiter{bucket: bucket, failMode: failMode}
}

// RateCheck consumes a token for the request's identity and returns 200 (allow)
// or 429 (deny). On a bucket/Redis error it applies the fail mode.
func (l *Limiter) RateCheck(c *echo.Context) error {
	identity := c.Request().Header.Get("x-api-key")
	if identity == "" {
		// Deliberate: unauthenticated callers share one "anon" bucket so keyless
		// traffic is still rate-limited rather than bypassing the limiter. Note
		// this is a single shared bucket (noisy-neighbor tradeoff for keyless calls).
		identity = "anon"
	}

	dec, err := l.bucket.Allow(c.Request().Context(), identity)
	if err != nil {
		if l.failMode == config.FailOpen {
			return c.NoContent(http.StatusOK)
		}
		c.Response().Header().Set("Retry-After", "1")
		return c.String(http.StatusTooManyRequests, "rate limit exceeded")
	}

	setRateLimitHeaders(c, dec)
	if !dec.Allowed {
		c.Response().Header().Set("Retry-After", strconv.Itoa(dec.RetryAfterSec))
		return c.String(http.StatusTooManyRequests, "rate limit exceeded")
	}
	return c.NoContent(http.StatusOK)
}

func setRateLimitHeaders(c *echo.Context, d Decision) {
	h := c.Response().Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
	h.Set("X-RateLimit-Reset", strconv.Itoa(d.ResetAfterSec))
}
