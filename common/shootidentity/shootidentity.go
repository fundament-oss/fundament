// Package shootidentity names the fundament-managed identities on every shoot
// that more than one component depends on: cluster-worker provisions them,
// kube-api-proxy authenticates and impersonates with them.
package shootidentity

const (
	// Namespace holds the fundament ServiceAccounts on every shoot.
	Namespace = "fundament-system"

	// ProxyServiceAccount is kube-api-proxy's own identity on a shoot. Its only
	// rights are to impersonate ServiceAccounts in Namespace and the
	// NamespaceListerGroup. RBAC can't match a name prefix, so that covers
	// every ServiceAccount in Namespace, including org admins' cluster-admin
	// ones; it grants the proxy nothing new, since it already holds the
	// shoot's admin kubeconfig.
	ProxyServiceAccount = "fundament-kube-api-proxy"

	// NamespaceListerGroup is added by kube-api-proxy when it serves a user's
	// namespace listing; it grants get/list/watch on namespaces only.
	NamespaceListerGroup = "fundament:namespace-listers"

	// NamespaceListerClusterRole is the ClusterRole bound to NamespaceListerGroup.
	NamespaceListerClusterRole = "fundament:namespace-lister"

	// LabelProjectID is the label cluster-worker puts on every tenant
	// namespace naming its project; kube-api-proxy filters listings on it,
	// so only cluster-worker may write it.
	LabelProjectID = "fundament.io/project-id"
)

// UserServiceAccountUsername is the apiserver username of a user's per-cluster
// ServiceAccount fundament-{userID} in Namespace.
func UserServiceAccountUsername(userID string) string {
	return "system:serviceaccount:" + Namespace + ":fundament-" + userID
}
