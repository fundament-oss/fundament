// Package kubename generates and validates names for Kubernetes (and Gardener)
// resources within their naming limits. Kubernetes resource names are DNS-1123
// labels capped at 63 chars (and Gardener imposes tighter budgets on project,
// shoot, and worker-pool names), so user-supplied names must be sanitized and
// bounded before they can be materialized.
//
// It has two layers:
//
//   - generic helpers (Bounded, HashHex) that turn an arbitrary human name into a
//     deterministic, length-bounded, collision-resistant label fragment; and
//   - the project-namespace naming contract (ValidateNamespace, GenerateNamespace):
//     org-api validates the user-supplied name before recording a tenant.namespaces
//     row, and cluster-worker derives the collision-free cluster-side name when it
//     materializes that row as a v1/Namespace on the shoot. Keeping both halves
//     here guarantees the producer and the consumer agree on the 63-char budget.
package kubename

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashHex returns the hex-encoded SHA-256 digest of data (64 chars).
func HashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Sanitize lowercases s and strips everything but [a-z0-9], the common starting
// point for deriving a valid Kubernetes name fragment from an arbitrary string.
func Sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeDNS1035 is like Sanitize but guarantees the result starts with a letter
// (prefixing "x" when it would otherwise start with a digit), as required by names
// that must be RFC-1035 labels — e.g. Gardener project, shoot, and worker names.
func SanitizeDNS1035(s string) string {
	s = Sanitize(s)
	if s != "" && s[0] >= '0' && s[0] <= '9' {
		s = "x" + s
	}
	return s
}

// Bounded builds a fixed-length name of prefixLen+hashLen chars:
//
//	sanitized truncated/padded to prefixLen, then hashLen chars of hashHex
//	starting at hashOffset.
//
// A sanitized value longer than prefixLen is truncated; a shorter one is padded
// with leading hashHex chars so the prefix is always exactly prefixLen wide.
// hashHex must be at least max(prefixLen, hashOffset+hashLen) chars long; a
// SHA-256 hex digest (64 chars, see HashHex) covers every caller. Callers that
// require a leading letter must apply that to sanitized before calling.
func Bounded(sanitized, hashHex string, prefixLen, hashLen, hashOffset int) string {
	if len(sanitized) > prefixLen {
		sanitized = sanitized[:prefixLen]
	}
	if len(sanitized) < prefixLen {
		sanitized += hashHex[:prefixLen-len(sanitized)]
	}
	return sanitized + hashHex[hashOffset:hashOffset+hashLen]
}
