package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The heartbeat stores the interval in whole seconds and requires at least
// one, so a sub-second interval is rejected along with a non-positive one.
func TestConfigValidate(t *testing.T) {
	valid := Config{Interval: 15 * time.Minute, Workers: 4, Timeout: 30 * time.Second}
	assert.NoError(t, valid.Validate())

	for _, interval := range []time.Duration{0, -time.Second, 500 * time.Millisecond} {
		cfg := valid
		cfg.Interval = interval
		assert.Error(t, cfg.Validate(), "interval %s", interval)
	}

	cfg := valid
	cfg.Timeout = 0
	assert.Error(t, cfg.Validate())
}
