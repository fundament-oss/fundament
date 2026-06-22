package kubename

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	// ProjectPrefixLen is the number of sanitized project-name chars that lead
	// the cluster-side namespace name.
	ProjectPrefixLen = 8
	// ProjectHashLen is the number of project-id hash chars appended to the
	// project prefix to keep it collision-free.
	ProjectHashLen = 4

	// namespacePrefixLen is the fixed cost the project prefix and its separator add
	// to the cluster-side name: sanitized(8) + hash(4) + "-"(1).
	namespacePrefixLen = ProjectPrefixLen + ProjectHashLen + 1

	// MaxNamespaceNameLength is the longest namespace name org-api accepts. The
	// cluster-side name is "<project-prefix>-<name>" and must stay within the
	// DNS-1123 label limit, so the user-facing portion is capped at
	// limit - prefix = 63 - 13 = 50.
	MaxNamespaceNameLength = validation.DNS1123LabelMaxLength - namespacePrefixLen
)

// reservedNamespaces holds namespace names that must never be created: the
// Kubernetes system namespaces and fundament's own system namespace. Names with
// the "kube-" prefix are reserved by convention and rejected separately in
// ValidateNamespace.
var reservedNamespaces = map[string]struct{}{
	"default":          {},
	"kube-system":      {},
	"kube-public":      {},
	"kube-node-lease":  {},
	"fundament-system": {},
}

// ValidateNamespace reports whether name is a usable project-namespace name: a
// valid DNS-1123 label, within MaxNamespaceNameLength (so the prefixed
// cluster-side name still fits), and not a reserved/system name. The name is
// materialized verbatim into the cluster-side namespace, so an invalid name would
// otherwise fail the sync indefinitely — this catches it at the API boundary
// instead.
func ValidateNamespace(name string) error {
	if len(name) > MaxNamespaceNameLength {
		return fmt.Errorf("namespace name %q is too long: %d chars, max %d", name, len(name), MaxNamespaceNameLength)
	}
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("invalid namespace name %q: %s", name, strings.Join(errs, "; "))
	}
	if _, ok := reservedNamespaces[name]; ok {
		return fmt.Errorf("namespace name %q is reserved", name)
	}
	if strings.HasPrefix(name, "kube-") {
		return fmt.Errorf("namespace name %q uses the reserved \"kube-\" prefix", name)
	}
	return nil
}

// GenerateNamespace derives the deterministic, collision-free cluster-side
// namespace name for a fundament namespace:
//
//	<sanitize(projectName)[:8] + hash(projectID)[:4]>-<name>
//
// The project-id hash keeps the names of two projects on the same cluster
// distinct even when their names sanitize identically, which is what makes a
// shared shoot safe. name is assumed to have passed ValidateNamespace, so the
// result is a valid DNS-1123 label of at most 63 chars. The name is stable
// because it derives only from immutable inputs (project id, project name, and
// the namespace name, none of which can change).
func GenerateNamespace(projectName string, projectID uuid.UUID, name string) string {
	// The project-id hash leads the suffix (offset = ProjectPrefixLen) and pads
	// short/empty project names so the prefix is always fixed-width. The project
	// prefix may start with a digit — the cluster-side name is "<prefix>-<name>"
	// and a leading digit is a valid DNS-1123 label start — so Sanitize (not
	// SanitizeDNS1035) is used here.
	prefix := Bounded(Sanitize(projectName), HashHex(projectID[:]),
		ProjectPrefixLen, ProjectHashLen, ProjectPrefixLen)
	return prefix + "-" + name
}
