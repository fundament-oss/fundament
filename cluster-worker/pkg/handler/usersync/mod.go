package usersync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Handler manages user ServiceAccount and ClusterRoleBinding lifecycle on shoot clusters.
type Handler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	shoot   shoot.ShootAccess
	logger  *slog.Logger
}

func New(pool *pgxpool.Pool, shootAccess shoot.ShootAccess, logger *slog.Logger) *Handler {
	return &Handler{
		pool:    pool,
		queries: db.New(pool),
		shoot:   shootAccess,
		logger:  logger.With("handler", "usersync"),
	}
}

// Sync processes a single outbox row for user access management.
// Dispatches based on the entity type in SyncContext.
func (h *Handler) Sync(ctx context.Context, id uuid.UUID, sc handler.SyncContext) error {
	switch sc.EntityType {
	case handler.EntityOrgUser:
		return h.syncOrgUser(ctx, id)
	case handler.EntityProjectMember:
		return h.syncProjectMember(ctx, id)
	case handler.EntityCluster:
		return h.syncClusterReady(ctx, id)
	default:
		return fmt.Errorf("unexpected entity type %s for user sync handler", sc.EntityType)
	}
}

// syncOrgUser handles an org membership change: fan out to all ready clusters for the org.
func (h *Handler) syncOrgUser(ctx context.Context, orgUserID uuid.UUID) error {
	ou, err := h.queries.OrgUserGetForSync(ctx, db.OrgUserGetForSyncParams{ID: orgUserID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("org user not found, skipping", "org_user_id", orgUserID)
			return nil
		}
		return fmt.Errorf("get org user: %w", err)
	}

	clusters, err := h.queries.ClusterListReadyForOrg(ctx, db.ClusterListReadyForOrgParams{
		OrganizationID: ou.OrganizationID,
	})
	if err != nil {
		return fmt.Errorf("list ready clusters for org: %w", err)
	}

	if len(clusters) == 0 {
		h.logger.Debug("no ready clusters for org, skipping", "org_user_id", orgUserID, "org_id", ou.OrganizationID)
		return nil
	}

	var errs []error
	for _, clusterID := range clusters {
		if err := h.syncUserToCluster(ctx, ou.UserID, clusterID); err != nil {
			h.logger.Error("failed to sync user to cluster",
				"user_id", ou.UserID,
				"cluster_id", clusterID,
				"error", err)
			h.createUserSyncEvent(ctx, clusterID, dbconst.ClusterEventEventType_UserSyncFailed,
				fmt.Sprintf("Org user sync failed for user %s: %s", ou.UserID, err))
			errs = append(errs, err)
		} else {
			h.createUserSyncEvent(ctx, clusterID, dbconst.ClusterEventEventType_UserSyncSucceeded,
				fmt.Sprintf("Org user synced: %s", ou.UserID))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("sync org user %s: %w", orgUserID, err)
	}
	return nil
}

// syncProjectMember handles a project membership change: sync to the one cluster.
func (h *Handler) syncProjectMember(ctx context.Context, projectMemberID uuid.UUID) error {
	pm, err := h.queries.ProjectMemberGetForSync(ctx, db.ProjectMemberGetForSyncParams{ID: projectMemberID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("project member not found, skipping", "project_member_id", projectMemberID)
			return nil
		}
		return fmt.Errorf("get project member: %w", err)
	}

	if err := h.syncUserToCluster(ctx, pm.UserID, pm.ClusterID); err != nil {
		h.createUserSyncEvent(ctx, pm.ClusterID, dbconst.ClusterEventEventType_UserSyncFailed,
			fmt.Sprintf("Project member sync failed for user %s: %s", pm.UserID, err))
		return err
	}
	h.createUserSyncEvent(ctx, pm.ClusterID, dbconst.ClusterEventEventType_UserSyncSucceeded,
		fmt.Sprintf("Project member synced: %s", pm.UserID))
	return nil
}

// syncClusterReady handles a cluster becoming ready: provision all users.
func (h *Handler) syncClusterReady(ctx context.Context, clusterID uuid.UUID) error {
	users, err := h.queries.UserListForCluster(ctx, db.UserListForClusterParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("list users for cluster: %w", err)
	}

	if len(users) == 0 {
		h.logger.Debug("no users for cluster, ensuring namespace only", "cluster_id", clusterID)
		if err := h.shoot.EnsureNamespace(ctx, clusterID, shoot.FundamentNamespace); err != nil {
			return fmt.Errorf("ensure namespace: %w", err)
		}
		h.createUserSyncEvent(ctx, clusterID, dbconst.ClusterEventEventType_UserSyncSucceeded, "No users to provision")
		return nil
	}

	// Ensure namespace exists before creating SAs.
	if err := h.shoot.EnsureNamespace(ctx, clusterID, shoot.FundamentNamespace); err != nil {
		h.createUserSyncEvent(ctx, clusterID, dbconst.ClusterEventEventType_UserSyncFailed, "ensure namespace: "+err.Error())
		return fmt.Errorf("ensure namespace: %w", err)
	}

	var errs []error
	synced := 0
	for _, user := range users {
		email := ""
		if user.Email.Valid {
			email = user.Email.String
		}
		if err := h.applyUserAccess(ctx, clusterID, user.UserID, email, user.AccessLevel); err != nil {
			h.logger.Error("failed to sync user on cluster ready",
				"user_id", user.UserID,
				"cluster_id", clusterID,
				"error", err)
			errs = append(errs, err)
		} else {
			synced++
		}
	}

	if err := errors.Join(errs...); err != nil {
		h.createUserSyncEvent(ctx, clusterID, dbconst.ClusterEventEventType_UserSyncFailed,
			fmt.Sprintf("Provisioned %d/%d users, %d failed", synced, len(users), len(errs)))
		return fmt.Errorf("sync cluster ready %s: %w", clusterID, err)
	}

	h.createUserSyncEvent(ctx, clusterID, dbconst.ClusterEventEventType_UserSyncSucceeded,
		fmt.Sprintf("Provisioned %d user(s)", synced))
	return nil
}

// syncUserToCluster resolves access and converges SA/CRB state for one user on one cluster.
func (h *Handler) syncUserToCluster(ctx context.Context, userID, clusterID uuid.UUID) error {
	accessLevel, err := h.queries.ResolveUserAccess(ctx, db.ResolveUserAccessParams{
		UserID:    userID,
		ClusterID: clusterID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("cluster not found for user sync, skipping", "user_id", userID, "cluster_id", clusterID)
			return nil
		}
		return fmt.Errorf("resolve user access: %w", err)
	}

	email, err := h.queries.UserGetEmail(ctx, db.UserGetEmailParams{ID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Info("user not found, skipping", "user_id", userID)
			return nil
		}
		return fmt.Errorf("get user email: %w", err)
	}

	emailStr := ""
	if email.Valid {
		emailStr = email.String
	}

	// Ensure namespace exists before creating resources.
	if accessLevel != "none" {
		if err := h.shoot.EnsureNamespace(ctx, clusterID, shoot.FundamentNamespace); err != nil {
			return fmt.Errorf("ensure namespace: %w", err)
		}
	}

	return h.applyUserAccess(ctx, clusterID, userID, emailStr, accessLevel)
}

// applyUserAccess converges the SA and CRB state based on the desired access level.
func (h *Handler) applyUserAccess(ctx context.Context, clusterID, userID uuid.UUID, email, accessLevel string) error {
	saName := shoot.SAName(userID)
	crbName := shoot.CRBName(userID)
	labels := map[string]string{
		shoot.LabelUserID: userID.String(),
	}
	annotations := map[string]string{
		shoot.AnnotationUserName: email,
	}

	switch accessLevel {
	case "admin":
		// Ensure SA + CRB
		if err := h.shoot.EnsureServiceAccount(ctx, clusterID, shoot.FundamentNamespace, saName, labels, annotations); err != nil {
			return fmt.Errorf("ensure SA for admin: %w", err)
		}
		if err := h.shoot.EnsureClusterRoleBinding(ctx, clusterID, crbName, shoot.FundamentNamespace, saName, labels, annotations); err != nil {
			return fmt.Errorf("ensure CRB for admin: %w", err)
		}
		h.logger.Info("synced admin access",
			"user_id", userID, "cluster_id", clusterID)

	case "member":
		// Ensure SA, delete CRB if exists
		if err := h.shoot.EnsureServiceAccount(ctx, clusterID, shoot.FundamentNamespace, saName, labels, annotations); err != nil {
			return fmt.Errorf("ensure SA for member: %w", err)
		}
		if err := h.shoot.DeleteClusterRoleBinding(ctx, clusterID, crbName); err != nil {
			return fmt.Errorf("delete CRB for member: %w", err)
		}
		h.logger.Info("synced member access",
			"user_id", userID, "cluster_id", clusterID)

	case "none":
		// Delete both SA and CRB
		if err := h.shoot.DeleteServiceAccount(ctx, clusterID, shoot.FundamentNamespace, saName); err != nil {
			return fmt.Errorf("delete SA: %w", err)
		}
		if err := h.shoot.DeleteClusterRoleBinding(ctx, clusterID, crbName); err != nil {
			return fmt.Errorf("delete CRB: %w", err)
		}
		h.logger.Info("removed user access",
			"user_id", userID, "cluster_id", clusterID)

	default:
		panic(fmt.Sprintf("unhandled access level: %s", accessLevel))
	}

	return nil
}

// Reconcile performs full user-access reconciliation for all ready clusters.
// For each cluster: list actual SAs/CRBs, compare against desired state from DB,
// create missing, delete orphaned, fix CRB mismatches.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
	}

	h.logger.Info("starting user sync reconciliation")

	clusterIDs, err := h.queries.ClusterListReady(ctx)
	if err != nil {
		return fmt.Errorf("list ready clusters: %w", err)
	}

	var errs []error
	for _, clusterID := range clusterIDs {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		if err := h.reconcileCluster(ctx, clusterID); err != nil {
			h.logger.Error("failed to reconcile cluster",
				"cluster_id", clusterID,
				"error", err)
			errs = append(errs, err)
		}
	}

	h.logger.Info("user sync reconciliation complete", "clusters", len(clusterIDs))
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("user sync reconcile: %w", err)
	}
	return nil
}

