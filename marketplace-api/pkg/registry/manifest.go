package registry

import (
	"fmt"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// parsedManifest is what the server reads out of the pushed bytes. The image is
// never sent separately, so it cannot drift from the manifest that gets hashed.
type parsedManifest struct {
	name    string
	version string
	image   string
	hash    string
}

// parseManifest validates the pushed bytes and derives everything the server
// stores about them. pluginruntime.ParseDefinition and HashManifest are reused
// rather than reimplemented: the plugin-controller recomputes the same hash over
// the same bytes at reconcile time to verify the install's consent pin
// (FUN-17), so a second implementation is a second chance for them to disagree.
func parseManifest(manifest []byte) (parsedManifest, error) {
	definition, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return parsedManifest{}, err
	}

	return parsedManifest{
		name:    definition.Metadata.Name,
		version: definition.Metadata.Version,
		image:   definition.Spec.Image,
		hash:    pluginruntime.HashManifest(manifest),
	}, nil
}

// checkManifestMatches rejects a manifest describing a different plugin or
// version than the request claims. Without it the stored row and the bytes it
// pins could disagree about what was actually published.
func (m parsedManifest) checkMatches(pluginName, requestedVersion string) error {
	if m.name != pluginName {
		return fmt.Errorf("manifest metadata.name %q does not match plugin %q", m.name, pluginName)
	}

	if normalizeVersion(m.version) != normalizeVersion(requestedVersion) {
		return fmt.Errorf("manifest metadata.version %q does not match requested version %q", m.version, requestedVersion)
	}

	return nil
}

// imageFor reports the container image recorded in a stored manifest. A version
// whose bytes no longer parse still lists, with an empty image, rather than
// failing the whole call.
func imageFor(manifest []byte) string {
	definition, err := pluginruntime.ParseDefinition(manifest)
	if err != nil {
		return ""
	}
	return definition.Spec.Image
}
