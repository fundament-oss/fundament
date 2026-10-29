package fleet

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

// organizationKey is where a page of organizations ends.
type organizationKey struct {
	Alias string    `json:"a"`
	Name  string    `json:"n"`
	ID    uuid.UUID `json:"i"`
}

func (s *Server) ListOrganizations(ctx context.Context, req *fleetv1.ListOrganizationsRequest) (*fleetv1.ListOrganizationsResponse, error) {
	size := pageSize(req.GetPageSize())
	// One more than the page, to tell whether another follows.
	params := db.OrganizationListParams{PageLimit: size + 1}
	var after organizationKey
	ok, err := decodePageToken(req.GetPageToken(), &after)
	if err != nil {
		return nil, err
	}
	if ok {
		params.AfterAlias = pgtype.Text{String: after.Alias, Valid: true}
		params.AfterName = pgtype.Text{String: after.Name, Valid: true}
		params.AfterID = pgtype.UUID{Bytes: after.ID, Valid: true}
	}

	rows, err := s.queries.OrganizationList(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	resp := fleetv1.ListOrganizationsResponse_builder{}
	if len(rows) > int(size) {
		rows = rows[:size]
		last := &rows[len(rows)-1]
		resp.NextPageToken = encodePageToken(organizationKey{Alias: last.Alias, Name: last.Name, ID: last.ID})
	}

	resp.Organizations, err = s.organizationSummaries(ctx, rows)
	if err != nil {
		return nil, err
	}
	return resp.Build(), nil
}

func (s *Server) GetOrganization(ctx context.Context, req *fleetv1.GetOrganizationRequest) (*fleetv1.GetOrganizationResponse, error) {
	id := uuid.MustParse(req.GetOrganizationId())

	rows, err := s.queries.OrganizationList(ctx, db.OrganizationListParams{
		OrganizationID: pgtype.UUID{Bytes: id, Valid: true},
		PageLimit:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	if len(rows) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("organization not found"))
	}

	organizations, err := s.organizationSummaries(ctx, rows)
	if err != nil {
		return nil, err
	}
	return fleetv1.GetOrganizationResponse_builder{Organization: organizations[0]}.Build(), nil
}

// organizationSummaries adds how many clusters of each organization need
// attention, derived per cluster the way the console does. The status query
// is limited to this page's organizations, so reading every page reads each
// live cluster once; with GetFleetSummary's own read, a full DCIM refresh
// derives every status twice. That is cheap next to the snapshot joins, and
// is left uncached so the counts always agree with the cluster list.
func (s *Server) organizationSummaries(ctx context.Context, rows []db.OrganizationListRow) ([]*fleetv1.OrganizationSummary, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	inputs, err := s.queries.ClusterStatusInputList(ctx, db.ClusterStatusInputListParams{OrganizationIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list cluster status: %w", err)
	}
	unhealthy := make(map[uuid.UUID]int32, len(rows))
	for i := range inputs {
		if clusterstatusOf(&inputs[i]).NeedsAttention() {
			unhealthy[inputs[i].OrganizationID]++
		}
	}

	organizations := make([]*fleetv1.OrganizationSummary, 0, len(rows))
	for i := range rows {
		organizations = append(organizations, organizationSummaryFrom(&rows[i], unhealthy[rows[i].ID]))
	}
	return organizations, nil
}
