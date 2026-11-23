package outbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/dbconst"
)

func TestMachineEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		from    dbconst.ClusterOutboxStatus
		event   Event
		want    dbconst.ClusterOutboxStatus
		allowed bool
	}{
		{name: "pending completes", from: dbconst.ClusterOutboxStatus_Pending, event: EventCompleted, want: dbconst.ClusterOutboxStatus_Completed, allowed: true},
		{name: "pending retries", from: dbconst.ClusterOutboxStatus_Pending, event: EventRetried, want: dbconst.ClusterOutboxStatus_Retrying, allowed: true},
		{name: "pending fails", from: dbconst.ClusterOutboxStatus_Pending, event: EventFailed, want: dbconst.ClusterOutboxStatus_Failed, allowed: true},
		{name: "retrying completes", from: dbconst.ClusterOutboxStatus_Retrying, event: EventCompleted, want: dbconst.ClusterOutboxStatus_Completed, allowed: true},
		{name: "retrying stays retrying", from: dbconst.ClusterOutboxStatus_Retrying, event: EventRetried, want: dbconst.ClusterOutboxStatus_Retrying, allowed: true},
		{name: "retrying fails", from: dbconst.ClusterOutboxStatus_Retrying, event: EventFailed, want: dbconst.ClusterOutboxStatus_Failed, allowed: true},
		{name: "completed is terminal", from: dbconst.ClusterOutboxStatus_Completed, event: EventFailed, allowed: false},
		{name: "failed is terminal", from: dbconst.ClusterOutboxStatus_Failed, event: EventCompleted, allowed: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			to, ok := Machine.Apply(tc.from, tc.event)

			require.Equal(t, tc.allowed, ok)
			if tc.allowed {
				assert.Equal(t, tc.want, to)
			}
		})
	}
}

func TestMachineMermaid(t *testing.T) {
	t.Parallel()

	got := Machine.Mermaid()

	assert.Contains(t, got, `[*] --> s0`)
	assert.Contains(t, got, `s0 --> s1 : completed`)
	assert.Contains(t, got, `s2 --> s2 : retried`)
	assert.Contains(t, got, `s2 --> s3 : failed`)
	assert.Contains(t, got, `s1 --> [*]`)
	assert.Contains(t, got, `s3 --> [*]`)
}
