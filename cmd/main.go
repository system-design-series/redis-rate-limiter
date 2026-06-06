package main

import (
	RateLimiter "RateLimiter/internal/rate-limiter"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	echoServer := echo.New()
	echoServer.GET("/", func(c *echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	api := echoServer.Group("/api")
	rateLimitModule := RateLimiter.New("token_bucket") // We can add any dependancies here
	api.GET("/rate-check", rateLimitModule.RateCheck)
	api.Use(middleware.RequestLogger())
	api.Use(middleware.Recover())
	if err := echoServer.Start(":8080"); err != nil {
		panic(err)
	}
}
