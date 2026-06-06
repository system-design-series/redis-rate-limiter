package RateLimiter

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

type Limiter struct {
	algorithm string
	// store, config, redis client, etc.
}

func New(algo string) *Limiter {
	return &Limiter{
		algorithm: "token_bucket",
	}
}
func (l *Limiter) RateCheck(context *echo.Context) error {
	apiKey := context.Request().Header.Get("x-api-key")
	isAllowed, err := l.validate(apiKey)
	if err != nil {
		return err
	}
	if !isAllowed {
		return context.String(http.StatusTooManyRequests, "rate limit exceeded")
	}
	return nil
}
