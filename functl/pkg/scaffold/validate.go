package scaffold

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// dnsLabelRegex matches valid DNS label names (RFC 1123). Kept in sync with
// plugin-controller/pkg/controller/resources.go, which rejects a
// PluginInstallation whose name does not match.
var dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxPluginNameLen caps the plugin name so the namespace and other child
// resources the controller derives from it -- all prefixed with "plugin-" --
// stay within Kubernetes' 63-character DNS-label limit. Kept in sync with
// maxInstallationNameLen in plugin-controller/pkg/controller/resources.go.
const maxPluginNameLen = 56

// validateName checks that a plugin name can be installed: the controller
// derives the plugin's namespace from it, so it must be a short DNS label.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name must not be empty")
	}
	if len(name) > maxPluginNameLen {
		return fmt.Errorf("plugin name %q exceeds maximum length of %d characters (the plugin's Kubernetes resources are prefixed with %q)", name, maxPluginNameLen, "plugin-")
	}
	if !dnsLabelRegex.MatchString(name) {
		return fmt.Errorf("plugin name %q is not a valid DNS label (must be lowercase alphanumeric or '-', and must start and end with an alphanumeric character)", name)
	}
	return nil
}

// validateModule applies a light sanity check to the Go module path. It is
// deliberately not a full module.CheckPath: `go mod tidy` reports a better
// error than we can, so this only catches the obviously wrong.
func validateModule(module string) error {
	if module == "" {
		return fmt.Errorf("module path must not be empty")
	}
	if strings.ContainsAny(module, " \t\n\"'") {
		return fmt.Errorf("module path %q must not contain whitespace or quotes", module)
	}
	if strings.HasPrefix(module, "/") || strings.HasSuffix(module, "/") {
		return fmt.Errorf("module path %q must not start or end with '/'", module)
	}
	if path.Clean(module) != module {
		return fmt.Errorf("module path %q must be a clean path (no '.', '..' or repeated '/')", module)
	}
	return nil
}

// validateCRD checks a "<plural>.<group>" CRD name, the form the console menu
// and uiHints keys use.
func validateCRD(crd string) error {
	plural, group, found := strings.Cut(crd, ".")
	if !found || plural == "" || group == "" {
		return fmt.Errorf("crd %q must be of the form <plural>.<group>, e.g. widgets.example.com", crd)
	}
	if !dnsLabelRegex.MatchString(plural) {
		return fmt.Errorf("crd %q has an invalid resource plural %q (must be a lowercase DNS label)", crd, plural)
	}
	for _, part := range strings.Split(group, ".") {
		if !dnsLabelRegex.MatchString(part) {
			return fmt.Errorf("crd %q has an invalid API group %q (must be a lowercase DNS subdomain)", crd, group)
		}
	}
	return nil
}

// validateKind checks the Go/Kubernetes Kind, which becomes both a customComponents
// key and a Go type name.
func validateKind(kind string) error {
	if kind == "" {
		return fmt.Errorf("kind must not be empty")
	}
	if !regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`).MatchString(kind) {
		return fmt.Errorf("kind %q must be UpperCamelCase, e.g. Widget", kind)
	}
	return nil
}

// validateTargetDir refuses to scatter a new project over existing files.
func validateTargetDir(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read target directory: %w", err)
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("directory %q is not empty (use --force to write into it anyway)", dir)
	}
	return nil
}
