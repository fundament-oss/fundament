package main

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugins/internal/catalogseedtest"
)

func TestListingNamesInstalledVersions(t *testing.T) {
	field, ok := reflect.TypeFor[pluginConfig]().FieldByName("EnvoyGatewayVersion")
	require.True(t, ok)
	assert.Equal(t, []string{"Envoy Gateway " + field.Tag.Get("envDefault")}, catalogseedtest.InstalledVersions(t, "gateway-api-envoy"))
}
