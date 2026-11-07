package cnpg

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/plugins/internal/catalogseed"
)

func TestListingNamesInstalledVersions(t *testing.T) {
	versions := catalogseed.InstalledVersions(t, "cloudnativepg")
	if assert.Len(t, versions, 1) {
		assert.Contains(t, versions[0], "(Helm chart "+Version+")")
	}
}
