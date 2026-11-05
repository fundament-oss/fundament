package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugins/internal/catalogseed"
)

func TestListingNamesInstalledVersions(t *testing.T) {
	field, ok := reflect.TypeFor[Config]().FieldByName("CephImage")
	require.True(t, ok)
	_, ceph, _ := strings.Cut(field.Tag.Get("envDefault"), ":")
	csi, _, _ := strings.Cut(csiImageValues["csi.cephcsi.tag"], "@")
	assert.Equal(t, []string{"Rook " + rookChartVersion, "Ceph " + ceph, "Ceph CSI " + csi}, catalogseed.InstalledVersions(t, "ceph-rook"))
}
