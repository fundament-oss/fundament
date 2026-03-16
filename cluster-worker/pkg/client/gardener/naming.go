package gardener

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

// Naming constants for Gardener resources.
// These are fixed-length to ensure consistent sizing:
// - Project: 6 chars (sanitized org) + 4 chars (hash) = 10 chars
// - Shoot: 8 chars (sanitized cluster) + 3 chars (random) = 11 chars
// - Combined: 10 + 11 = 21 chars (exactly at Gardener limit)
const (
	// ProjectNamePrefixLength is the number of chars from the sanitized org name.
	ProjectNamePrefixLength = 6
	// ProjectNameHashLength is the number of hash chars appended to project name.
	ProjectNameHashLength = 4
	// ProjectNameTotalLength is the fixed total length of project names.
	ProjectNameTotalLength = 10 // 6 + 4

	// ShootNamePrefixLength is the number of chars from the sanitized cluster name.
	ShootNamePrefixLength = 8
	// ShootNameHashLength is the number of hash chars appended to shoot name.
	ShootNameHashLength = 3
	// ShootNameTotalLength is the fixed total length of shoot names.
	ShootNameTotalLength = 11 // 8 + 3

	// MaxCombinedLength is the Gardener-enforced limit for project + shoot names.
	MaxCombinedLength = 21 // 10 + 11 = 21
)

// sanitizeName converts a name to a valid Kubernetes name (lowercase alphanumeric).
// It removes all non-alphanumeric characters and ensures the name starts with a letter.
func sanitizeName(name string) string {
	// Lowercase
	name = strings.ToLower(name)

	// Remove all non-alphanumeric characters
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		}
	}
	s := result.String()

	// Ensure starts with a letter (K8s naming requirement)
	if s != "" && s[0] >= '0' && s[0] <= '9' {
		s = "x" + s
	}

	return s
}

// ProjectName generates a deterministic project name from an organization name.
// Format: sanitize(orgName)[:6] + hash(orgName)[:4] = 10 chars (fixed)
//
// The hash is computed from the original (pre-sanitized) name, so organizations
// with names that sanitize to the same prefix but have different original names
// will produce different project names.
func ProjectName(orgName string) string {
	sanitized := sanitizeName(orgName)

	// Hash the original name (before sanitization) for the suffix
	hash := sha256.Sum256([]byte(orgName))
	hashStr := hex.EncodeToString(hash[:])

	// Truncate or pad sanitized name to prefix length
	if len(sanitized) > ProjectNamePrefixLength {
		sanitized = sanitized[:ProjectNamePrefixLength]
	}
	// Pad short/empty names with hash chars
	if len(sanitized) < ProjectNamePrefixLength {
		sanitized += hashStr[:ProjectNamePrefixLength-len(sanitized)]
	}

	// Append hash suffix
	return sanitized + hashStr[ProjectNamePrefixLength:ProjectNamePrefixLength+ProjectNameHashLength]
}

// NamespaceFromProjectName returns the Gardener namespace for a project.
// Gardener namespaces follow the pattern: garden-{project-name}
func NamespaceFromProjectName(projectName string) string {
	return "garden-" + projectName
}

// GenerateShootName generates a deterministic shoot name from a cluster name and ID.
// Format: sanitize(clusterName)[:8] + hash(clusterID)[:3] = 11 chars (fixed)
//
// The hash suffix is derived from the cluster ID, ensuring the same cluster always
// produces the same shoot name across retries and reconciles. This prevents
// duplicate shoots when label-based lookup fails.
func GenerateShootName(clusterName string, clusterID uuid.UUID) string {
	sanitized := sanitizeName(clusterName)

	// Hash the cluster ID for deterministic suffix and padding
	hash := sha256.Sum256(clusterID[:])
	hashStr := hex.EncodeToString(hash[:])

	// Truncate to prefix length
	if len(sanitized) > ShootNamePrefixLength {
		sanitized = sanitized[:ShootNamePrefixLength]
	}
	// Pad short/empty names with hash chars
	if len(sanitized) < ShootNamePrefixLength {
		sanitized += hashStr[:ShootNamePrefixLength-len(sanitized)]
	}

	return sanitized + hashStr[:ShootNameHashLength]
}
