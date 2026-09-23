package projectrbac

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// DesiredLister is the query Converger needs; *db.Queries satisfies it.
type DesiredLister interface {
	ProjectRoleBindingListForCluster(ctx context.Context, arg db.ProjectRoleBindingListForClusterParams) ([]db.ProjectRoleBindingListForClusterRow, error)
}

// Converger converges all project RoleBindings on one cluster.
type Converger struct {
	queries DesiredLister
	shoot   shoot.ShootAccess
	logger  *slog.Logger
}

func NewConverger(queries DesiredLister, shootAccess shoot.ShootAccess, logger *slog.Logger) *Converger {
	return &Converger{
		queries: queries,
		shoot:   shootAccess,
		logger:  logger.With("component", "project-rbac"),
	}
}

// clusterLocks serializes Converge per cluster across every Converger in the
// process: usersync and namespace sync each build one, and the outbox and
// reconcile workers call them concurrently. Two passes on one cluster race on a
// role change, where one deletes a binding to recreate it with the new roleRef
// while the other fails to find it. Values are chan struct{} with capacity 1.
var clusterLocks sync.Map

// lockCluster waits for the cluster's lock, or for ctx to end.
func lockCluster(ctx context.Context, clusterID uuid.UUID) (func(), error) {
	v, _ := clusterLocks.LoadOrStore(clusterID, make(chan struct{}, 1))
	lock := v.(chan struct{})
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for project role binding lock on cluster %s: %w", clusterID, ctx.Err())
	}
}

// Converge makes the cluster's project RoleBindings match the database. It is
// idempotent and cluster-wide, so any trigger (membership change, namespace
// created, cluster ready, reconcile) can call it without scoping.
func (c *Converger) Converge(ctx context.Context, clusterID uuid.UUID) error {
	unlock, err := lockCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	defer unlock()

	rows, err := c.queries.ProjectRoleBindingListForCluster(ctx, db.ProjectRoleBindingListForClusterParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("list desired project role bindings: %w", err)
	}
	desired := make([]Desired, len(rows))
	for i, r := range rows {
		desired[i] = Desired{UserID: r.UserID, NamespaceID: r.NamespaceID, Role: r.Role}
	}

	namespaces, err := c.shoot.ListNamespaces(ctx, clusterID, LabelNamespaceID)
	if err != nil {
		return fmt.Errorf("list managed namespaces: %w", err)
	}

	actual, err := c.shoot.ListRoleBindings(ctx, clusterID, shoot.LabelUserID)
	if err != nil {
		return fmt.Errorf("list role bindings: %w", err)
	}

	var errs []error
	for _, a := range BuildPlan(desired, namespaces, actual) {
		err := c.apply(ctx, clusterID, a)
		if err != nil {
			errs = append(errs, err)
		}
	}
	err = errors.Join(errs...)
	if err != nil {
		return fmt.Errorf("converge project role bindings on cluster %s: %w", clusterID, err)
	}
	return nil
}

func (c *Converger) apply(ctx context.Context, clusterID uuid.UUID, a Action) error {
	if a.Delete {
		err := c.shoot.DeleteRoleBinding(ctx, clusterID, a.Namespace, a.Name)
		if err != nil {
			return fmt.Errorf("delete role binding %s/%s: %w", a.Namespace, a.Name, err)
		}
		c.logger.Info("deleted project role binding", "cluster_id", clusterID, "namespace", a.Namespace, "name", a.Name)
		return nil
	}
	err := c.shoot.EnsureRoleBinding(ctx, clusterID, a.Namespace, a.Name, RoleRef(a.ClusterRole), Subjects(a.UserID), Labels(a.UserID))
	if err != nil {
		return fmt.Errorf("ensure role binding %s/%s: %w", a.Namespace, a.Name, err)
	}
	c.logger.Info("ensured project role binding", "cluster_id", clusterID, "namespace", a.Namespace, "name", a.Name, "cluster_role", a.ClusterRole)
	return nil
}
