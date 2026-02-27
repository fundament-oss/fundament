package gardener

import (
	"context"
	"fmt"
	"log/slog"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProviderConfig holds cloud provider-specific configuration.
type ProviderConfig struct {
	Type                   string // e.g., "local", "metal", "aws"
	CloudProfile           string // e.g., "local", "metal", "aws"
	CredentialsBindingName string // e.g., "local", "metal-credentials" (required for all providers)

	// CredentialsSecretRef is the reference to the shared credentials secret.
	// This is used to create CredentialsBindings in new project namespaces.
	// Format: "namespace/name" (e.g., "garden-local/local")
	CredentialsSecretRef string

	MachineImageName    string // e.g., "local", "gardenlinux"
	MachineImageVersion string // e.g., "1.0.0", "1592.2.0"
	DefaultMachineType  string // e.g., "local", "n1-standard-4" (fallback when no node pools configured)
}

// NewProviderConfig creates a ProviderConfig with defaults for the local provider.
// Override fields as needed for other providers.
func NewProviderConfig() ProviderConfig {
	return ProviderConfig{
		Type:                   "local",
		CloudProfile:           "local",
		CredentialsBindingName: "local",              // Name of CredentialsBinding to create/reference
		CredentialsSecretRef:   "garden-local/local", // Shared secret for local provider
		MachineImageName:       "local",
		MachineImageVersion:    "1.0.0",
		DefaultMachineType:     "local",
	}
}

// RealClient implements Client using the actual Gardener API.
type RealClient struct {
	client   client.Client
	provider ProviderConfig
	logger   *slog.Logger
}

// NewReal creates a new RealClient that connects to Gardener.
// If kubeconfigPath is empty, it uses in-cluster config.
func NewReal(kubeconfigPath string, provider ProviderConfig, logger *slog.Logger) (*RealClient, error) {
	// Build REST config
	var clientConfig clientcmd.ClientConfig

	if kubeconfigPath != "" {
		// Load from file
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)
	} else {
		// Use in-cluster config
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build REST config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := gardencorev1beta1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add Gardener core types to scheme: %w", err)
	}
	if err := securityv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add Gardener security types to scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	logger.Info("connected to Gardener API",
		"host", cfg.Host,
		"provider", provider.Type,
		"cloudProfile", provider.CloudProfile)

	return &RealClient{
		client:   c,
		provider: provider,
		logger:   logger,
	}, nil
}

// EnsureProject creates the Gardener Project if it doesn't exist (idempotent).
// First searches for existing project by organization ID label, then creates if not found.
// Also ensures a CredentialsBinding exists in the project namespace.
// Returns the actual namespace from project.Spec.Namespace.
// Note: The namespace is created asynchronously by Gardener.
// If not ready yet returns empty string and shoot creation will fail and retry later.
func (r *RealClient) EnsureProject(ctx context.Context, projectName string, orgID uuid.UUID) (string, error) {
	namespace, found, err := r.getProjectNamespaceByOrgID(ctx, orgID)
	if err != nil {
		return "", err
	}
	if found {
		return namespace, nil
	}

	// No project found, create new one
	project := &gardencorev1beta1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: projectName,
			Labels: map[string]string{
				LabelOrganizationID: orgID.String(),
			},
		},
		Spec: gardencorev1beta1.ProjectSpec{
			Description: new("Fundament managed clusters"),
		},
	}

	r.logger.Info("creating gardener project",
		"project", projectName,
		"organization_id", orgID)

	if err := r.client.Create(ctx, project); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition: another worker created it, search again by label
			namespace, found, err := r.getProjectNamespaceByOrgID(ctx, orgID)
			if err != nil {
				return "", fmt.Errorf("after create conflict: %w", err)
			}
			if found {
				return namespace, nil
			}
			return "", fmt.Errorf("project exists but not found by label")
		}
		return "", fmt.Errorf("failed to create project: %w", err)
	}

	// Project just created, namespace won't be set yet (async)
	// Return empty - caller should handle retry
	return "", nil
}

// ApplyShoot creates or updates a Shoot in Gardener.
// Uses cluster ID label to find existing shoots. ShootName is only used for creation.
func (r *RealClient) ApplyShoot(ctx context.Context, cluster *ClusterToSync) error {
	if cluster.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if cluster.ShootName == "" {
		return fmt.Errorf("shoot name is required")
	}

	existing, err := r.getShootByClusterID(ctx, cluster.ID)
	if err != nil {
		return fmt.Errorf("failed to look up existing shoot: %w", err)
	}

	if existing != nil {
		r.updateShootSpec(existing, cluster)

		r.logger.Info("updating shoot",
			"shoot", existing.Name,
			"cluster_id", cluster.ID,
			"namespace", existing.Namespace)

		if err := r.client.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update shoot: %w", err)
		}
		return nil
	}

	shoot := r.buildShootSpec(cluster)

	r.logger.Info("creating shoot",
		"shoot", cluster.ShootName,
		"cluster_id", cluster.ID,
		"namespace", cluster.Namespace)

	if err := r.client.Create(ctx, shoot); err != nil {
		return fmt.Errorf("failed to create shoot: %w", err)
	}
	return nil
}

