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

func TestClassify(t *testing.T) {
	const week = 7 * 24 * time.Hour
	tests := []struct {
		name   string
		age    time.Duration
		minAge time.Duration
		want   ResultKind
	}{
		{"older than minimum", 14 * 24 * time.Hour, week, KindPassed},
		{"younger than minimum", 24 * time.Hour, week, KindViolation},
		{"exact boundary passes", week, week, KindPassed},
		// Clock skew / antedating: a publish time in the future yields a
		// negative age and must be flagged as a violation, not silently
		// treated as zero.
		{"future-dated is a violation", -24 * time.Hour, week, KindViolation},
		{"zero minimum allows just-published", 0, 0, KindPassed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Classify(tt.age, tt.minAge))
		})
	}
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
