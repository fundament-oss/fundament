package logs

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"unset falls back to the default", 0, defaultLimit},
		{"negative falls back to the default", -1, defaultLimit},
		{"in range passes through", 250, 250},
		{"at the ceiling passes through", MaxLimit, MaxLimit},
		{"above the ceiling is clamped", MaxLimit + 1, MaxLimit},
		{"int32 max is clamped", math.MaxInt32, MaxLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EffectiveLimit(tt.limit))
		})
	}
}
