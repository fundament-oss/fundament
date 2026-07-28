package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateClusterRequest],
) (*connect.Response[organizationv1.UpdateClusterResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	params := db.ClusterUpdateParams{
		ID: clusterID,
	}

	if req.Msg.HasKubernetesVersion() {
		// Resolve the new version against the catalog within the cluster's
		// region; the text and the catalog reference update in lockstep.
		cluster, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
		}

		offering, err := s.queries.RegionKubernetesVersionResolve(ctx, db.RegionKubernetesVersionResolveParams{
			RegionName: cluster.Region,
			Version:    req.Msg.GetKubernetesVersion(),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("kubernetes version %q is not offered in region %q", req.Msg.GetKubernetesVersion(), cluster.Region))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve region offering: %w", err))
		}

		params.KubernetesVersion = pgtype.Text{String: req.Msg.GetKubernetesVersion(), Valid: true}
		params.KubernetesVersionID = pgtype.UUID{Bytes: offering.KubernetesVersionID, Valid: true}
	}

	rowsAffected, err := s.queries.ClusterUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update cluster: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
	}

	s.logger.InfoContext(ctx, "cluster updated", "cluster_id", clusterID)

	return connect.NewResponse(organizationv1.UpdateClusterResponse_builder{}.Build()), nil
}
