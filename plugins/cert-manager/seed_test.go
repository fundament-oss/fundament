package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/plugins/internal/catalogseed"
)

func TestListingNamesInstalledVersions(t *testing.T) {
	assert.Equal(t, []string{"cert-manager " + chartVersion}, catalogseed.InstalledVersions(t, "cert-manager"))
}
