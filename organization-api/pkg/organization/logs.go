package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// logsClient selects the log backend for a cluster. Mock-only for now: the
// per-shoot Vali resolution and the kube-api-proxy plugin-log path arrive with
// the LOGS_URL selector in later stack layers.
//
// authToken is the caller's bearer token, forwarded to the kube-api-proxy on
// the Kubernetes path so it can authorise the request.
func (s *Server) logsClient(_ context.Context, _ uuid.UUID, _ string) logs.Client {
	if s.mockLogsClient != nil {
		return s.mockLogsClient
	}
	return logs.StubClient{}
}

// QueryLogs returns a bounded set of log entries for a cluster.
func (s *Server) QueryLogs(
	ctx context.Context,
	req *organizationv1.QueryLogsRequest,
) (*organizationv1.QueryLogsResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}
	if err := s.assertClusterExists(ctx, clusterID); err != nil {
		return nil, err
	}

	client := s.logsClient(ctx, clusterID, bearerToken(ctx))
	params := logs.QueryParams{
		ClusterID: clusterID.String(),
		Namespace: req.GetNamespace(),
		Pod:       req.GetPod(),
		Container: req.GetContainer(),
		Search:    req.GetSearch(),
		Limit:     int(req.GetLimit()),
	}
	if req.HasStart() {
		params.Start = req.GetStart().AsTime()
	}
	if req.HasEnd() {
		params.End = req.GetEnd().AsTime()
	}

	entries, err := client.Query(ctx, &params)
	if err != nil {
		return nil, mapLogError(err)
	}

	return organizationv1.QueryLogsResponse_builder{
		Entries: toProtoEntries(entries),
		Backend: toProtoBackend(client.Backend()),
	}.Build(), nil
}

// TailLogs streams new log entries until the client disconnects.
func (s *Server) TailLogs(
	ctx context.Context,
	req *organizationv1.TailLogsRequest,
	stream *connect.ServerStream[organizationv1.LogEntry],
) error {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Cluster(clusterID)); err != nil {
		return err
	}
	if err := s.assertClusterExists(ctx, clusterID); err != nil {
		return err
	}

	client := s.logsClient(ctx, clusterID, bearerToken(ctx))
	params := logs.QueryParams{
		ClusterID: clusterID.String(),
		Namespace: req.GetNamespace(),
		Pod:       req.GetPod(),
		Container: req.GetContainer(),
		Search:    req.GetSearch(),
	}

	ch, err := client.Tail(ctx, &params)
	if err != nil {
		return mapLogError(err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case entry, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(toProtoEntry(&entry)); err != nil {
				return fmt.Errorf("send log entry: %w", err)
			}
		}
	}
}

// GetLogLabels returns the distinct filter values available for a cluster.
func (s *Server) GetLogLabels(
	ctx context.Context,
	req *organizationv1.GetLogLabelsRequest,
) (*organizationv1.GetLogLabelsResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}
	if err := s.assertClusterExists(ctx, clusterID); err != nil {
		return nil, err
	}

	client := s.logsClient(ctx, clusterID, bearerToken(ctx))
	labels, err := client.Labels(ctx, clusterID.String(), req.GetNamespace())
	if err != nil {
		return nil, mapLogError(err)
	}

	return organizationv1.GetLogLabelsResponse_builder{
		Namespaces: labels.Namespaces,
		Pods:       labels.Pods,
		Containers: labels.Containers,
		Backend:    toProtoBackend(client.Backend()),
	}.Build(), nil
}

func (s *Server) assertClusterExists(ctx context.Context, clusterID uuid.UUID) error {
	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("get cluster: %w", err))
	}
	return nil
}

func bearerToken(ctx context.Context) string {
	const prefix = "Bearer "
	info, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return ""
	}
	v := info.RequestHeader().Get("Authorization")
	if len(v) > len(prefix) && strings.EqualFold(v[:len(prefix)], prefix) {
		return v[len(prefix):]
	}
	return ""
}

func toProtoEntry(e *logs.Entry) *organizationv1.LogEntry {
	return organizationv1.LogEntry_builder{
		Timestamp: timestamppb.New(e.Timestamp),
		Level:     e.Level,
		Cluster:   e.Cluster,
		Namespace: e.Namespace,
		Pod:       e.Pod,
		Container: e.Container,
		Message:   e.Message,
		Fields:    e.Fields,
	}.Build()
}

func toProtoEntries(entries []logs.Entry) []*organizationv1.LogEntry {
	out := make([]*organizationv1.LogEntry, 0, len(entries))
	for i := range entries {
		out = append(out, toProtoEntry(&entries[i]))
	}
	return out
}

func toProtoBackend(b logs.Backend) organizationv1.LogBackend {
	switch b {
	case logs.BackendLoki:
		return organizationv1.LogBackend_LOG_BACKEND_LOKI
	case logs.BackendKubernetes:
		return organizationv1.LogBackend_LOG_BACKEND_KUBERNETES
	case logs.BackendNone:
		return organizationv1.LogBackend_LOG_BACKEND_NONE
	default:
		panic(fmt.Sprintf("unhandled log backend %q", b))
	}
}

func mapLogError(err error) error {
	if errors.Is(err, logs.ErrPodRequired) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}
