package RateLimiter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"RateLimiter/internal/config"

	"github.com/labstack/echo/v5"
)

type stubBucket struct {
	dec Decision
	err error
}

func (s stubBucket) Allow(ctx context.Context, identity string) (Decision, error) {
	return s.dec, s.err
}

func newServer(b Bucket, fm config.FailMode) *echo.Echo {
	e := echo.New()
	l := New(b, fm)
	e.GET("/api/rate-check", l.RateCheck)
	return e
}

func do(e *echo.Echo, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/rate-check", nil)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestRateCheck_Allowed(t *testing.T) {
	e := newServer(stubBucket{dec: Decision{Allowed: true, Limit: 100, Remaining: 87, ResetAfterSec: 42}}, config.FailClosed)
	rec := do(e, "k1")
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "87" {
		t.Errorf("X-RateLimit-Remaining=%q, want 87", got)
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "100" {
		t.Errorf("X-RateLimit-Limit=%q, want 100", got)
	}
	if rec.Header().Get("Retry-After") != "" {
		t.Error("Retry-After must be absent on 200")
	}
}

func TestRateCheck_Denied(t *testing.T) {
	e := newServer(stubBucket{dec: Decision{Allowed: false, Limit: 100, Remaining: 0, RetryAfterSec: 12, ResetAfterSec: 12}}, config.FailClosed)
	rec := do(e, "k1")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status=%d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "12" {
		t.Errorf("Retry-After=%q, want 12", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining=%q, want 0", got)
	}
}

func TestRateCheck_FailClosed(t *testing.T) {
	e := newServer(stubBucket{err: errors.New("redis down")}, config.FailClosed)
	rec := do(e, "k1")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status=%d, want 429 (fail closed)", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After=%q, want 1", got)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("X-RateLimit-* must be absent when failing closed")
	}
}

func TestRateCheck_FailOpen(t *testing.T) {
	e := newServer(stubBucket{err: errors.New("redis down")}, config.FailOpen)
	rec := do(e, "k1")
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200 (fail open)", rec.Code)
	}
}
