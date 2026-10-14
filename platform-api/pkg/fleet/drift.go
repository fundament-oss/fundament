package fleet

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

func (s *Server) ListDriftFindings(ctx context.Context, _ *fleetv1.ListDriftFindingsRequest) (*fleetv1.ListDriftFindingsResponse, error) {
	findings, err := s.driftFindings(ctx)
	if err != nil {
		return nil, err
	}
	return fleetv1.ListDriftFindingsResponse_builder{Findings: findings}.Build(), nil
}

// driftFindings runs one query per kind and returns them ordered by kind, then
// oldest first.
func (s *Server) driftFindings(ctx context.Context) ([]*fleetv1.DriftFinding, error) {
	staleBefore := pgtype.Timestamptz{Time: s.staleBefore(), Valid: true}
	var findings []*fleetv1.DriftFinding

	for _, kind := range []fleetv1.DriftKind{
		fleetv1.DriftKind_DRIFT_KIND_SHOOT_WITHOUT_CLUSTER,
		fleetv1.DriftKind_DRIFT_KIND_SHOOT_OF_DELETED_CLUSTER,
		fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT,
		fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE,
		fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH,
	} {
		found, err := s.driftOfKind(ctx, kind, staleBefore)
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}

	slices.SortStableFunc(findings, func(a, b *fleetv1.DriftFinding) int {
		return cmp.Compare(a.GetKind(), b.GetKind())
	})
	return findings, nil
}

func (s *Server) driftOfKind(ctx context.Context, kind fleetv1.DriftKind, staleBefore pgtype.Timestamptz) ([]*fleetv1.DriftFinding, error) {
	var findings []*fleetv1.DriftFinding

	switch kind {
	case fleetv1.DriftKind_DRIFT_KIND_SHOOT_WITHOUT_CLUSTER:
		rows, err := s.queries.DriftShootWithoutCluster(ctx)
		if err != nil {
			return nil, fmt.Errorf("drift %v: %w", kind, err)
		}
		for i := range rows {
			r := &rows[i]
			findings = append(findings, fleetv1.DriftFinding_builder{
				Kind:           kind,
				ClusterId:      r.ClusterID.String(),
				ShootName:      &r.ShootName,
				ShootNamespace: &r.ShootNamespace,
				Since:          timestampOrNil(r.FirstSeen),
				Detail:         "Gardener has a shoot whose cluster id matches no cluster.",
			}.Build())
		}

	case fleetv1.DriftKind_DRIFT_KIND_SHOOT_OF_DELETED_CLUSTER:
		rows, err := s.queries.DriftShootOfDeletedCluster(ctx)
		if err != nil {
			return nil, fmt.Errorf("drift %v: %w", kind, err)
		}
		for i := range rows {
			r := &rows[i]
			findings = append(findings, fleetv1.DriftFinding_builder{
				Kind:             kind,
				OrganizationId:   new(r.OrganizationID.String()),
				OrganizationName: &r.OrganizationName,
				ClusterId:        r.ClusterID.String(),
				ClusterName:      &r.ClusterName,
				ShootName:        &r.ShootName,
				ShootNamespace:   &r.ShootNamespace,
				Since:            timestampOrNil(r.Since),
				Detail:           "The cluster is deleted but Gardener still lists its shoot. Reconcile deletes orphaned shoots, so this should clear by itself.",
			}.Build())
		}

	case fleetv1.DriftKind_DRIFT_KIND_CLUSTER_WITHOUT_SHOOT:
		rows, err := s.queries.DriftClusterWithoutShoot(ctx, db.DriftClusterWithoutShootParams{FreshAfter: staleBefore})
		if err != nil {
			return nil, fmt.Errorf("drift %v: %w", kind, err)
		}
		for i := range rows {
			r := &rows[i]
			findings = append(findings, fleetv1.DriftFinding_builder{
				Kind:             kind,
				OrganizationId:   new(r.OrganizationID.String()),
				OrganizationName: &r.OrganizationName,
				ClusterId:        r.ClusterID.String(),
				ClusterName:      &r.ClusterName,
				Since:            timestampOrNil(r.Since),
				Detail:           "The cluster was synced to Gardener, but Gardener lists no shoot for it.",
			}.Build())
		}

	case fleetv1.DriftKind_DRIFT_KIND_SNAPSHOT_STALE:
		rows, err := s.queries.DriftSnapshotStale(ctx, db.DriftSnapshotStaleParams{StaleBefore: staleBefore})
		if err != nil {
			return nil, fmt.Errorf("drift %v: %w", kind, err)
		}
		for i := range rows {
			r := &rows[i]
			detail := "Never collected."
			if r.Observed.Valid {
				detail = "Last collected " + r.Observed.Time.UTC().Format(time.RFC3339) + "."
			}
			if r.LastError.Valid {
				detail += " Last error: " + r.LastError.String
			}
			findings = append(findings, fleetv1.DriftFinding_builder{
				Kind:             kind,
				OrganizationId:   new(r.OrganizationID.String()),
				OrganizationName: &r.OrganizationName,
				ClusterId:        r.ClusterID.String(),
				ClusterName:      &r.ClusterName,
				Since:            timestampOrNil(r.Since),
				Detail:           detail,
			}.Build())
		}

	case fleetv1.DriftKind_DRIFT_KIND_KUBERNETES_VERSION_MISMATCH:
		rows, err := s.queries.DriftKubernetesVersionMismatch(ctx)
		if err != nil {
			return nil, fmt.Errorf("drift %v: %w", kind, err)
		}
		for i := range rows {
			r := &rows[i]
			findings = append(findings, fleetv1.DriftFinding_builder{
				Kind:             kind,
				OrganizationId:   new(r.OrganizationID.String()),
				OrganizationName: &r.OrganizationName,
				ClusterId:        r.ClusterID.String(),
				ClusterName:      &r.ClusterName,
				ShootName:        &r.ShootName,
				ShootNamespace:   &r.ShootNamespace,
				Since:            timestampOrNil(r.Since),
				Detail:           fmt.Sprintf("The database has Kubernetes %s, the shoot runs %s.", r.KubernetesVersion, r.ShootKubernetesVersion),
			}.Build())
		}

	default:
		panic(fmt.Sprintf("unhandled drift kind %v", kind))
	}

	return findings, nil
}
