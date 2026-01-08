// Package gardener provides the real Gardener client implementation.
package gardener

import (
	"context"
	"fmt"
	"log/slog"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProviderConfig holds cloud provider-specific configuration.
type ProviderConfig struct {
	Type              string // e.g., "metal", "local"
	CloudProfile      string // e.g., "metal", "local"
	SecretBindingName string // e.g., "metal-credentials", "" for local
	Region            string // e.g., "nl-central-1", "local"
	MachineType       string // e.g., "m5.xlarge", "local"
	MachineImageName  string // e.g., "gardenlinux", "local"
	MachineImageVer   string // e.g., "1592.1.0", "1.0.0"
	KubernetesVersion string // e.g., "1.29.4", "1.31.1"
	Zone              string // e.g., "nl-central-1a", "0" for local
}

// RealClient implements Client using the actual Gardener API.
type RealClient struct {
	client    client.Client
	namespace string // Gardener project namespace, e.g., "garden-fundament"
	provider  ProviderConfig
	logger    *slog.Logger
}

// NewReal creates a new RealClient that connects to Gardener.
// If kubeconfigPath is empty, it uses in-cluster config.
func NewReal(kubeconfigPath string, namespace string, provider ProviderConfig, logger *slog.Logger) (*RealClient, error) {
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

	// Create scheme with Gardener types
	scheme := runtime.NewScheme()
	if err := gardencorev1beta1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add Gardener types to scheme: %w", err)
	}

	// Create controller-runtime client
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	logger.Info("connected to Gardener API",
		"namespace", namespace,
		"host", cfg.Host,
		"provider", provider.Type,
		"cloudProfile", provider.CloudProfile)

	return &RealClient{
		client:    c,
		namespace: namespace,
		provider:  provider,
		logger:    logger,
	}, nil
}

// ApplyShoot creates or updates a Shoot in Gardener.
func (r *RealClient) ApplyShoot(ctx context.Context, cluster ClusterToSync) error {
	shootName := ShootName(cluster.OrganizationName, cluster.Name)

	// Check if Shoot already exists
	existing := &gardencorev1beta1.Shoot{}
	err := r.client.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      shootName,
	}, existing)

	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get existing shoot: %w", err)
	}

	shoot := r.buildShootSpec(cluster)

	if apierrors.IsNotFound(err) {
		// Create new Shoot
		r.logger.Info("creating shoot",
			"shoot", shootName,
			"cluster_id", cluster.ID,
			"namespace", r.namespace)

		if err := r.client.Create(ctx, shoot); err != nil {
			return fmt.Errorf("failed to create shoot: %w", err)
		}
		return nil
	}

	// Update existing Shoot - preserve resourceVersion for optimistic locking
	shoot.ResourceVersion = existing.ResourceVersion
	r.logger.Info("updating shoot",
		"shoot", shootName,
		"cluster_id", cluster.ID,
		"namespace", r.namespace)

	if err := r.client.Update(ctx, shoot); err != nil {
		return fmt.Errorf("failed to update shoot: %w", err)
	}

	return nil
}

// DeleteShoot deletes a Shoot by cluster info.
func (r *RealClient) DeleteShoot(ctx context.Context, cluster ClusterToSync) error {
	shootName := ShootName(cluster.OrganizationName, cluster.Name)
	return r.DeleteShootByName(ctx, shootName)
}

// DeleteShootByName deletes a Shoot by name.
func (r *RealClient) DeleteShootByName(ctx context.Context, name string) error {
	shoot := &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.namespace,
		},
	}

	r.logger.Info("deleting shoot",
		"shoot", name,
		"namespace", r.namespace)

	if err := r.client.Delete(ctx, shoot); err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Debug("shoot already deleted", "shoot", name)
			return nil
		}
		return fmt.Errorf("failed to delete shoot: %w", err)
	}

	return nil
}

// ListShoots returns all Shoots managed by this worker.
func (r *RealClient) ListShoots(ctx context.Context) ([]ShootInfo, error) {
	shootList := &gardencorev1beta1.ShootList{}

	// List all shoots in our namespace with fundament.io labels
	if err := r.client.List(ctx, shootList,
		client.InNamespace(r.namespace),
		client.HasLabels{"fundament.io/cluster-id"},
	); err != nil {
		return nil, fmt.Errorf("failed to list shoots: %w", err)
	}

	var shoots []ShootInfo
	for _, shoot := range shootList.Items {
		clusterIDStr := shoot.Labels["fundament.io/cluster-id"]
		clusterID, err := uuid.Parse(clusterIDStr)
		if err != nil {
			r.logger.Warn("shoot has invalid cluster-id label",
				"shoot", shoot.Name,
				"cluster_id", clusterIDStr)
			continue
		}

		shoots = append(shoots, ShootInfo{
			Name:      shoot.Name,
			ClusterID: clusterID,
			Labels:    shoot.Labels,
		})
	}

	return shoots, nil
}

