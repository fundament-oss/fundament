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
	"golang.org/x/sync/singleflight"

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
// frontend (metal-from-proto.ts). Label names are the exporter's.
//
// Every query aggregates by the labels the page reads, and nothing else, for
// two reasons. Serials, management IPs and the scrape's own labels stay in the
// store. And the scrape labels (job, namespace, pod) are what tell two copies
// of one series apart: while an exporter pod is replaced, the old pod's series
// linger for the lookback window next to the new one's. Grouping them away
// first means a sum or a count sees each machine, interface or partition once.
var queries = []query{
	// Capacity per partition and size: the ten gauges, by __name__.
	{name: queryCapacity, expr: `max by (__name__, partition, size) ({__name__=~"metal_partition_capacity_(total|allocated|allocatable|waiting|phoned_home|faulty|unavailable|other|reservations_total|reservations_used)"})`},

	// Machines, joined on machineid in the frontend.
	{name: "machine_allocation", expr: `max by (machineid, partition, machinename, clusterTag, role, state, imageId) (metal_machine_allocation_info)`},
	{name: "machine_size", expr: `max by (machineid, size) (metal_machine_hardware_info)`},
	{name: "machine_power", expr: `max by (__name__, machineid) ({__name__=~"metal_machine_power_(usage|state|supplies_total|supplies_healthy)"})`},
	// The exporter publishes 0 or 1 for every known issue; only 1 is an issue.
	{name: "machine_issues", expr: `max by (machineid, issueid) (metal_machine_issues == 1)`},
	{name: "machine_issue_info", expr: `max by (issueid, severity, description, refurl) (metal_machine_issues_info)`},

	// Networks, joined on networkId.
	{name: "network_info", expr: `max by (networkId, name, partition) (metal_network_info)`},
	{name: "network_usage", expr: `max by (__name__, networkId) ({__name__=~"metal_network_(ip|prefix)_(used|available)"})`},

	// Switches, joined on switchname.
	{name: "switch_info", expr: `max by (switchname, partition, rackid, metalCoreVersion, osVendor, osVersion) (metal_switch_info)`},
	{name: "switch_state", expr: `max by (__name__, switchname) ({__name__=~"metal_switch_(metal_core_up|sync_failed)"})`},
	// BGP sessions per switch. The exporter publishes the timestamp only for an
	// interface that has a BGP port state, and 0 while the session is not
	// established; metal_switch_interface_info also lists interfaces without
	// BGP, so it would inflate the total.
	{name: "switch_bgp_total", expr: `count by (switchname) (max by (switchname, device) (metal_switch_interface_bgp_established_timestamp))`},
	{name: "switch_bgp_established", expr: `count by (switchname) (max by (switchname, device) (metal_switch_interface_bgp_established_timestamp) > 0)`},

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
	{name: "power", expr: `sum(max by (machineid) (metal_machine_power_usage))`, window: day, step: dayStep},
	{name: "sync_durations", expr: `max by (switchname) (metal_switch_sync_durations_ms)`, window: day, step: dayStep},
	{name: "request_rate", expr: `sum by (class) (label_replace(rate(metal_api_requests_total[5m]), "class", "${1}xx", "code", "([1-5])..")) * 60`, window: day, step: dayStep},
	{name: "allocation_allocated", expr: `sum(max by (partition, size) (metal_partition_capacity_allocated))`, window: week, step: weekStep},
	{name: "allocation_total", expr: `sum(max by (partition, size) (metal_partition_capacity_total))`, window: week, step: weekStep},
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
//
// It keeps the last snapshot for cacheFor and lets concurrent callers share
// one read: every DCIM page load asks for a snapshot (the sidebar badge reads
// it), and the series only change every minute or two.
type Reader struct {
	client prom.Client
	now    func() time.Time

	group singleflight.Group

	mu     sync.Mutex
	cached *Snapshot
}

// NewReader returns a Reader over client.
func NewReader(client prom.Client) *Reader {
	return &Reader{client: client, now: time.Now}
}

const (
	// maxConcurrent bounds the query burst against the store.
	maxConcurrent = 8

	// cacheFor is how long a snapshot is answered again rather than read anew.
	cacheFor = 30 * time.Second

	// readTimeout bounds one shared read. It runs detached from the caller
	// that started it, so the callers waiting on it are not failed by that
	// one going away.
	readTimeout = 30 * time.Second
)

// Snapshot returns the store's current snapshot: a cached one younger than
// cacheFor, or a new read shared with any caller already waiting for one.
// Any failing query fails the read: a page with half its panels silently
// empty reads as a healthy estate. It returns ErrNoData when the store holds
// no capacity series. Errors are not cached.
func (r *Reader) Snapshot(ctx context.Context) (*Snapshot, error) {
	if snap := r.fresh(); snap != nil {
		return snap, nil
	}
	ch := r.group.DoChan("snapshot", func() (any, error) {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), readTimeout)
		defer cancel()
		snap, err := r.read(readCtx)
		if err != nil {
			return nil, err
		}
		r.mu.Lock()
		r.cached = snap
		r.mu.Unlock()
		return snap, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err() //nolint:wrapcheck // the caller's own context error
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err //nolint:wrapcheck // wrapped in read
		}
		return res.Val.(*Snapshot), nil //nolint:forcetypeassert // only read stores into the group
	}
}

// fresh returns the cached snapshot while it is younger than cacheFor.
func (r *Reader) fresh() *Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached == nil || r.now().Sub(r.cached.CollectedAt) >= cacheFor {
		return nil
	}
	return r.cached
}

// read runs every query once.
func (r *Reader) read(ctx context.Context) (*Snapshot, error) {
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
	// End on a step boundary, so two reads a minute apart draw the same axis,
	// and start one step after end-window, so the window holds window/step
	// points: with both ends in, the first and last point of the 24 h charts
	// would share a clock label.
	end := now.Truncate(q.step)
	series, err := r.client.QueryRange(ctx, q.expr, end.Add(-q.window+q.step), end, q.step)
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