func (h *Handler) reconcileCluster(ctx context.Context, clusterID uuid.UUID) error {
	// 1. Ensure namespace exists.
	if err := h.shoot.EnsureNamespace(ctx, clusterID, shoot.FundamentNamespace); err != nil {
		return fmt.Errorf("ensure namespace: %w", err)
	}

	// 2. Get desired state from DB.
	desiredUsers, err := h.queries.UserListForCluster(ctx, db.UserListForClusterParams{ClusterID: clusterID})
	if err != nil {
		return fmt.Errorf("list users for cluster: %w", err)
	}

	// 3. Get actual state from shoot.
	actualSAs, err := h.shoot.ListServiceAccounts(ctx, clusterID, shoot.FundamentNamespace, shoot.LabelUserID)
	if err != nil {
		return fmt.Errorf("list SAs: %w", err)
	}

	actualCRBs, err := h.shoot.ListClusterRoleBindings(ctx, clusterID, shoot.LabelUserID)
	if err != nil {
		return fmt.Errorf("list CRBs: %w", err)
	}

	// 4. Build plan, then apply.
	plan := buildReconcilePlan(desiredUsers, actualSAs, actualCRBs)
	if err := h.applyReconcilePlan(ctx, clusterID, plan); err != nil {
		return fmt.Errorf("reconcile cluster %s: %w", clusterID, err)
	}
	return nil
}

