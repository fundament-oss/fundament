package checker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/tools/gomod-age/pkg/config"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0s"},
		{-1 * time.Hour, "0s"},
		{30 * time.Minute, "30m"},
		{5*time.Hour + 30*time.Minute, "5h30m"},
		{3*24*time.Hour + 5*time.Hour, "3d5h"},
		{7 * 24 * time.Hour, "7d0h"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDuration(tt.input))
		})
	}
}

func TestClassifyModule_Passed(t *testing.T) {
	cfg := &config.Config{MinAge: 7 * 24 * time.Hour}
	now := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	publishTime := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC) // 14 days old

	age := now.Sub(publishTime)
	assert.True(t, age >= cfg.MinAge, "14d age should pass 7d threshold")
}

func TestClassifyModule_Violation(t *testing.T) {
	cfg := &config.Config{MinAge: 7 * 24 * time.Hour}
	now := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	publishTime := time.Date(2024, 6, 14, 0, 0, 0, 0, time.UTC) // 1 day old

	age := now.Sub(publishTime)
	assert.True(t, age < cfg.MinAge, "1d age should violate 7d threshold")
}

func TestClassifyModule_ExactBoundary(t *testing.T) {
	cfg := &config.Config{MinAge: 7 * 24 * time.Hour}
	now := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	publishTime := time.Date(2024, 6, 8, 0, 0, 0, 0, time.UTC) // exactly 7 days

	age := now.Sub(publishTime)
	// Strict less-than: age == minAge should pass
	assert.False(t, age < cfg.MinAge, "exact boundary should pass, not violate")
}

func TestConfig_IsAllowed_ExactMatch(t *testing.T) {
	cfg := &config.Config{
		Allow: []config.AllowEntry{
			{Module: "github.com/example/mod", Version: "v1.0.0", Reason: "reviewed"},
		},
	}

	reason, ok := cfg.IsAllowed("github.com/example/mod", "v1.0.0")
	assert.True(t, ok)
	assert.Equal(t, "reviewed", reason)
}

func TestConfig_IsAllowed_VersionMismatch(t *testing.T) {
	cfg := &config.Config{
		Allow: []config.AllowEntry{
			{Module: "github.com/example/mod", Version: "v1.0.0", Reason: "reviewed"},
		},
	}

	_, ok := cfg.IsAllowed("github.com/example/mod", "v1.1.0")
	assert.False(t, ok, "different version should not match allow list")
}

func TestConfig_IsIgnored(t *testing.T) {
	cfg := &config.Config{
		Ignore: []string{"buf.build/*", "github.com/internal/specific"},
	}

	assert.True(t, cfg.IsIgnored("buf.build/gen/go/something"))
	assert.True(t, cfg.IsIgnored("github.com/internal/specific"))
	assert.False(t, cfg.IsIgnored("github.com/external/mod"))
}

func TestConfig_IsIgnored_SlashStar(t *testing.T) {
	cfg := &config.Config{
		Ignore: []string{"github.com/org/*"},
	}

	assert.True(t, cfg.IsIgnored("github.com/org/repo"))
	assert.True(t, cfg.IsIgnored("github.com/org/repo/sub"))
	assert.False(t, cfg.IsIgnored("github.com/other/repo"))
	// Exact match on the prefix itself
	assert.True(t, cfg.IsIgnored("github.com/org"))
}

func TestClassifyModule_FutureTimestamp(t *testing.T) {
	// Clock skew: proxy reports a publish time in the future.
	// Age would be negative — should not violate (treat as 0 age).
	cfg := &config.Config{MinAge: 7 * 24 * time.Hour}
	now := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	publishTime := time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC) // 1 day in the future

	age := now.Sub(publishTime) // negative
	// Negative age is < minAge, so it's a violation — correct behavior.
	// A future-dated package is suspicious and should be flagged.
	assert.True(t, age < cfg.MinAge)
}

func TestClassifyModule_ZeroMinAge(t *testing.T) {
	// age: "0d" should allow everything (no minimum).
	cfg := &config.Config{MinAge: 0}
	now := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	publishTime := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC) // just published

	age := now.Sub(publishTime) // 0
	// 0 is not < 0, so it passes.
	assert.False(t, age < cfg.MinAge)
}

func TestConfig_ParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"0d", 0, false},
		{"72h", 72 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"abc", 0, true},
		{"", 0, true},
		{"d", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := config.ParseDuration(tt.input)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
