// Package redisx builds the configured Redis client for the rate limiter.
// It is named redisx (not redis) to avoid colliding with the upstream
// go-redis package, which callers import directly.
package redisx

import (
	"context"

	"RateLimiter/internal/config"

	redis "github.com/redis/go-redis/v9"
)

// New constructs a Redis client from config. It does not dial; the first
// command (or a Ping) establishes the connection.
func New(cfg config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        cfg.RedisAddr,
		Password:    cfg.RedisPassword,
		DB:          cfg.RedisDB,
		PoolSize:    cfg.RedisPoolSize,
		DialTimeout: cfg.DialTimeout,
		ReadTimeout: cfg.ReadTimeout,
	})
}

// Ping checks Redis reachability for readiness probes.
func Ping(ctx context.Context, c *redis.Client) error {
	return c.Ping(ctx).Err()
}
