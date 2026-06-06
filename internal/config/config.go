// Package config loads and validates the rate limiter's runtime configuration
// from environment variables. Invalid values fail fast at boot.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type FailMode string

const (
	FailOpen   FailMode = "open"
	FailClosed FailMode = "closed"
)

// Config is the validated runtime configuration.
type Config struct {
	Port string

	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisPoolSize int
	DialTimeout   time.Duration
	ReadTimeout   time.Duration

	Capacity   int // max tokens (burst ceiling)
	RefillRate int // tokens added per second

	FailMode  FailMode
	KeyPrefix string
	Scheme    string
}

// Load reads configuration from the environment, applies defaults for unset or
// empty values, and validates the result. A validation failure is returned as an
// error so the caller can fail fast at startup.
func Load() (Config, error) {
	c := Config{
		Port:          getEnv("RL_PORT", "8080"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		KeyPrefix:     getEnv("RL_KEY_PREFIX", "rl"),
		Scheme:        getEnv("RL_SCHEME", "token_bucket"),
		FailMode:      FailMode(getEnv("RL_FAIL_MODE", "closed")),
	}

	var err error
	if c.RedisDB, err = getEnvInt("REDIS_DB", 0); err != nil {
		return Config{}, err
	}
	if c.RedisPoolSize, err = getEnvInt("REDIS_POOL_SIZE", 50); err != nil {
		return Config{}, err
	}
	dialMs, err := getEnvInt("REDIS_DIAL_TIMEOUT_MS", 200)
	if err != nil {
		return Config{}, err
	}
	readMs, err := getEnvInt("REDIS_READ_TIMEOUT_MS", 100)
	if err != nil {
		return Config{}, err
	}
	c.DialTimeout = time.Duration(dialMs) * time.Millisecond
	c.ReadTimeout = time.Duration(readMs) * time.Millisecond

	if c.Capacity, err = getEnvInt("RL_BUCKET_CAPACITY", 100); err != nil {
		return Config{}, err
	}
	if c.RefillRate, err = getEnvInt("RL_REFILL_RATE", 10); err != nil {
		return Config{}, err
	}

	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	if c.Scheme != "token_bucket" {
		return fmt.Errorf("unsupported RL_SCHEME %q (only token_bucket implemented)", c.Scheme)
	}
	if c.FailMode != FailOpen && c.FailMode != FailClosed {
		return fmt.Errorf("invalid RL_FAIL_MODE %q (want open|closed)", c.FailMode)
	}
	if c.Capacity <= 0 {
		return fmt.Errorf("RL_BUCKET_CAPACITY must be > 0, got %d", c.Capacity)
	}
	if c.RefillRate <= 0 {
		return fmt.Errorf("RL_REFILL_RATE must be > 0, got %d", c.RefillRate)
	}
	if c.RedisAddr == "" {
		return fmt.Errorf("REDIS_ADDR must not be empty")
	}
	return nil
}

// getEnv returns the env value for key, or def when the var is unset or empty.
func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// getEnvInt parses an integer env var, returning def when unset or empty.
func getEnvInt(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}
