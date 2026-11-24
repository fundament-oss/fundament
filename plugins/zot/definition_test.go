package main

import (
	"os"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestDefinition(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err)
	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err)

	// The Console's installation name and the image name depend on it.
	assert.Equal(t, "zot", def.Metadata.Name)

	_, err = os.Stat(def.Metadata.Icon)
	require.NoError(t, err, "metadata.icon must name a file next to definition.yaml")

	// The sidebar item is the CRD that the plugin applies from crds/.
	consoleCRD, err := os.ReadFile("crds/zot.fundament.io_registries.yaml")
	require.NoError(t, err)
	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(consoleCRD, &crd))
	assert.Equal(t, consoleCRDName, crd.Name)
	assert.Equal(t, apiextensionsv1.ClusterScoped, crd.Spec.Scope, "the organization sidebar shows only cluster-scoped CRDs")
	assert.Equal(t, registryGVK.Kind, crd.Spec.Names.Kind)
	require.Len(t, crd.Spec.Versions, 1)
	assert.Equal(t, registryGVK.Version, crd.Spec.Versions[0].Name)

	assert.Equal(t, []string{consoleCRDName}, def.Spec.CRDs)
	assert.Empty(t, def.Spec.Menu.Project)
	require.Len(t, def.Spec.Menu.Organization, 1)
	entry := def.Spec.Menu.Organization[0]
	assert.Equal(t, consoleCRDName, entry.CRD)
	_, err = os.Stat(entry.Icon)
	require.NoError(t, err, "the menu icon must name a file next to definition.yaml")

	require.Len(t, def.Spec.AllowedResources, 1)
	assert.Equal(t, registryGVK.Group, def.Spec.AllowedResources[0].Group)
	assert.Equal(t, registryGVK.Version, def.Spec.AllowedResources[0].Version)
	assert.Equal(t, crd.Spec.Names.Plural, def.Spec.AllowedResources[0].Resource)
	assert.ElementsMatch(t, []string{"list", "get"}, def.Spec.AllowedResources[0].Verbs)
}

// The registry runs these checks when functl publishes the icons: an SVG that
// fails them would only show up at publish time.
func TestIconsPassThePublishChecks(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("definition.yaml")
	require.NoError(t, err)
	def, err := pluginruntime.ParseSourceDefinition(src)
	require.NoError(t, err)

	files := map[string][]byte{}
	resolve := func(path string) string {
		content, err := os.ReadFile(path) //nolint:gosec // paths from the plugin's own definition.yaml
		require.NoError(t, err)
		digest := pluginruntime.IconDigest(content)
		files[digest] = content
		return digest
	}
	def.Metadata.Icon = resolve(def.Metadata.Icon)
	for i := range def.Spec.Menu.Organization {
		def.Spec.Menu.Organization[i].Icon = resolve(def.Spec.Menu.Organization[i].Icon)
	}

	_, err = pluginruntime.ValidateIcons(&def, files)
	require.NoError(t, err)
}
