package gardener

// NamespaceFromProjectName returns the Gardener namespace for a project.
// Gardener namespaces follow the pattern: garden-{project-name}.
//
// Deterministic name generation for Gardener resources (project, shoot, and
// worker-pool names) lives in common/kubename.
func NamespaceFromProjectName(projectName string) string {
	return "garden-" + projectName
}
