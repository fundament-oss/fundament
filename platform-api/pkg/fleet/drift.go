package fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

// driftKey is where a page of findings ends.
type driftKey struct {
	Kind          string    `json:"k"`
	FirstDetected time.Time `json:"f"`
	ID            uuid.UUID `json:"i"`
}

// ListDriftFindings reads the open findings cluster-worker's reconcile loop
// recorded. They are only as current as its last pass, which the summary
// reports.
func (s *Server) ListDriftFindings(ctx context.Context, req *fleetv1.ListDriftFindingsRequest) (*fleetv1.ListDriftFindingsResponse, error) {
	size := pageSize(req.GetPageSize())
	params := db.DriftFindingListParams{PageLimit: size + 1}
	var after driftKey
	ok, err := decodePageToken(req.GetPageToken(), &after)
	if err != nil {
		return nil, err
	}
	if ok {
		params.AfterKind = pgtype.Text{String: after.Kind, Valid: true}
		params.AfterFirstDetected = pgtype.Timestamptz{Time: after.FirstDetected, Valid: true}
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}

	rows, err := s.queries.DriftFindingList(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list drift findings: %w", err)
	}
	resp := fleetv1.ListDriftFindingsResponse_builder{}
	if len(rows) > int(size) {
		rows = rows[:size]
		last := &rows[len(rows)-1]
		resp.NextPageToken = encodePageToken(driftKey{Kind: last.Kind, FirstDetected: last.FirstDetected.Time, ID: last.ID})
	}

	resp.Findings = make([]*fleetv1.DriftFinding, 0, len(rows))
	for i := range rows {
		resp.Findings = append(resp.Findings, driftFindingFrom(&rows[i]))
	}
	return resp.Build(), nil
}

func driftFindingFrom(r *db.DriftFindingListRow) *fleetv1.DriftFinding {
	kind, detail := driftKindDetail(r)
	b := fleetv1.DriftFinding_builder{
		Id:               r.ID.String(),
		Kind:             kind,
		OrganizationName: stringOrNil(r.OrganizationName),
		ClusterName:      stringOrNil(r.ClusterName),
		ShootName:        stringOrNil(r.ShootName),
		ShootNamespace:   stringOrNil(r.ShootNamespace),
		FirstDetected:    timestampOrNil(r.FirstDetected),
		LastDetected:     timestampOrNil(r.LastDetected),
		Detail:           detail,
	}
	if r.ClusterID.Valid {
		b.ClusterId = new(uuid.UUID(r.ClusterID.Bytes).String())
	}
	if r.OrganizationID.Valid {
		b.OrganizationId = new(uuid.UUID(r.OrganizationID.Bytes).String())
	}
	return b.Build()
}

func driftKindDetail(r *db.DriftFindingListRow) (fleetv1.DriftKind, string) {
	switch dbconst.DriftFindingKind(r.Kind) {
	case dbconst.DriftFindingKind_ShootWithoutCluster:
		return fleetv1.DriftKind_DRIFT_KIND_SHOOT_WITHOUT_CLUSTER,
			"Gardener has a shoot whose cluster id matches no cluster."
	case dbconst.DriftFindingKind_ShootOfDeletedCluster:
		return fleetv1.DriftKind_DRIFT_KIND_SHOOT_OF_DELETED_CLUSTER,
			"The cluster is deleted but Gardener still lists its shoot. Reconcile deletes orphaned shoots, so this should clear by itself."
	case dbconst.DriftFindingKind_ClusterWithoutShoot:
		return fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT,
			"The cluster was synced to Gardener, but Gardener lists no shoot for it."
	case dbconst.DriftFindingKind_KubernetesVersionMismatch:
		return fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH,
			fmt.Sprintf("The database has Kubernetes %s, the shoot runs %s.", r.Expected.String, r.Observed.String)
	case dbconst.DriftFindingKind_ShootInvalidClusterId:
		return fleetv1.DriftKind_DRIFT_KIND_SHOOT_INVALID_CLUSTER_ID,
			fmt.Sprintf("Gardener has a shoot whose fundament.io/cluster-id label %q is no cluster id.", r.Observed.String)
	default:
		panic(fmt.Sprintf("unhandled drift kind %q", r.Kind))
	}
}
