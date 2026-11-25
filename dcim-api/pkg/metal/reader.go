// Package metal reads what metal-stack's exporters publish from a
// Prometheus-compatible store, for DCIM's Bare metal section.
//
// The reader runs a fixed set of named PromQL queries and hands the raw
// results back by name. It does not join them: the frontend's types are shaped
// after the series' labels, so the join happens once, there.
package metal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
)

// ErrNoData means the store answered but holds no metal-metrics-exporter
// series: the exporter is not deployed, not scraped or not yet reporting.
var ErrNoData = errors.New("no metal-stack series in the store")

// query is one named PromQL expression. A zero window makes it an instant
// query; otherwise it is a range query over window, at step.
type query struct {
	name   string
	expr   string
	window time.Duration
	step   time.Duration
}

const (
	day  = 24 * time.Hour
	week = 7 * day

	// The 24 h charts at 5 min (288 points) and the 7 d chart at 1 h (168
	// points): what the charts' category axes and the demo fixtures draw.
	dayStep  = 5 * time.Minute
	weekStep = time.Hour

	// queryCapacity is the query whose emptiness means there is nothing to show.
	queryCapacity = "capacity"
)

// queries is everything the section shows. Names are the contract with the
// frontend (metal-from-proto.ts). Label names are the exporter's; aggregations
// keep only the labels the page reads, so serials and management IPs stay in
// the store.
var queries = []query{
	// Capacity per partition and size: the ten gauges, by __name__.
	{name: queryCapacity, expr: `{__name__=~"metal_partition_capacity_(total|allocated|allocatable|waiting|phoned_home|faulty|unavailable|other|reservations_total|reservations_used)"}`},

	// Machines, joined on machineid in the frontend.
	{name: "machine_allocation", expr: `max by (machineid, partition, machinename, clusterTag, role, state, imageId) (metal_machine_allocation_info)`},
	{name: "machine_size", expr: `max by (machineid, size) (metal_machine_hardware_info)`},
	{name: "machine_power", expr: `{__name__=~"metal_machine_power_(usage|state|supplies_total|supplies_healthy)"}`},
	// The exporter publishes 0 or 1 for every known issue; only 1 is an issue.
	{name: "machine_issues", expr: `max by (machineid, issueid) (metal_machine_issues == 1)`},
	{name: "machine_issue_info", expr: `max by (issueid, severity, description, refurl) (metal_machine_issues_info)`},

	// Networks, joined on networkId.
	{name: "network_info", expr: `max by (networkId, name, partition) (metal_network_info)`},
	{name: "network_usage", expr: `{__name__=~"metal_network_(ip|prefix)_(used|available)"}`},

	// Switches, joined on switchname.
	{name: "switch_info", expr: `max by (switchname, partition, rackid, metalCoreVersion, osVendor, osVersion) (metal_switch_info)`},
	{name: "switch_state", expr: `{__name__=~"metal_switch_(metal_core_up|sync_failed)"}`},
	{name: "switch_bgp_total", expr: `count by (switchname) (metal_switch_interface_info)`},
	// The timestamp is 0 while a session is not established.
	{name: "switch_bgp_established", expr: `count by (switchname) (metal_switch_interface_bgp_established_timestamp > 0)`},

	// Control-plane components, joined on uuid.
	{name: "component_up", expr: `max by (uuid, type, identifier) (metal_component_up)`},
	{name: "component_token", expr: `min by (uuid) (metal_component_token_lifetime_seconds)`},

	// metal-api latency over 24 h: cumulative buckets, sum and count.
	{name: "rest_latency_buckets", expr: `sum by (le) (increase(metal_api_request_duration_seconds_bucket[24h]))`},
	{name: "rest_latency_sum", expr: `sum(increase(metal_api_request_duration_seconds_sum[24h]))`},
	{name: "rest_latency_count", expr: `sum(increase(metal_api_request_duration_seconds_count[24h]))`},
	{name: "grpc_latency_buckets", expr: `sum by (le) (increase(metal_api_grpc_request_duration_seconds_bucket[24h]))`},
	{name: "grpc_latency_sum", expr: `sum(increase(metal_api_grpc_request_duration_seconds_sum[24h]))`},
	{name: "grpc_latency_count", expr: `sum(increase(metal_api_grpc_request_duration_seconds_count[24h]))`},

	// metal-core errors by operation, from the partition collectors (plan
	// steps 8-9). Empty until then.
	{name: "core_errors", expr: `sum by (operation) (increase(metal_core_total_errors[24h]))`},

	// Over time.
	{name: "power", expr: `sum(metal_machine_power_usage)`, window: day, step: dayStep},
	{name: "sync_durations", expr: `max by (switchname) (metal_switch_sync_durations_ms)`, window: day, step: dayStep},
	{name: "request_rate", expr: `sum by (class) (label_replace(rate(metal_api_requests_total[5m]), "class", "${1}xx", "code", "([1-5])..")) * 60`, window: day, step: dayStep},
	{name: "allocation_allocated", expr: `sum(metal_partition_capacity_allocated)`, window: week, step: weekStep},
	{name: "allocation_total", expr: `sum(metal_partition_capacity_total)`, window: week, step: weekStep},
}

// Result is the answer to one query: Samples for an instant query, Series for
// a range query.
type Result struct {
	Samples []prom.Sample
	Series  []prom.TimeSeries
}

// Snapshot is every query's result, read at one moment.
type Snapshot struct {
	CollectedAt time.Time
	Results     map[string]Result
}

// Reader runs the queries against one store.
type Reader struct {
	client prom.Client
	now    func() time.Time
}

// NewReader returns a Reader over client.
func NewReader(client prom.Client) *Reader {
	return &Reader{client: client, now: time.Now}
}

// maxConcurrent bounds the query burst against the store.
const maxConcurrent = 8

// Snapshot runs every query. Any failing query fails the snapshot: a page
// with half its panels silently empty reads as a healthy estate. It returns
// ErrNoData when the store holds no capacity series.
func (r *Reader) Snapshot(ctx context.Context) (*Snapshot, error) {
	now := r.now()
	results := make(map[string]Result, len(queries))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)
	for _, q := range queries {
		g.Go(func() error {
			res, err := r.run(gctx, q, now)
			if err != nil {
				return fmt.Errorf("query %s: %w", q.name, err)
			}
			mu.Lock()
			results[q.name] = res
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // already wrapped per query
	}

	if len(results[queryCapacity].Samples) == 0 {
		return nil, ErrNoData
	}
	return &Snapshot{CollectedAt: now, Results: results}, nil
}

func (r *Reader) run(ctx context.Context, q query, now time.Time) (Result, error) {
	if q.window == 0 {
		samples, err := r.client.Query(ctx, q.expr, now)
		if err != nil {
			return Result{}, fmt.Errorf("instant: %w", err)
		}
		return Result{Samples: samples}, nil
	}
	// End on a step boundary, so two reads a minute apart draw the same axis.
	end := now.Truncate(q.step)
	series, err := r.client.QueryRange(ctx, q.expr, end.Add(-q.window), end, q.step)
	if err != nil {
		return Result{}, fmt.Errorf("range: %w", err)
	}
	return Result{Series: series}, nil
}

// IsBadQuery reports whether err is the store rejecting one of our queries
// (HTTP 400 malformed, 422 unexecutable): a bug in the PromQL above, which
// must surface rather than read as "metal-stack is unreachable". Anything else
// is environmental. Same rule as organization-api's isPromBadQuery.
func IsBadQuery(err error) bool {
	var statusErr *prom.StatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusBadRequest || statusErr.StatusCode == http.StatusUnprocessableEntity
}
