package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/mod/semver"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateCluster(
	ctx context.Context,
	req *organizationv1.UpdateClusterRequest,
) (*organizationv1.UpdateClusterResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	params := db.ClusterUpdateParams{
		ID: clusterID,
	}

	if req.HasKubernetesVersion() {
		// Resolve the new version against the catalog within the cluster's
		// region; the text and the catalog reference update in lockstep.
		cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
		}
		// ClusterGetByID also returns deleted clusters.
		if cluster.Deleted.Valid {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}

		// A cluster never moves back to a lower Kubernetes version: Gardener
		// rejects one, and the cluster sync keeps the version the Shoot runs.
		// This compares against the stored version, which can be older than
		// the one the Shoot runs if Gardener's maintenance updated it.
		current, requested := semverOf(cluster.KubernetesVersion), semverOf(req.GetKubernetesVersion())
		if semver.IsValid(current) && semver.IsValid(requested) && semver.Compare(requested, current) < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("kubernetes version %q is lower than the cluster's version %q; downgrades are not supported",
					req.GetKubernetesVersion(), cluster.KubernetesVersion))
		}

		offering, err := s.queries.RegionKubernetesVersionResolve(ctx, db.RegionKubernetesVersionResolveParams{
			RegionName: cluster.Region,
			Version:    req.GetKubernetesVersion(),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("kubernetes version %q is not offered in region %q", req.GetKubernetesVersion(), cluster.Region))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve region offering: %w", err))
		}

		params.KubernetesVersion = pgtype.Text{String: req.GetKubernetesVersion(), Valid: true}
		params.KubernetesVersionID = pgtype.UUID{Bytes: offering.KubernetesVersionID, Valid: true}
		params.ExpectedKubernetesVersion = pgtype.Text{String: cluster.KubernetesVersion, Valid: true}
	}

	rowsAffected, err := s.queries.ClusterUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster: %w", err))
	}

	if rowsAffected != 1 {
		if params.ExpectedKubernetesVersion.Valid {
			return nil, connect.NewError(connect.CodeAborted,
				fmt.Errorf("cluster was changed or deleted during the update; retry"))
		}
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster updated", "cluster_id", clusterID)

	return organizationv1.UpdateClusterResponse_builder{}.Build(), nil
}

// semverOf gives a Kubernetes version the single leading "v" that
// golang.org/x/mod/semver expects.
func semverOf(version string) string {
	return "v" + strings.TrimPrefix(version, "v")
}
