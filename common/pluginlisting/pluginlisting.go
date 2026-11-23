// Package pluginlisting derives the listing fields a plugin's definition.yaml
// owns. They are copied onto appstore.plugins when a version becomes the newest
// published one; every other listing field is edited through UpdatePlugin.
package pluginlisting

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/mod/semver"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// Limits mirror registry.v1.CreatePluginRequest's validation.
const (
	MaxDisplayName      = 255
	MaxDescriptionShort = 255
	MaxLicense          = 64
	MaxTags             = 20
)

// Fields are the listing columns owned by definition.yaml.
type Fields struct {
	DisplayName      string
	DescriptionShort string
	RepositoryURL    string
	License          string
	Tags             []string
}

// FromDefinition maps a parsed definition onto the listing. The one-line
// metadata.description becomes the short description; the long description
// stays with the listing.
func FromDefinition(def *pluginruntime.PluginDefinition) Fields {
	displayName := def.Metadata.DisplayName
	if displayName == "" {
		displayName = def.Metadata.Name
	}

	return Fields{
		DisplayName:      displayName,
		DescriptionShort: def.Metadata.Description,
		RepositoryURL:    def.Metadata.URLs.Repository,
		License:          def.Metadata.License,
		Tags:             def.Metadata.Tags,
	}
}

// FromManifest parses manifest and returns its listing fields.
func FromManifest(manifest []byte) (Fields, error) {
	def, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return Fields{}, fmt.Errorf("parsing plugin definition: %w", err)
	}
	return FromDefinition(&def), nil
}

// Validate rejects fields the listing cannot hold, so a version is refused at
// push rather than failing its approval later.
func (f *Fields) Validate() error {
	if n := utf8.RuneCountInString(f.DisplayName); n > MaxDisplayName {
		return fmt.Errorf("metadata.displayName is %d characters, the maximum is %d", n, MaxDisplayName)
	}
	if n := utf8.RuneCountInString(f.DescriptionShort); n > MaxDescriptionShort {
		return fmt.Errorf("metadata.description is %d characters, the maximum is %d", n, MaxDescriptionShort)
	}
	if n := utf8.RuneCountInString(f.License); n > MaxLicense {
		return fmt.Errorf("metadata.license is %d characters, the maximum is %d", n, MaxLicense)
	}
	if f.RepositoryURL != "" {
		if u, err := url.Parse(f.RepositoryURL); err != nil || !u.IsAbs() {
			return fmt.Errorf("metadata.urls.repository %q is not an absolute URL", f.RepositoryURL)
		}
	}
	if len(f.Tags) > MaxTags {
		return fmt.Errorf("metadata.tags has %d entries, the maximum is %d", len(f.Tags), MaxTags)
	}
	for _, tag := range f.Tags {
		if strings.TrimSpace(tag) == "" {
			return errors.New("metadata.tags contains a blank tag")
		}
	}
	return nil
}

// IsNewest reports whether version sorts at or above every one of others by
// semver, so approving a fix on an older version line leaves the listing alone.
// Versions are stored without the "v" prefix; one that is not valid semver
// sorts below every valid one.
func IsNewest(version string, others []string) bool {
	v := canonical(version)
	for _, other := range others {
		if semver.Compare(v, canonical(other)) < 0 {
			return false
		}
	}
	return true
}

func canonical(version string) string {
	return "v" + strings.TrimPrefix(version, "v")
}
