package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/dcim-api/pkg/metal"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

func (s *Server) GetMetalSnapshot(
	ctx context.Context,
	_ *dcimv1.GetMetalSnapshotRequest,
) (*dcimv1.GetMetalSnapshotResponse, error) {
	if s.metal == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("no metal-stack metrics configured"))
	}

	snapshot, err := s.metal.Snapshot(ctx)
	if err != nil {
		if metal.IsBadQuery(err) {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("metal-stack query rejected: %w", err))
		}
		s.logger.WarnContext(ctx, "metal-stack metrics unavailable", "error", err)
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("metal-stack metrics are unavailable"))
	}

	results := make(map[string]*dcimv1.MetalQueryResult, len(snapshot.Results))
	for name, res := range snapshot.Results {
		results[name] = metalResultToProto(res)
	}

	return dcimv1.GetMetalSnapshotResponse_builder{
		CollectedAt: timestamppb.New(snapshot.CollectedAt),
		Results:     results,
	}.Build(), nil
}

func metalResultToProto(res metal.Result) *dcimv1.MetalQueryResult {
	samples := make([]*dcimv1.MetalSample, len(res.Samples))
	for i, sample := range res.Samples {
		samples[i] = dcimv1.MetalSample_builder{Labels: sample.Labels, Value: sample.Value}.Build()
	}
	series := make([]*dcimv1.MetalSeries, len(res.Series))
	for i, ts := range res.Series {
		series[i] = dcimv1.MetalSeries_builder{Labels: ts.Labels, Points: metalPointsToProto(ts.Samples)}.Build()
	}
	return dcimv1.MetalQueryResult_builder{Samples: samples, Series: series}.Build()
}

func metalPointsToProto(points []prom.DataPoint) []*dcimv1.MetalPoint {
	out := make([]*dcimv1.MetalPoint, len(points))
	for i, p := range points {
		out[i] = dcimv1.MetalPoint_builder{Time: timestamppb.New(p.Time), Value: p.Value}.Build()
	}
	return out
}
