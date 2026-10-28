package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteCluster(
	ctx context.Context,
	req *organizationv1.DeleteClusterRequest,
) (*organizationv1.DeleteClusterResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.ClusterDelete(ctx, db.ClusterDeleteParams{ID: clusterID})
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			// Raised by clusters_tr_verify_deleted: the cluster still has projects.
			// RaiseException is the code of any plpgsql RAISE, so this holds only
			// while that trigger is the one on clusters that raises when deleted
			// is set; a second one would need its own ERRCODE to tell them apart.
			// Refused rather than cascaded: deleting the projects and their
			// namespaces together with the cluster would change what a soft
			// delete takes with it, which is a product decision still to be made.
			if pgErr.Code == pgerrcode.RaiseException {
				return nil, connect.NewError(connect.CodeFailedPrecondition, s.clusterProjectsError(ctx, clusterID))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete cluster: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster deleted", "cluster_id", clusterID)

	return organizationv1.DeleteClusterResponse_builder{}.Build(), nil
}

// clusterProjectsError names the projects that keep a cluster from being
// deleted, so the caller knows what to remove first.
func (s *Server) clusterProjectsError(ctx context.Context, clusterID uuid.UUID) error {
	const msg = "cluster has projects that must be deleted first"

	projects, err := s.queries.ProjectListByClusterID(ctx, db.ProjectListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		s.logger.WarnContext(ctx, "list projects blocking cluster deletion", "cluster_id", clusterID, "error", err)
		return errors.New(msg)
	}
	if len(projects) == 0 {
		return errors.New(msg)
	}

	names := make([]string, 0, len(projects))
	for i := range projects {
		names = append(names, fmt.Sprintf("%q", projects[i].Name))
	}

	return fmt.Errorf("%s: %s", msg, strings.Join(names, ", "))
}