// GetShootStatus returns the current reconciliation status of a Shoot.
func (r *RealClient) GetShootStatus(ctx context.Context, cluster ClusterToSync) (string, string, error) {
	shootName := ShootName(cluster.OrganizationName, cluster.Name)

	shoot := &gardencorev1beta1.Shoot{}
	err := r.client.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      shootName,
	}, shoot)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return "pending", "Shoot not found in Gardener", nil
		}
		return "", "", fmt.Errorf("failed to get shoot: %w", err)
	}

	// Check if being deleted
	if shoot.DeletionTimestamp != nil {
		return "deleting", "Shoot is being deleted", nil
	}

	// Check last operation status
	if shoot.Status.LastOperation != nil {
		op := shoot.Status.LastOperation

		switch op.State {
		case gardencorev1beta1.LastOperationStateProcessing:
			return "progressing", fmt.Sprintf("%s: %s", op.Type, op.Description), nil
		case gardencorev1beta1.LastOperationStateError, gardencorev1beta1.LastOperationStateFailed:
			return "error", op.Description, nil
		case gardencorev1beta1.LastOperationStateSucceeded:
			// Check if all conditions are healthy
			if r.isShootHealthy(shoot) {
				return "ready", "Shoot is ready", nil
			}
			return "progressing", "Shoot reconciled but not all conditions healthy", nil
		case gardencorev1beta1.LastOperationStateAborted:
			return "error", "Operation was aborted: " + op.Description, nil
		}
	}

	// No last operation, likely still being created
	return "progressing", "Shoot is being created", nil
}

// isShootHealthy checks if all key conditions are True.
func (r *RealClient) isShootHealthy(shoot *gardencorev1beta1.Shoot) bool {
	requiredConditions := []gardencorev1beta1.ConditionType{
		gardencorev1beta1.ShootAPIServerAvailable,
		gardencorev1beta1.ShootControlPlaneHealthy,
		gardencorev1beta1.ShootSystemComponentsHealthy,
	}

	conditionMap := make(map[gardencorev1beta1.ConditionType]gardencorev1beta1.ConditionStatus)
	for _, c := range shoot.Status.Conditions {
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

// buildShootSpec creates a Shoot spec from cluster info using provider config.
func (r *RealClient) buildShootSpec(cluster ClusterToSync) *gardencorev1beta1.Shoot {
	shootName := ShootName(cluster.OrganizationName, cluster.Name)

	// TODO: Make these configurable per-cluster once we extend the DB schema
	minWorkers := int32(1)
	maxWorkers := int32(3)
	maxSurge := intstr.FromInt32(1)
	maxUnavailable := intstr.FromInt32(0)

	// Use provider config values, falling back to defaults
	machineType := r.provider.MachineType
	if machineType == "" {
		machineType = DefaultMachineType
	}
	machineImageName := r.provider.MachineImageName
	if machineImageName == "" {
		machineImageName = DefaultMachineImageName
	}
	machineImageVer := r.provider.MachineImageVer
	if machineImageVer == "" {
		machineImageVer = DefaultMachineImageVersion
	}
	k8sVersion := r.provider.KubernetesVersion
	if k8sVersion == "" {
		k8sVersion = DefaultKubernetesVersion
	}

	shoot := &gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shootName,
			Namespace: r.namespace,
			Labels: map[string]string{
				"fundament.io/cluster-id": cluster.ID.String(),
				"fundament.io/organization": cluster.OrganizationName,
			},
		},
		Spec: gardencorev1beta1.ShootSpec{
			CloudProfile: &gardencorev1beta1.CloudProfileReference{
				Kind: "CloudProfile",
				Name: r.provider.CloudProfile,
			},
			Region: r.provider.Region,
			Kubernetes: gardencorev1beta1.Kubernetes{
				Version: k8sVersion,
			},
			Provider: gardencorev1beta1.Provider{
				Type: r.provider.Type,
				Workers: []gardencorev1beta1.Worker{
					{
						Name: "default",
						Machine: gardencorev1beta1.Machine{
							Type: machineType,
							Image: &gardencorev1beta1.ShootMachineImage{
								Name:    machineImageName,
								Version: ptr.To(machineImageVer),
							},
						},
						Minimum:        minWorkers,
						Maximum:        maxWorkers,
						MaxSurge:       &maxSurge,
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			Networking: &gardencorev1beta1.Networking{
				Type:  ptr.To("calico"), // Default CNI
				Nodes: ptr.To("10.0.0.0/16"),
			},
		},
	}

	// Only set SecretBindingName if configured (local provider doesn't need it)
	if r.provider.SecretBindingName != "" {
		shoot.Spec.SecretBindingName = ptr.To(r.provider.SecretBindingName)
	}

	// Only set zones if configured (local provider doesn't support zones)
	if r.provider.Zone != "" {
		shoot.Spec.Provider.Workers[0].Zones = []string{r.provider.Zone}
	}

	return shoot
}
