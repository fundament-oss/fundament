package gardener

// LabelPrefix is the domain prefix for all Fundament labels.
// TODO: Update when final domain is determined.
const LabelPrefix = "fundament.io"

// Label and annotation keys for Fundament-managed Gardener resources.
var (
	// LabelClusterID is the label key for the Fundament cluster UUID.
	// Used as the primary identifier for looking up Shoots.
	LabelClusterID = LabelPrefix + "/cluster-id"

	// LabelOrganizationID is the label key for the organization UUID.
	// Used for filtering and organization-level queries.
	LabelOrganizationID = LabelPrefix + "/organization-id"

	// AnnotationClusterName is the annotation key for the original cluster name.
	// Stored as annotation (not label) since it may change and is for reference only.
	AnnotationClusterName = LabelPrefix + "/cluster-name"
)
