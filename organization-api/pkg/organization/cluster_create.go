package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
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

	params := db.ClusterCreateParams{
		OrganizationID:    organizationID,
		Name:              req.Msg.GetName(),
		Region:            req.Msg.GetRegion(),
		KubernetesVersion: req.Msg.GetKubernetesVersion(),
	}

	// Catalog path: resolve the (region, version) pair and fill the legacy text
	// columns from it (expand phase - the worker still reads the text columns).
	// Legacy path: the text fields are stored as-is, ids stay NULL.
	if req.Msg.GetRegionId() != "" || req.Msg.GetKubernetesVersionId() != "" {
		regionID, err := parseUUIDField(req.Msg.GetRegionId(), "region_id")
		if err != nil {
			return nil, err
		}
		versionID, err := parseUUIDField(req.Msg.GetKubernetesVersionId(), "kubernetes_version_id")
		if err != nil {
			return nil, err
		}

		offering, err := s.queries.RegionKubernetesVersionGet(ctx, db.RegionKubernetesVersionGetParams{
			RegionID:            regionID,
			KubernetesVersionID: versionID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the kubernetes version is not offered in the selected region"))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve region offering: %w", err))
		}

		params.Region = offering.RegionName
		params.KubernetesVersion = offering.Version
		params.RegionID = pgtype.UUID{Bytes: regionID, Valid: true}
		params.KubernetesVersionID = pgtype.UUID{Bytes: versionID, Valid: true}
	} else if req.Msg.GetRegion() == "" || req.Msg.GetKubernetesVersion() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("either region_id and kubernetes_version_id or region and kubernetes_version must be set"))
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
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the kubernetes version is not offered in the selected region"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create cluster: %w", err))
	}

	s.logger.InfoContext(ctx, "cluster created",
		"cluster_id", clusterID,
		"organization_id", organizationID,
		"name", req.Msg.GetName(),
		"region", params.Region,
	)

	return connect.NewResponse(organizationv1.CreateClusterResponse_builder{
		ClusterId: clusterID.String(),
	}.Build()), nil
}

// parseUUIDField parses a request uuid field, mapping failures (including
// empty) to InvalidArgument.
func parseUUIDField(value, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a valid uuid", field))
	}
	return id, nil
}
