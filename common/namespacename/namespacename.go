// Package namespacename is the single source of truth for the naming contract of
// project namespaces: org-api validates the user-supplied name (Validate) before
// recording a tenant.namespaces row, and cluster-worker derives the collision-free
// cluster-side name (Generate) when it materializes that row as a v1/Namespace on
// the shoot. Keeping both halves here guarantees the producer and the consumer
// agree on the 63-char DNS-1123 budget.
package namespacename

import (
	"crypto/sha256"
	"encoding/hex"
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

	// prefixLen is the fixed cost the project prefix and its separator add to the
	// cluster-side name: sanitized(8) + hash(4) + "-"(1).
	prefixLen = ProjectPrefixLen + ProjectHashLen + 1

	// MaxNameLength is the longest namespace name org-api accepts. The cluster-side
	// name is "<project-prefix>-<name>" and must stay within the DNS-1123 label
	// limit, so the user-facing portion is capped at limit - prefix = 63 - 13 = 50.
	MaxNameLength = validation.DNS1123LabelMaxLength - prefixLen
)

// reserved holds namespace names that must never be created: the Kubernetes
// system namespaces and fundament's own system namespace. Names with the "kube-"
// prefix are reserved by convention and rejected separately in Validate.
var reserved = map[string]struct{}{
	"default":          {},
	"kube-system":      {},
	"kube-public":      {},
	"kube-node-lease":  {},
	"fundament-system": {},
}

// Validate reports whether name is a usable project-namespace name: a valid
// DNS-1123 label, within MaxNameLength (so the prefixed cluster-side name still
// fits), and not a reserved/system name. The name is materialized verbatim into
// the cluster-side namespace, so an invalid name would otherwise fail the sync
// indefinitely — this catches it at the API boundary instead.
func Validate(name string) error {
	if len(name) > MaxNameLength {
		return fmt.Errorf("namespace name %q is too long: %d chars, max %d", name, len(name), MaxNameLength)
	}
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("invalid namespace name %q: %s", name, strings.Join(errs, "; "))
	}
	if _, ok := reserved[name]; ok {
		return fmt.Errorf("namespace name %q is reserved", name)
	}
	if strings.HasPrefix(name, "kube-") {
		return fmt.Errorf("namespace name %q uses the reserved \"kube-\" prefix", name)
	}
	return nil
}

// Generate derives the deterministic, collision-free cluster-side namespace name
// for a fundament namespace:
//
//	<sanitize(projectName)[:8] + hash(projectID)[:4]>-<name>
//
// The project-id hash keeps the names of two projects on the same cluster
// distinct even when their names sanitize identically, which is what makes a
// shared shoot safe. name is assumed to have passed Validate, so the result is a
// valid DNS-1123 label of at most 63 chars. The name is stable across reconciles
// because it derives only from immutable inputs (the project id) and the row's
// current name; renames are handled label-side, not by renaming the resource.
func Generate(projectName string, projectID uuid.UUID, name string) string {
	sanitized := sanitize(projectName)

	hash := sha256.Sum256(projectID[:])
	hashStr := hex.EncodeToString(hash[:])

	if len(sanitized) > ProjectPrefixLen {
		sanitized = sanitized[:ProjectPrefixLen]
	}
	// Pad short/empty sanitized names with hash chars so the prefix is fixed-width.
	if len(sanitized) < ProjectPrefixLen {
		sanitized += hashStr[:ProjectPrefixLen-len(sanitized)]
	}

	prefix := sanitized + hashStr[ProjectPrefixLen:ProjectPrefixLen+ProjectHashLen]
	return prefix + "-" + name
}

// sanitize lowercases and strips everything but [a-z0-9], matching the convention
// used for other generated Gardener names.
func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
