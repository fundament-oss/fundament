package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Pins rookCRDNames to the kinds the reconcilers watch: a watched CRD not
// waited on at install can fail the cache sync and take the manager down.
func TestRookCRDNamesCoverWatchedKinds(t *testing.T) {
	t.Parallel()
	for _, plural := range []string{"cephclusters", "cephblockpools", "cephfilesystems", "cephobjectstores"} {
		assert.Contains(t, rookCRDNames, plural+".ceph.rook.io")
	}
}
