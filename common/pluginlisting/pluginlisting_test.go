package pluginlisting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func TestFromDefinition(t *testing.T) {
	def := &pluginruntime.PluginDefinition{}
	def.Metadata.Name = "cert-manager"
	def.Metadata.DisplayName = "Cert Manager"
	def.Metadata.Description = "Issues certificates."
	def.Metadata.License = "Apache-2.0"
	def.Metadata.URLs.Repository = "https://example.com/repo"
	def.Metadata.Tags = []string{"tls", "security"}

	assert.Equal(t, Fields{
		DisplayName:      "Cert Manager",
		DescriptionShort: "Issues certificates.",
		RepositoryURL:    "https://example.com/repo",
		License:          "Apache-2.0",
		Tags:             []string{"tls", "security"},
	}, FromDefinition(def))
}

func TestFromDefinitionFallbacksAndLimits(t *testing.T) {
	def := &pluginruntime.PluginDefinition{}
	def.Metadata.Name = "cert-manager"
	def.Metadata.Description = strings.Repeat("é", MaxDescriptionShort+10)
	for range MaxTags + 5 {
		def.Metadata.Tags = append(def.Metadata.Tags, "t")
	}

	got := FromDefinition(def)

	assert.Equal(t, "cert-manager", got.DisplayName)
	assert.Equal(t, strings.Repeat("é", MaxDescriptionShort), got.DescriptionShort)
	assert.Len(t, got.Tags, MaxTags)
}

func TestValidate(t *testing.T) {
	ok := Fields{DisplayName: strings.Repeat("a", MaxDisplayName), License: strings.Repeat("a", MaxLicense)}
	require.NoError(t, ok.Validate())

	long := Fields{DisplayName: strings.Repeat("a", MaxDisplayName+1)}
	require.ErrorContains(t, long.Validate(), "displayName")

	long = Fields{License: strings.Repeat("a", MaxLicense+1)}
	require.ErrorContains(t, long.Validate(), "license")

	relative := Fields{RepositoryURL: "example.com/repo"}
	require.ErrorContains(t, relative.Validate(), "urls.repository")
}

func TestIsNewest(t *testing.T) {
	tests := []struct {
		name    string
		version string
		others  []string
		want    bool
	}{
		{"first version", "1.0.0", nil, true},
		{"newer than all", "1.2.0", []string{"1.0.0", "1.1.9"}, true},
		{"fix on an older line", "1.0.1", []string{"2.0.0"}, false},
		{"equal is newest", "1.0.0", []string{"1.0.0"}, true},
		{"release beats its prerelease", "2.0.0", []string{"2.0.0-rc.1"}, true},
		{"prerelease below its release", "2.0.0-rc.1", []string{"2.0.0"}, false},
		{"v prefix tolerated", "v1.1.0", []string{"1.0.0"}, true},
		{"numeric not lexical", "1.10.0", []string{"1.9.0"}, true},
		{"invalid sorts lowest", "garbage", []string{"0.0.1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNewest(tt.version, tt.others))
		})
	}
}