// applyReconcilePlan executes the reconciliation actions via ShootAccess.
func (h *Handler) applyReconcilePlan(ctx context.Context, clusterID uuid.UUID, plan ReconcilePlan) error {
	var errs []error
	for _, action := range plan {
		switch action.Type {
		case ActionEnsureSA:
			labels := map[string]string{shoot.LabelUserID: action.UserID.String()}
			annotations := map[string]string{shoot.AnnotationUserName: action.Email}
			if err := h.shoot.EnsureServiceAccount(ctx, clusterID, shoot.FundamentNamespace, shoot.SAName(action.UserID), labels, annotations); err != nil {
				errs = append(errs, err)
			}
		case ActionDeleteSA:
			if err := h.shoot.DeleteServiceAccount(ctx, clusterID, shoot.FundamentNamespace, action.Name); err != nil {
				errs = append(errs, err)
			}
		case ActionEnsureCRB:
			labels := map[string]string{shoot.LabelUserID: action.UserID.String()}
			annotations := map[string]string{shoot.AnnotationUserName: action.Email}
			saName := shoot.SAName(action.UserID)
			crbName := shoot.CRBName(action.UserID)
			if err := h.shoot.EnsureClusterRoleBinding(ctx, clusterID, crbName, shoot.FundamentNamespace, saName, labels, annotations); err != nil {
				errs = append(errs, err)
			}
		case ActionDeleteCRB:
			if err := h.shoot.DeleteClusterRoleBinding(ctx, clusterID, action.Name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("apply reconcile plan: %w", err)
	}
	return nil
}

// createUserSyncEvent writes a cluster event for user sync operations.
// Errors are logged but not returned — event creation is best-effort.
func (h *Handler) createUserSyncEvent(ctx context.Context, clusterID uuid.UUID, eventType dbconst.ClusterEventEventType, message string) {
	if err := h.queries.ClusterCreateUserSyncEvent(ctx, db.ClusterCreateUserSyncEventParams{
		ClusterID: clusterID,
		EventType: string(eventType),
		Message:   pgtype.Text{String: message, Valid: true},
	}); err != nil {
		h.logger.Warn("failed to create user sync event",
			"cluster_id", clusterID,
			"event_type", eventType,
			"error", err)
	}
}

func groupResourcesByUserID(resources []shoot.ResourceInfo) (map[uuid.UUID][]shoot.ResourceInfo, map[string]struct{}) {
	grouped := make(map[uuid.UUID][]shoot.ResourceInfo)
	orphans := make(map[string]struct{})

	for _, resource := range resources {
		uid, ok := resource.Labels[shoot.LabelUserID]
		if !ok {
			orphans[resource.Name] = struct{}{}
			continue
		}

		userID, err := uuid.Parse(uid)
		if err != nil {
			orphans[resource.Name] = struct{}{}
			continue
		}

		grouped[userID] = append(grouped[userID], resource)
	}

	return grouped, orphans
}

func classifyServiceAccounts(resources []shoot.ResourceInfo, userID uuid.UUID, labels, annotations map[string]string) (bool, []string) {
	canonicalName := shoot.SAName(userID)
	hasCanonical := false
	var duplicates []string

	for i := range resources {
		if resources[i].Name == canonicalName {
			hasCanonical = serviceAccountMatches(&resources[i], labels, annotations)
			continue
		}
		duplicates = append(duplicates, resources[i].Name)
	}

	return hasCanonical, duplicates
}

func classifyClusterRoleBindings(resources []shoot.ResourceInfo, userID uuid.UUID, labels, annotations map[string]string) (bool, []string) {
	canonicalName := shoot.CRBName(userID)
	hasCanonical := false
	var duplicates []string

	for i := range resources {
		if resources[i].Name == canonicalName {
			hasCanonical = clusterRoleBindingMatches(&resources[i], userID, labels, annotations)
			continue
		}
		duplicates = append(duplicates, resources[i].Name)
	}

	return hasCanonical, duplicates
}

func serviceAccountMatches(resource *shoot.ResourceInfo, labels, annotations map[string]string) bool {
	return metadataContains(resource, labels, annotations)
}

func clusterRoleBindingMatches(resource *shoot.ResourceInfo, userID uuid.UUID, labels, annotations map[string]string) bool {
	if !metadataContains(resource, labels, annotations) {
		return false
	}

	expectedSubject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      shoot.SAName(userID),
		Namespace: shoot.FundamentNamespace,
	}
	expectedRoleRef := rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "cluster-admin",
	}

	return resource.RoleRef == expectedRoleRef &&
		len(resource.Subjects) == 1 &&
		resource.Subjects[0] == expectedSubject
}

func metadataContains(resource *shoot.ResourceInfo, labels, annotations map[string]string) bool {
	for k, v := range labels {
		if resource.Labels[k] != v {
			return false
		}
	}

	for k, v := range annotations {
		if resource.Annotations[k] != v {
			return false
		}
	}

	return true
}

func resourceNames(resources []shoot.ResourceInfo) []string {
	names := make([]string, 0, len(resources))
	for _, resource := range resources {
		names = append(names, resource.Name)
	}
	slices.Sort(names)
	return names
}

func sortedResourceNames(names map[string]struct{}) []string {
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}
