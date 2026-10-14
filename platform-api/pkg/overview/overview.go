package overview

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	platformv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/platform/v1"
)

func (s *Server) GetPlatformOverview(ctx context.Context, _ *platformv1.GetPlatformOverviewRequest) (*platformv1.GetPlatformOverviewResponse, error) {
	resp, err := s.overview(ctx)
	if err != nil {
		return nil, err
	}

	// Browsers and proxies may keep it only for what is left of the TTL, so
	// the TTL stays the bound on how stale the counts are.
	if callInfo, ok := connect.CallInfoForHandlerContext(ctx); ok {
		remaining := max(s.ttl-s.clock().Sub(resp.GetGeneratedAt().AsTime()), 0)
		callInfo.ResponseHeader().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(math.Ceil(remaining.Seconds()))))
	}

	return resp, nil
}

// overview returns the cached response while it is younger than the TTL and
// otherwise reads once for every caller waiting at that moment.
func (s *Server) overview(ctx context.Context) (*platformv1.GetPlatformOverviewResponse, error) {
	s.mu.Lock()
	if s.cached != nil && s.clock().Sub(s.cachedAt) < s.ttl {
		cached := s.cached
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	// The read runs detached from the first caller's context, so that caller
	// hanging up does not fail everyone sharing the flight. It has a deadline
	// of its own: a stalled query would otherwise hold the flight, and so
	// every later caller, indefinitely.
	ch := s.group.DoChan("overview", func() (any, error) {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), readTimeout)
		defer cancel()
		resp, err := s.read(readCtx)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.cached = resp
		s.cachedAt = s.clock()
		s.mu.Unlock()
		return resp, nil
	})

	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, ctx.Err())
	case res := <-ch:
		if res.Err != nil {
			// The error names the database; it goes to the log, not the caller.
			s.logger.ErrorContext(ctx, "failed to read platform overview", "error", res.Err)
			return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("platform overview unavailable"))
		}
		resp, _ := res.Val.(*platformv1.GetPlatformOverviewResponse)
		return resp, nil
	}
}

// read runs tenant.platform_overview(). sqlc cannot infer the columns of a
// table function, so the rows are scanned here.
func (s *Server) read(ctx context.Context) (*platformv1.GetPlatformOverviewResponse, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT organization_name, organization_alias, clusters, projects, namespaces
		FROM tenant.platform_overview()`)
	if err != nil {
		return nil, fmt.Errorf("query platform overview: %w", err)
	}
	defer rows.Close()

	totals := platformv1.PlatformTotals_builder{}
	var organizations []*platformv1.OrganizationOverview
	for rows.Next() {
		var (
			name, alias                    string
			clusters, projects, namespaces int64
		)
		if err := rows.Scan(&name, &alias, &clusters, &projects, &namespaces); err != nil {
			return nil, fmt.Errorf("scan platform overview: %w", err)
		}
		organizations = append(organizations, platformv1.OrganizationOverview_builder{
			Name:       name,
			Alias:      alias,
			Clusters:   int32(clusters),   //nolint:gosec // counts of one organization's objects
			Projects:   int32(projects),   //nolint:gosec // counts of one organization's objects
			Namespaces: int32(namespaces), //nolint:gosec // counts of one organization's objects
		}.Build())
		totals.Clusters += int32(clusters)     //nolint:gosec // platform-wide counts stay far below 2^31
		totals.Projects += int32(projects)     //nolint:gosec // platform-wide counts stay far below 2^31
		totals.Namespaces += int32(namespaces) //nolint:gosec // platform-wide counts stay far below 2^31
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read platform overview: %w", err)
	}
	totals.Organizations = int32(len(organizations)) //nolint:gosec // platform-wide counts stay far below 2^31

	return platformv1.GetPlatformOverviewResponse_builder{
		Totals:        totals.Build(),
		Organizations: organizations,
		GeneratedAt:   timestamppb.New(s.clock()),
	}.Build(), nil
}
