package cluster_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler/cluster"
)

// A limit of zero or less would report the stalled state on the first check,
// so the worker refuses to start with one.
func TestConfigValidate(t *testing.T) {
	t.Parallel()

	valid := cluster.Config{
		StatusPendingTimeout:  20 * time.Minute,
		StatusUpdatingWarning: 30 * time.Minute,
		StatusDeletingAlert:   2 * time.Hour,
	}
	require.NoError(t, valid.Validate())

	zeroPending := valid
	zeroPending.StatusPendingTimeout = 0
	assert.EqualError(t, zeroPending.Validate(), "CLUSTER_STATUS_PENDING_TIMEOUT must be positive, got 0s")

	negativeUpdating := valid
	negativeUpdating.StatusUpdatingWarning = -time.Minute
	assert.EqualError(t, negativeUpdating.Validate(), "CLUSTER_STATUS_UPDATING_WARNING must be positive, got -1m0s")

	zeroDeleting := valid
	zeroDeleting.StatusDeletingAlert = 0
	assert.EqualError(t, zeroDeleting.Validate(), "CLUSTER_STATUS_DELETING_ALERT must be positive, got 0s")
}
