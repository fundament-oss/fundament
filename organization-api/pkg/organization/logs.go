package organization

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/gardener"
	"github.com/fundament-oss/fundament/organization-api/pkg/logs"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// logsClientFor selects the log backend for one cluster:
//   - "per-shoot" → the cluster's Vali through its Plutono datasource proxy
//     (resolved from the monitoring secret, cached, self-healing);
//   - "mock" (or empty/unset) → generated data (local dev, CI);
//   - anything else → one global Loki-API backend at that URL, no auth.
//
// "per-shoot" is an explicit sentinel rather than the empty string because
// the env layer (caarlos0/env) substitutes envDefault for set-but-empty
// variables, silently collapsing "" into "mock" — same convention as
// PROMETHEUS_URL.
func (s *Server) logsClientFor(ctx context.Context, clusterID uuid.UUID) (logs.Client, error) {
	switch s.logsURL {
	case "per-shoot":
		return s.perShootLogs.clientFor(ctx, clusterID)
	case "", "mock":
		if s.mockLogsClient != nil {
			return s.mockLogsClient, nil
		}
		return logs.StubClient{}, nil
	default:
		return logs.NewLokiClient(s.logsURL, s.logsOpts...), nil
	}
}

// logLogsUnavailable records why a cluster has no logs backend. A missing
// shoot/monitoring stack is expected while a cluster is provisioning and logs
// at debug; an exhausted datasource probe or any other resolution failure is
// worth a warning (ADR-0027: probe exhaustion on a healthy shoot is the
// VictoriaLogs-migration signal).
func (s *Server) logLogsUnavailable(ctx context.Context, clusterID uuid.UUID, err error) {
	if errors.Is(err, gardener.ErrNotFound) {
		s.logger.DebugContext(ctx, "per-shoot logs not available yet", "cluster_id", clusterID)
		return
	}
	s.logger.WarnContext(ctx, "resolve per-shoot logs backend", "cluster_id", clusterID, "error", err)
}

// kubeProxyInternalURL is the kube-api-proxy base URL for server-side calls.
// KUBE_API_PROXY_URL is the browser-facing URL (also embedded in kubeconfigs
// handed to users) and is not necessarily routable from inside the cluster;
// KUBE_API_PROXY_INTERNAL_URL overrides it for org-api's own requests.
func (s *Server) kubeProxyInternalURL() string {
	if s.config.KubeAPIProxyInternalURL != "" {
		return s.config.KubeAPIProxyInternalURL
	}
	return s.config.KubeAPIProxyURL
}

// callerAuthHeaders extracts the caller's credential headers for forwarding to
// the kube-api-proxy: Authorization for token clients (functl), Cookie for
// the browser session. The proxy authenticates either and enforces per-user
// Kubernetes authorization.
func callerAuthHeaders(ctx context.Context) http.Header {
	out := http.Header{}
	info, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return out
	}
	h := info.RequestHeader()
	for _, name := range []string{"Authorization", "Cookie"} {
		if values := h.Values(name); len(values) > 0 {
			out[name] = values
		}
	}
	return out
}

// pluginPodClient reroutes a query pinned to a specific namespace+pod that the
// Vali-backed client does not cover: plugin pods are ordinary workloads and
// never reach Vali (valitail ships system components only), so they are read
// through the kube-api-proxy with the caller's own credentials — per-user
// Kubernetes authorization applies. Returns the original client when the
// request is not pod-scoped, the namespace is covered, or no proxy is
// configured.
func (s *Server) pluginPodClient(ctx context.Context, client logs.Client, params *logs.QueryParams, auth http.Header) logs.Client {
	if params.Namespace == "" || params.Pod == "" || s.kubeProxyInternalURL() == "" {
		return client
	}
	if client.Backend() != logs.BackendLoki {
		return client
	}
	labels, err := client.Labels(ctx, params.ClusterID, "", params.Start, params.End)
	if err != nil {
		return client
	}
	if slices.Contains(labels.Namespaces, params.Namespace) {
		return client
	}
	return logs.NewKubeClient(s.kubeProxyInternalURL(), auth)
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

	params := logs.QueryParams{
		ClusterID: clusterID.String(),
		Namespace: req.GetNamespace(),
		Pod:       req.GetPod(),
		Container: req.GetContainer(),
		Search:    req.GetSearch(),
		Limit:     logs.EffectiveLimit(int(req.GetLimit())),
	}
	if req.HasStart() {
		params.Start = req.GetStart().AsTime()
	}
	if req.HasEnd() {
		params.End = req.GetEnd().AsTime()
	}

	client, err := s.logsClientFor(ctx, clusterID)
	if err != nil {
		s.logLogsUnavailable(ctx, clusterID, err)
		return organizationv1.QueryLogsResponse_builder{
			Backend: organizationv1.LogBackend_LOG_BACKEND_NONE,
		}.Build(), nil
	}
	client = s.pluginPodClient(ctx, client, &params, callerAuthHeaders(ctx))

	entries, err := client.Query(ctx, &params)
	if err != nil {
		if isDegradableLogError(err) {
			s.logger.WarnContext(ctx, "logs backend unreachable, degrading", "cluster_id", clusterID, "error", err)
			return organizationv1.QueryLogsResponse_builder{
				Backend: organizationv1.LogBackend_LOG_BACKEND_NONE,
			}.Build(), nil
		}
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

	params := logs.QueryParams{
		ClusterID: clusterID.String(),
		Namespace: req.GetNamespace(),
		Pod:       req.GetPod(),
		Container: req.GetContainer(),
		Search:    req.GetSearch(),
	}

	client, err := s.logsClientFor(ctx, clusterID)
	if err != nil {
		// No backend: keep the stream open but silent until the client
		// disconnects, so the UI's live tail doesn't error-loop against a
		// provisioning cluster.
		s.logLogsUnavailable(ctx, clusterID, err)
		<-ctx.Done()
		return nil
	}
	client = s.pluginPodClient(ctx, client, &params, callerAuthHeaders(ctx))

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

	// Label values are time-scoped in Vali; default to a generous window so
	// dropdowns stay populated on quiet clusters.
	end := time.Now()
	start := end.Add(-24 * time.Hour)
	if req.HasStart() {
		start = req.GetStart().AsTime()
	}
	if req.HasEnd() {
		end = req.GetEnd().AsTime()
	}

	client, err := s.logsClientFor(ctx, clusterID)
	if err != nil {
		s.logLogsUnavailable(ctx, clusterID, err)
		return organizationv1.GetLogLabelsResponse_builder{
			Backend: organizationv1.LogBackend_LOG_BACKEND_NONE,
		}.Build(), nil
	}

	labels, err := client.Labels(ctx, clusterID.String(), req.GetNamespace(), start, end)
	if err != nil {
		if isDegradableLogError(err) {
			s.logger.WarnContext(ctx, "logs backend unreachable, degrading", "cluster_id", clusterID, "error", err)
			return organizationv1.GetLogLabelsResponse_builder{
				Backend: organizationv1.LogBackend_LOG_BACKEND_NONE,
			}.Build(), nil
		}
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

// isDegradableLogError reports whether a backend failure should degrade to an
// empty LOG_BACKEND_NONE response instead of failing the RPC: transport-level
// errors (unreachable ingress, hibernated shoot) and non-2xx statuses other
// than 400 (a 400 is a bad query and the caller should see it).
func isDegradableLogError(err error) bool {
	var statusErr *logs.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode != http.StatusBadRequest
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}
