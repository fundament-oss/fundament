package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/plugins/internal/catalogseedtest"
)

func TestListingNamesInstalledVersions(t *testing.T) {
	assert.Equal(t, []string{"cert-manager " + chartVersion}, catalogseedtest.InstalledVersions(t, "cert-manager"))
}