// DeleteShootByClusterID deletes a Shoot by cluster ID label.
func (r *RealClient) DeleteShootByClusterID(ctx context.Context, clusterID uuid.UUID) error {
	shoot, err := r.getShootByClusterID(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to look up shoot: %w", err)
	}

	if shoot == nil {
		r.logger.Debug("shoot not found for deletion", "cluster_id", clusterID)
		return nil
	}

	return r.deleteShoot(ctx, shoot)
}

// ListShoots returns all Shoots managed by this worker (across all namespaces).
func (r *RealClient) ListShoots(ctx context.Context) ([]ShootInfo, error) {
	shootList := &gardencorev1beta1.ShootList{}

	// List all shoots with fundament.io labels (across all namespaces)
	if err := r.client.List(ctx, shootList,
		client.HasLabels{LabelClusterID},
	); err != nil {
		return nil, fmt.Errorf("failed to list shoots: %w", err)
	}

	shoots := make([]ShootInfo, 0, len(shootList.Items))
	for i := range shootList.Items {
		shoot := &shootList.Items[i]
		clusterIDStr := shoot.Labels[LabelClusterID]
		clusterID, err := uuid.Parse(clusterIDStr)
		if err != nil {
			r.logger.Warn("shoot has invalid cluster-id label",
				"shoot", shoot.Name,
				"namespace", shoot.Namespace,
				"cluster_id", clusterIDStr)
			continue
		}

		shoots = append(shoots, ShootInfo{
			Name:      shoot.Name,
			Namespace: shoot.Namespace,
			ClusterID: clusterID,
			Labels:    shoot.Labels,
		})
	}

	return shoots, nil
}

// GetShootStatus returns the current reconciliation status of a Shoot.
func (r *RealClient) GetShootStatus(ctx context.Context, cluster *ClusterToSync) (*ShootStatus, error) {
	shoot, err := r.getShootByClusterID(ctx, cluster.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up shoot: %w", err)
	}

	if shoot == nil {
		return &ShootStatus{Status: StatusPending, Message: MsgShootNotFound}, nil
	}

	if shoot.DeletionTimestamp != nil {
		return &ShootStatus{Status: StatusDeleting, Message: "Shoot is being deleted"}, nil
	}

	if shoot.Status.LastOperation != nil {
		op := shoot.Status.LastOperation

		switch op.State {
		case gardencorev1beta1.LastOperationStatePending, gardencorev1beta1.LastOperationStateProcessing:
			return &ShootStatus{Status: StatusProgressing, Message: fmt.Sprintf("%s: %s", op.Type, op.Description)}, nil
		case gardencorev1beta1.LastOperationStateError, gardencorev1beta1.LastOperationStateFailed:
			return &ShootStatus{Status: StatusError, Message: op.Description}, nil
		case gardencorev1beta1.LastOperationStateSucceeded:
			if r.isShootHealthy(shoot) {
				return &ShootStatus{Status: StatusReady, Message: MsgShootReady}, nil
			}
			return &ShootStatus{Status: StatusProgressing, Message: "Shoot reconciled but not all conditions healthy"}, nil
		case gardencorev1beta1.LastOperationStateAborted:
			return &ShootStatus{Status: StatusError, Message: "Operation was aborted: " + op.Description}, nil
		}
	}

	// No last operation, likely still being created
	return &ShootStatus{Status: StatusProgressing, Message: "Shoot is being created"}, nil
}

