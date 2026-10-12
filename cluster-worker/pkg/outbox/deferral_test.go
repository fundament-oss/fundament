package outbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

func TestDeferralDelay(t *testing.T) {
	t.Parallel()

	const defaultDelay = 30 * time.Second

	tests := []struct {
		name           string
		deferralsSoFar int32
		precondErr     *handler.PreconditionError
		want           time.Duration
	}{
		{name: "plain precondition", precondErr: handler.NewPreconditionError("parent cluster not synced to Gardener"), want: defaultDelay},
		{name: "first deferral uses the shorter recheck", precondErr: handler.NewPreconditionErrorWithFirstRetry("project namespace not ready", 5*time.Second), want: 5 * time.Second},
		{name: "later deferrals use the default", deferralsSoFar: 1, precondErr: handler.NewPreconditionErrorWithFirstRetry("project namespace not ready", 5*time.Second), want: defaultDelay},
		{name: "a longer first recheck never extends the default", precondErr: handler.NewPreconditionErrorWithFirstRetry("slow", time.Minute), want: defaultDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, deferralDelay(defaultDelay, tt.deferralsSoFar, tt.precondErr))
		})
	}
}
