package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"RateLimiter/internal/config"
	RateLimiter "RateLimiter/internal/rate-limiter"
	"RateLimiter/internal/redisx"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	client := redisx.New(cfg)
	defer client.Close()

	bucket := RateLimiter.NewTokenBucket(client, cfg.Capacity, cfg.RefillRate, cfg.KeyPrefix)
	limiter := RateLimiter.New(bucket, cfg.FailMode)

	e := echo.New()

	e.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	// Liveness: process is up.
	e.GET("/healthz", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	// Readiness: Redis reachable. Independent of the request-time fail mode.
	e.GET("/readyz", func(c *echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 500*time.Millisecond)
		defer cancel()
		if err := redisx.Ping(ctx, client); err != nil {
			return c.String(http.StatusServiceUnavailable, "redis unreachable")
		}
		return c.NoContent(http.StatusOK)
	})

	api := e.Group("/api")
	api.Use(middleware.RequestLogger())
	api.Use(middleware.Recover())
	api.GET("/rate-check", limiter.RateCheck)

	if err := e.Start(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
