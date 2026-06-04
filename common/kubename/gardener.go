package kubename

import "github.com/google/uuid"

// Naming constants for Gardener resources.
// These are fixed-length to ensure consistent sizing:
// - Project: 6 chars (sanitized org) + 4 chars (hash) = 10 chars
// - Shoot: 8 chars (sanitized cluster) + 3 chars (hash) = 11 chars
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

	// WorkerNamePrefixLength is the number of chars from the sanitized pool name.
	WorkerNamePrefixLength = 4
	// WorkerNameHashLength is the number of hash chars appended to worker name.
	WorkerNameHashLength = 4
	// WorkerNameTotalLength is the fixed total length of generated worker names.
	//
	// Worker pool names propagate into Gardener machine names, and on the local
	// provider into a per-machine Service named "machine-<machineName>". Service
	// names are DNS-1035 labels capped at 63 chars. The machine name is derived as:
	//
	//   <technicalID>-<workerName>-z<zoneIdx>-<poolHash>-<machineSuffix>
	//
	// with technicalID = "shoot--"+project(10)+"--"+shoot(11) = 30 (fixed by
	// ProjectName/GenerateShootName), poolHash = 5 chars (WorkerPoolHashV1) and
	// machineSuffix = 5 chars (Kubernetes GenerateName). Worst-case budget:
	//
	//   63 - len("machine-")=8 - 30 - len("-")=1 - len("-z99")=4
	//      - len("-XXXXX")=6 - len("-XXXXX")=6 = 8
	//
	// so generated worker names are capped at 8 chars to keep the Service valid.
	WorkerNameTotalLength = 8 // 4 + 4
)

// ProjectName generates a deterministic Gardener project name from an organization
// name. Format: sanitize(orgName)[:6] + hash(orgName)[:4] = 10 chars (fixed).
//
// The hash is computed from the original (pre-sanitized) name, so organizations
// with names that sanitize to the same prefix but have different original names
// will produce different project names.
func ProjectName(orgName string) string {
	return Bounded(SanitizeDNS1035(orgName), HashHex([]byte(orgName)),
		ProjectNamePrefixLength, ProjectNameHashLength, ProjectNamePrefixLength)
}

// GenerateWorkerName generates a deterministic, length-bounded worker pool name.
// Format: sanitize(poolName)[:4] + hash(poolName)[:4] = 8 chars (fixed).
//
// Worker pool names flow into Gardener machine names (and, on the local provider,
// into "machine-<machineName>" Services capped at 63 chars). Bounding the worker
// name to WorkerNameTotalLength guarantees the derived names stay within that
// limit regardless of the user-supplied pool name length. The hash suffix is
// derived from the original pool name so names stay unique within a cluster and
// stable across reconciles.
func GenerateWorkerName(poolName string) string {
	return Bounded(SanitizeDNS1035(poolName), HashHex([]byte(poolName)),
		WorkerNamePrefixLength, WorkerNameHashLength, WorkerNamePrefixLength)
}

// GenerateShootName generates a deterministic shoot name from a cluster name and ID.
// Format: sanitize(clusterName)[:8] + hash(clusterID)[:3] = 11 chars (fixed).
//
// The hash suffix is derived from the cluster ID, ensuring the same cluster always
// produces the same shoot name across retries and reconciles. This prevents
// duplicate shoots when label-based lookup fails. Unlike the other bounded names
// the hash suffix is taken from the start of the digest (hashOffset 0), preserving
// the original shoot-name output.
func GenerateShootName(clusterName string, clusterID uuid.UUID) string {
	return Bounded(SanitizeDNS1035(clusterName), HashHex(clusterID[:]),
		ShootNamePrefixLength, ShootNameHashLength, 0)
}