// ensureCredentialsBinding creates a CredentialsBinding in the namespace if it doesn't exist.
// The binding references the shared credentials secret configured in ProviderConfig.
func (r *RealClient) ensureCredentialsBinding(ctx context.Context, namespace string) error {
	if r.provider.CredentialsSecretRef == "" {
		// No credentials secret configured, skip
		return nil
	}

	bindingName := r.provider.CredentialsBindingName
	if bindingName == "" {
		bindingName = "local"
	}

	existing := &securityv1alpha1.CredentialsBinding{}
	err := r.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      bindingName,
	}, existing)

	if err == nil {
		r.logger.Debug("credentials binding already exists",
			"namespace", namespace,
			"name", bindingName)
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get credentials binding: %w", err)
	}

	secretNs, secretName, err := parseSecretRef(r.provider.CredentialsSecretRef)
	if err != nil {
		return fmt.Errorf("invalid credentials secret ref: %w", err)
	}

	binding := &securityv1alpha1.CredentialsBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
		},
		Provider: securityv1alpha1.CredentialsBindingProvider{
			Type: r.provider.Type,
		},
		CredentialsRef: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       secretName,
			Namespace:  secretNs,
		},
	}

	r.logger.Info("creating credentials binding",
		"namespace", namespace,
		"name", bindingName,
		"secretRef", r.provider.CredentialsSecretRef)

	if err := r.client.Create(ctx, binding); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil // Race condition, that's fine
		}
		return fmt.Errorf("failed to create credentials binding: %w", err)
	}

	return nil
}

// parseSecretRef parses "namespace/name" format into separate parts.
func parseSecretRef(ref string) (namespace, name string, err error) {
	parts := make([]string, 0, 2)
	for i, j := 0, 0; i <= len(ref); i++ {
		if i == len(ref) || ref[i] == '/' {
			parts = append(parts, ref[j:i])
			j = i + 1
		}
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected format 'namespace/name', got %q", ref)
	}
	return parts[0], parts[1], nil
}

// getProjectNamespaceByOrgID finds a Project by organization ID label and returns its namespace.
// Returns (namespace, true, nil) if found, ("", false, nil) if not found.
// Also ensures CredentialsBinding exists if namespace is ready.
func (r *RealClient) getProjectNamespaceByOrgID(ctx context.Context, orgID uuid.UUID) (string, bool, error) {
	projectList := &gardencorev1beta1.ProjectList{}
	if err := r.client.List(ctx, projectList,
		client.MatchingLabels{LabelOrganizationID: orgID.String()},
	); err != nil {
		return "", false, fmt.Errorf("failed to list projects: %w", err)
	}

	if len(projectList.Items) == 0 {
		return "", false, nil
	}

	project := &projectList.Items[0]
	namespace := ""
	if project.Spec.Namespace != nil {
		namespace = *project.Spec.Namespace
	}

	r.logger.Debug("found existing project by organization label",
		"project", project.Name,
		"namespace", namespace,
		"organization_id", orgID)

	// Ensure CredentialsBinding exists in the namespace
	if namespace != "" {
		if err := r.ensureCredentialsBinding(ctx, namespace); err != nil {
			return "", false, fmt.Errorf("failed to ensure credentials binding: %w", err)
		}
	}

	return namespace, true, nil
}

// getShootByClusterID finds a Shoot by its cluster ID label.
// Returns nil if not found.
func (r *RealClient) getShootByClusterID(ctx context.Context, clusterID uuid.UUID) (*gardencorev1beta1.Shoot, error) {
	shootList := &gardencorev1beta1.ShootList{}
	if err := r.client.List(ctx, shootList,
		client.MatchingLabels{LabelClusterID: clusterID.String()},
	); err != nil {
		return nil, fmt.Errorf("list shoots: %w", err)
	}

	if len(shootList.Items) == 0 {
		return nil, nil
	}

	if len(shootList.Items) > 1 {
		r.logger.Warn("multiple shoots found for cluster ID",
			"cluster_id", clusterID,
			"count", len(shootList.Items))
	}

	return &shootList.Items[0], nil
}

// deleteShoot deletes a shoot, adding the required confirmation annotation.
func (r *RealClient) deleteShoot(ctx context.Context, shoot *gardencorev1beta1.Shoot) error {
	// Add the required confirmation annotation
	if shoot.Annotations == nil {
		shoot.Annotations = make(map[string]string)
	}
	shoot.Annotations["confirmation.gardener.cloud/deletion"] = "true"

	if err := r.client.Update(ctx, shoot); err != nil {
		return fmt.Errorf("failed to add deletion confirmation annotation: %w", err)
	}

	r.logger.Info("deleting shoot",
		"shoot", shoot.Name,
		"namespace", shoot.Namespace)

	if err := r.client.Delete(ctx, shoot); err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Debug("shoot already deleted", "shoot", shoot.Name)
			return nil
		}
		return fmt.Errorf("failed to delete shoot: %w", err)
	}

	return nil
}

