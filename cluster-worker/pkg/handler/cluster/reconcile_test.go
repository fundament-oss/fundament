package cluster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// Two shoots with one cluster id are ordered by creation, then uid, so the
// same one is picked whatever order Gardener lists them in.
func TestOlderShoot(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := gardener.ShootInfo{UID: "b", Created: created}
	newer := gardener.ShootInfo{UID: "a", Created: created.Add(time.Minute)}
	assert.True(t, olderShoot(older, newer))
	assert.False(t, olderShoot(newer, older))

	sameA := gardener.ShootInfo{UID: "a", Created: created}
	sameB := gardener.ShootInfo{UID: "b", Created: created}
	assert.True(t, olderShoot(sameA, sameB))
	assert.False(t, olderShoot(sameB, sameA))
	assert.False(t, olderShoot(sameA, sameA))
}
