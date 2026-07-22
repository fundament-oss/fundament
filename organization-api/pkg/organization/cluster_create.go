package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateCluster(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateClusterRequest],
) (*connect.Response[organizationv1.CreateClusterResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanCreateCluster(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	// Resolve the (region, version) names against the catalog: creation is only
	// valid for offered combinations (pre-validate with ListRegions).
	offering, err := s.queries.RegionKubernetesVersionResolve(ctx, db.RegionKubernetesVersionResolveParams{
		RegionName: req.Msg.GetRegion(),
		Version:    req.Msg.GetKubernetesVersion(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("kubernetes version %q is not offered in region %q", req.Msg.GetKubernetesVersion(), req.Msg.GetRegion()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve region offering: %w", err))
	}

	params := db.ClusterCreateParams{
		OrganizationID:      organizationID,
		Name:                req.Msg.GetName(),
		Region:              req.Msg.GetRegion(),
		KubernetesVersion:   req.Msg.GetKubernetesVersion(),
		RegionID:            pgtype.UUID{Bytes: offering.RegionID, Valid: true},
		KubernetesVersionID: pgtype.UUID{Bytes: offering.KubernetesVersionID, Valid: true},
	}

	clusterID, err := s.queries.ClusterCreate(ctx, params)
	if err != nil {
		// ErrNoRows means the WHERE NOT EXISTS condition was false: a cluster with this name already exists
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a cluster with the name %q already exists", req.Msg.GetName()))
		}
		// Composite FK: the (region, version) pair vanished from the catalog
		// between resolve and insert.
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok &&
			pgErr.Code == pgerrcode.ForeignKeyViolation && pgErr.ConstraintName == dbconst.ConstraintClustersFkRegionVersion {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("kubernetes version %q is not offered in region %q", req.Msg.GetKubernetesVersion(), req.Msg.GetRegion()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster created",
		"cluster_id", clusterID,
		"organization_id", organizationID,
		"name", req.Msg.GetName(),
		"region", req.Msg.GetRegion(),
	)

	return connect.NewResponse(organizationv1.CreateClusterResponse_builder{
		ClusterId: clusterID.String(),
	}.Build()), nil
}
