package organization

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// maxLogsResponseBytes is the transport ceiling for a logs response. The entry
// limit and the per-entry field cap already bound it; this is the backstop that
// does not depend on either being right.
const maxLogsResponseBytes = 64 << 20

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

// logClientForSource picks the backend for one request.
//
// Plugin workloads are ordinary pods and never reach Vali (valitail ships system
// components only), so they are read through the kube-api-proxy with the
// caller's own credentials — per-user Kubernetes authorization applies. The
// console models that choice as an explicit switch, so it arrives on the wire:
// deriving it by probing Vali's label values cost three round trips on every
// query and tail, and guessed wrong whenever the probe failed (silently, keeping
// Vali and returning empty results) or the namespace was merely quiet in the
// requested window.
//
// Resolving the source before touching Vali also means a cluster whose Vali is
// unreachable can still serve plugin logs, which is exactly when that fallback
// is worth having.
func (s *Server) logClientForSource(
	ctx context.Context,
	clusterID uuid.UUID,
	source organizationv1.LogSource,
	auth http.Header,
) (logs.Client, error) {
	if source != organizationv1.LogSource_LOG_SOURCE_PLUGIN {
		return s.logsClientFor(ctx, clusterID)
	}
	// "mock" means "talk to no real backend". Honour that for plugin logs too,
	// rather than forwarding the caller's live credentials to a real proxy while
	// the operator asked for generated data.
	if s.logsURL == "" || s.logsURL == "mock" {
		return s.logsClientFor(ctx, clusterID)
	}
	base := s.kubeProxyInternalURL()
	if base == "" {
		return nil, fmt.Errorf("plugin logs need a kube-api-proxy URL: %w", gardener.ErrNotFound)
	}
	return logs.NewKubeClient(base, auth), nil
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
		Levels:    req.GetLevels(),
		Limit:     logs.EffectiveLimit(int(req.GetLimit())),
	}
	if req.HasStart() {
		params.Start = req.GetStart().AsTime()
	}
	if req.HasEnd() {
		params.End = req.GetEnd().AsTime()
	}

	client, err := s.logClientForSource(ctx, clusterID, req.GetSource(), callerAuthHeaders(ctx))
	if err != nil {
		s.logLogsUnavailable(ctx, clusterID, err)
		return organizationv1.QueryLogsResponse_builder{
			Backend: organizationv1.LogBackend_LOG_BACKEND_NONE,
		}.Build(), nil
	}

	entries, err := client.Query(ctx, &params)
	if err != nil {
		if s.degradeLogError(ctx, clusterID, err) {
			return organizationv1.QueryLogsResponse_builder{
				Backend: organizationv1.LogBackend_LOG_BACKEND_NONE,
			}.Build(), nil
		}
		return nil, mapLogError(err)
	}

	// Enforce the level filter exactly. Backends only narrow their queries
	// approximately, so that the entry limit lands on a relevant set.
	entries = logs.FilterByLevels(entries, params.Levels)

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
		Levels:    req.GetLevels(),
	}
	wantLevels := logs.NormalizedLevels(params.Levels)

	client, err := s.logClientForSource(ctx, clusterID, req.GetSource(), callerAuthHeaders(ctx))
	if err != nil {
		s.logLogsUnavailable(ctx, clusterID, err)
		if errors.Is(err, gardener.ErrNotFound) {
			// Genuinely not provisioned yet: keep the stream open but silent
			// until the client disconnects, so the UI's live tail doesn't
			// error-loop against a cluster that is still coming up.
			<-ctx.Done()
			return nil
		}
		// Anything else is a backend we expected to work. Ending the stream lets
		// the client stop claiming to be live, rather than sitting on a socket
		// that will never carry an entry and never retries resolution.
		return mapLogError(err)
	}

	ch, err := client.Tail(ctx, &params)
	if err != nil {
		return mapLogError(err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			// A failed tail must not look like a quiet cluster: end the stream
			// with an error so the client stops claiming to be live and can
			// reconnect.
			if ev.Err != nil {
				s.logger.WarnContext(ctx, "log tail ended on backend error",
					"cluster_id", clusterID, "error", ev.Err)
				return mapLogError(ev.Err)
			}
			if wantLevels != nil && !wantLevels[logs.NormalizeLevel(ev.Entry.Level)] {
				continue
			}
			if err := stream.Send(toProtoEntry(&ev.Entry)); err != nil {
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
		if s.degradeLogError(ctx, clusterID, err) {
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

// degradeLogError decides whether a backend failure becomes an empty
// LOG_BACKEND_NONE response, and logs it at the level its kind deserves.
//
// ADR-0027 requires per-cluster failures to degrade rather than fail the RPC,
// but degrading *everything* made four different situations indistinguishable:
// "not provisioned", "unreachable", "misconfigured" and "unauthorized" all
// rendered indefinitely as "this cluster has no log backend", visible only in a
// warn log. Only the first two are properties of the cluster; the other two are
// faults that need to be seen.
func (s *Server) degradeLogError(ctx context.Context, clusterID uuid.UUID, err error) bool {
	switch classifyLogError(err) {
	case logErrorEnvironmental:
		s.logger.WarnContext(ctx, "logs backend unreachable, degrading",
			"cluster_id", clusterID, "error", err)
		return true
	case logErrorConfig:
		// Credentials or grants that no longer work, or a response we could not
		// parse — ADR-0027 notes an exhausted Vali path is how the VictoriaLogs
		// migration will announce itself. Surface it.
		s.logger.ErrorContext(ctx, "logs backend rejected or unparseable, surfacing",
			"cluster_id", clusterID, "error", err)
		return false
	case logErrorCaller, logErrorCanceled:
		return false
	default:
		panic(fmt.Sprintf("unhandled log error kind for %v", err))
	}
}

// logErrorKind classifies a log-backend failure by who has to act on it.
type logErrorKind int

const (
	// logErrorEnvironmental: the cluster has no reachable backend right now —
	// unreachable ingress, hibernated shoot, gateway error. Degrade.
	logErrorEnvironmental logErrorKind = iota
	// logErrorCaller: the request was bad. Return it to the caller.
	logErrorCaller
	// logErrorConfig: our configuration is wrong, or the backend is no longer
	// the shape we expect. Surface rather than hide behind an empty response.
	logErrorConfig
	// logErrorCanceled: the caller went away mid-flight.
	logErrorCanceled
)

func classifyLogError(err error) logErrorKind {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return logErrorCanceled
	case errors.Is(err, logs.ErrPodRequired):
		return logErrorCaller
	}

	var statusErr *logs.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return logErrorCaller
		case http.StatusUnauthorized, http.StatusForbidden:
			// A 401 that survived re-resolution means the monitoring secret is
			// genuinely wrong; a 403 means Plutono no longer grants the
			// datasource proxy — the assumption ADR-0027 rests on.
			return logErrorConfig
		default:
			return logErrorEnvironmental
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return logErrorEnvironmental
	}

	// Unclassified: a decode failure or an unexpected envelope. That is a
	// property of our integration, not of the cluster, so reporting an empty
	// success would be the same silent lie this classification exists to remove.
	return logErrorConfig
}
