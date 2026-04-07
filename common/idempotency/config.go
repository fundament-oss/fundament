package idempotency

import "time"

// Config holds configuration for the idempotency store.
type Config struct {
	// TTL is how long idempotency keys are retained before expiry.
	TTL time.Duration
}

func (c Config) ttl() time.Duration {
	if c.TTL == 0 {
		return 24 * time.Hour
	}
	return c.TTL
}
