package gardener

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
	// ShootNameRandomLength is the number of random chars appended to shoot name.
	ShootNameRandomLength = 3
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
// (e.g., "Acme Corp" vs "Acme-Corp") will produce different project names.
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

// GenerateShootName generates a shoot name with a random suffix.
// Format: sanitize(clusterName)[:8] + random(3) = 11 chars (fixed)
//
// The random suffix ensures uniqueness within a project, even for clusters
// with names that sanitize to the same prefix (e.g., "prod" and "production"
// both truncate to "producti").
func GenerateShootName(clusterName string) string {
	sanitized := sanitizeName(clusterName)

	// Truncate to prefix length
	if len(sanitized) > ShootNamePrefixLength {
		sanitized = sanitized[:ShootNamePrefixLength]
	}
	// Pad short/empty names with random chars
	if len(sanitized) < ShootNamePrefixLength {
		sanitized += randomAlphanumeric(ShootNamePrefixLength - len(sanitized))
	}

	return sanitized + randomAlphanumeric(ShootNameRandomLength)
}

// randomAlphanumeric generates n random lowercase alphanumeric characters.
// Uses crypto/rand for cryptographically secure randomness.
func randomAlphanumeric(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read should never fail on a properly configured system.
		// If it does, there's a serious system issue and we should panic.
		panic("crypto/rand failed: " + err.Error())
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}
