package reconcile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The heartbeat stores the interval in whole seconds and requires at least
// one, so a sub-second interval is rejected along with a non-positive one.
func TestConfigValidate(t *testing.T) {
	assert.NoError(t, Config{Interval: 5 * time.Minute}.Validate())
	assert.NoError(t, Config{Interval: time.Second}.Validate())

	for _, interval := range []time.Duration{0, -time.Second, 500 * time.Millisecond} {
		assert.Error(t, Config{Interval: interval}.Validate(), "interval %s", interval)
	}
}
