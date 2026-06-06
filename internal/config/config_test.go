package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	// Force every relevant var to present-but-empty so Load() uses defaults
	// regardless of the host environment. getEnv/getEnvInt treat "" as unset.
	for _, k := range []string{
		"RL_PORT", "REDIS_ADDR", "RL_SCHEME", "RL_FAIL_MODE",
		"RL_BUCKET_CAPACITY", "RL_REFILL_RATE", "RL_KEY_PREFIX",
		"REDIS_DB", "REDIS_POOL_SIZE", "REDIS_DIAL_TIMEOUT_MS", "REDIS_READ_TIMEOUT_MS",
	} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Capacity != 100 || cfg.RefillRate != 10 {
		t.Errorf("defaults: capacity=%d refill=%d, want 100/10", cfg.Capacity, cfg.RefillRate)
	}
	if cfg.FailMode != FailClosed {
		t.Errorf("default fail mode = %q, want closed", cfg.FailMode)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("default RedisAddr = %q, want localhost:6379", cfg.RedisAddr)
	}
}

func TestLoadInvalidScheme(t *testing.T) {
	t.Setenv("RL_SCHEME", "sliding_window") // not implemented this iteration
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unimplemented scheme, got nil")
	}
}

func TestLoadInvalidFailMode(t *testing.T) {
	t.Setenv("RL_FAIL_MODE", "maybe")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid fail mode, got nil")
	}
}

func TestLoadNonIntCapacity(t *testing.T) {
	t.Setenv("RL_BUCKET_CAPACITY", "lots")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-integer capacity, got nil")
	}
}

func TestLoadNonPositiveRefill(t *testing.T) {
	t.Setenv("RL_REFILL_RATE", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-positive refill, got nil")
	}
}