// isShootHealthy checks if all key conditions are True.
func (r *RealClient) isShootHealthy(shoot *gardencorev1beta1.Shoot) bool {
	requiredConditions := []gardencorev1beta1.ConditionType{
		gardencorev1beta1.ShootAPIServerAvailable,
		gardencorev1beta1.ShootControlPlaneHealthy,
		gardencorev1beta1.ShootSystemComponentsHealthy,
	}

	conditionMap := make(map[gardencorev1beta1.ConditionType]gardencorev1beta1.ConditionStatus)
	for i := range shoot.Status.Conditions {
		c := &shoot.Status.Conditions[i]
		conditionMap[c.Type] = c.Status
	}

	for _, required := range requiredConditions {
		status, exists := conditionMap[required]
		if !exists || status != gardencorev1beta1.ConditionTrue {
			return false
		}
	}

	return true
}

// buildShootSpec creates a new Shoot spec from cluster info using provider config.
func (r *RealClient) buildShootSpec(cluster *ClusterToSync) *gardencorev1beta1.Shoot {
	shoot := &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.ShootName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				LabelClusterID:      cluster.ID.String(),
				LabelOrganizationID: cluster.OrganizationID.String(),
			},
			Annotations: map[string]string{
				AnnotationClusterName: cluster.Name,
			},
		},
		Spec: gardencorev1beta1.ShootSpec{
			CloudProfile: &gardencorev1beta1.CloudProfileReference{
				Kind: "CloudProfile",
				Name: r.provider.CloudProfile,
			},
			Region: cluster.Region,
			Kubernetes: gardencorev1beta1.Kubernetes{
				Version: cluster.KubernetesVersion,
			},
			Provider: gardencorev1beta1.Provider{
				Type:    r.provider.Type,
				Workers: r.buildWorkers(cluster),
			},
			Networking: &gardencorev1beta1.Networking{
				Type:  new("calico"), // Default CNI
				Nodes: new("10.0.0.0/16"),
			},
		},
	}

	// Set CredentialsBindingName (required for all providers, including local)
	if r.provider.CredentialsBindingName != "" {
		shoot.Spec.CredentialsBindingName = new(r.provider.CredentialsBindingName)
	}

	return shoot
}

// buildWorkers converts node pools to Gardener worker groups.
// If the cluster has no node pools, a default worker group is created.
func (r *RealClient) buildWorkers(cluster *ClusterToSync) []gardencorev1beta1.Worker {
	if len(cluster.NodePools) == 0 {
		maxSurge := intstr.FromInt32(1)
		maxUnavailable := intstr.FromInt32(0)
		imageVersion := r.provider.MachineImageVersion
		return []gardencorev1beta1.Worker{
			{
				Name: "default",
				Machine: gardencorev1beta1.Machine{
					Type: r.provider.DefaultMachineType,
					Image: &gardencorev1beta1.ShootMachineImage{
						Name:    r.provider.MachineImageName,
						Version: &imageVersion,
					},
				},
				Minimum:        1,
				Maximum:        3,
				MaxSurge:       &maxSurge,
				MaxUnavailable: &maxUnavailable,
			},
		}
	}

	workers := make([]gardencorev1beta1.Worker, len(cluster.NodePools))
	for i, np := range cluster.NodePools {
		maxSurge := intstr.FromInt32(1)
		maxUnavailable := intstr.FromInt32(0)
		imageVersion := r.provider.MachineImageVersion
		w := gardencorev1beta1.Worker{
			Name: np.Name,
			Machine: gardencorev1beta1.Machine{
				Type: np.MachineType,
				Image: &gardencorev1beta1.ShootMachineImage{
					Name:    r.provider.MachineImageName,
					Version: &imageVersion,
				},
			},
			Minimum:        np.AutoscaleMin,
			Maximum:        np.AutoscaleMax,
			MaxSurge:       &maxSurge,
			MaxUnavailable: &maxUnavailable,
		}
		if len(np.Zones) > 0 {
			w.Zones = np.Zones
		}
		workers[i] = w
	}
	return workers
}

// updateShootSpec updates an existing Shoot's spec and labels.
func (r *RealClient) updateShootSpec(shoot *gardencorev1beta1.Shoot, cluster *ClusterToSync) {
	if shoot.Labels == nil {
		shoot.Labels = make(map[string]string)
	}
	shoot.Labels[LabelClusterID] = cluster.ID.String()
	shoot.Labels[LabelOrganizationID] = cluster.OrganizationID.String()

	if shoot.Annotations == nil {
		shoot.Annotations = make(map[string]string)
	}
	shoot.Annotations[AnnotationClusterName] = cluster.Name

	shoot.Spec.Region = cluster.Region
	shoot.Spec.Kubernetes.Version = cluster.KubernetesVersion
	shoot.Spec.Provider.Workers = r.buildWorkers(cluster)
}
