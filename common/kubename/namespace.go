package kubename

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
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

// ValidateNamespace reports whether name is a usable namespace name in the
// given project: a valid DNS-1123 label, not a reserved/system name, and short
// enough that the cluster-side name (GenerateNamespace) is still a DNS-1123
// label. The name is materialized into a v1/Namespace on the shoot, so an
// invalid name would otherwise fail the sync indefinitely — this catches it at
// the API boundary instead.
func ValidateNamespace(projectName, name string) error {
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("invalid namespace name %q: %s", name, strings.Join(errs, "; "))
	}
	if _, ok := reservedNamespaces[name]; ok {
		return fmt.Errorf("namespace name %q is reserved", name)
	}
	if strings.HasPrefix(name, "kube-") {
		return fmt.Errorf("namespace name %q uses the reserved \"kube-\" prefix", name)
	}
	if limit := MaxNamespaceNameLength(projectName); len(name) > limit {
		return fmt.Errorf("namespace name %q is too long for project %q: %d chars, max %d (project name + namespace name may be at most %d)",
			name, projectName, len(name), limit, maxCombinedLength)
	}
	return nil
}

const (
	// TenantNamespacePrefix starts every namespace fundament creates for a
	// project. Nothing else on a cluster may use it, so a project namespace can
	// never take the name of a system or plugin namespace (a project "cert" with
	// a namespace "manager" must not become cert-manager's namespace).
	TenantNamespacePrefix = "tnt-"
	// NamespaceSeparator joins the project and namespace names. New project
	// names may not contain it, so different pairs never produce the same name
	// ("a-b"+"c" and "a"+"b-c" give tnt-a-b--c and tnt-a--b-c).
	NamespaceSeparator = "--"

	// maxCombinedLength is what a DNS-1123 label leaves for the project and
	// namespace names together.
	maxCombinedLength = validation.DNS1123LabelMaxLength - len(TenantNamespacePrefix) - len(NamespaceSeparator)
)

// MaxNamespaceNameLength is the longest namespace name a project can hold: the
// cluster-side name (GenerateNamespace) must fit a DNS-1123 label (63 chars).
func MaxNamespaceNameLength(projectName string) int {
	return maxCombinedLength - len(projectName)
}

// GenerateNamespace derives the cluster-side name for a fundament namespace:
//
//	tnt-<projectName>--<name>
//
// The same scheme as Gardener's seed namespaces (shoot--<project>--<shoot>):
// a fixed prefix separates tenant namespaces from everything else on the
// cluster, and a separator that project names may not contain keeps them
// apart from each other. Project names are unique per cluster and immutable, so
// the name is readable in kubectl and GUI tools and stable for the namespace's
// life. Projects created before the separator was reserved may contain "--";
// for those org-api still refuses a namespace whose generated name is already
// in use on its cluster, and cluster-worker never adopts a namespace it doesn't
// own. name is assumed to have passed ValidateNamespace.
//
// Namespaces created before this scheme keep their name; the cluster-worker
// finds them by their fundament.io/namespace-id label.
func GenerateNamespace(projectName, name string) string {
	return TenantNamespacePrefix + projectName + NamespaceSeparator + name
}
