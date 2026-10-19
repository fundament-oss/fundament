# Node pool provisioning status — design

Issue: https://gitlab.com/digilab.overheid.nl/miscellaneous/issues/-/work_items/1314
Branch: `fix-node-pool-provisioning-status`, stacked on PR #494 (`fix/shoot-status-freshness`).

## Problem

When the platform has no free machine for a new node pool, the pool is saved, the cluster
stays "Running"/"Ready", and the event history stops at "Node pool created". Gardener knows
exactly why (reconcile stuck, `EveryNodeReady: False` naming the pool's machine deployment),
but none of it reaches the user through console, functl, or events.

Root causes addressed here:

1. Ready clusters were never re-polled — **solved by PR #494** (fast/slow poll lanes,
   `shoot_health`, health events). Two gaps remain on top of #494:
   `EveryNodeReady` is not among the checked conditions, and the ready+`Reconcile` rule
   short-circuits without refreshing health, so a stuck worker reconcile freezes "healthy".
2. Node pool status derives from node metrics only (`node_pool_runtime.go`); a pool whose
   machines never came up shows "Unknown status", identical to a healthy pool with
   unreachable Prometheus.
3. No pre-acceptance capacity validation — **out of scope** (agreed): accept the pool, then
   monitor and report truthfully.

## Agreed scope

- Build on top of PR #494; our PR stays draft/blocked until #494 merges.
- E2E test runs against a real Gardener on demand (local-gardener or deploy-remote);
  scheduled CI run is a follow-up.
- functl gains node-pool output in `cluster get` (in scope); no new subcommands beyond that.
- Pool state is derived from the garden API only (Shoot status already in hand); no node
  listing via the shoot admin kubeconfig.
- "Updating"/"Unhealthy" are derived server-side in organization-api and served through the
  `ClusterStatus` enum so console, functl, and Terraform share one mapping.

## 1. cluster-worker: read what Gardener already reports

- `isShootHealthy` (`cluster-worker/pkg/client/gardener/real.go`) adds
  `gardencorev1beta1.ShootEveryNodeReady` to the required conditions.
- `ShootStatus` (client.go) carries, in addition to #494's fields: the `EveryNodeReady`
  condition message and the shoot's `.status.lastErrors` (code + description). Served from
  #494's watch-backed status cache; no extra Gardener requests.
- The #494 rule for ready+`Reconcile` polls changes: status and message still stick, but
  health and per-pool states are re-evaluated on every poll. A periodic reconcile on a
  healthy cluster still produces no event churn (health unchanged → no event).

## 2. Per-pool status, derived in the status poll

The status handler (`cluster-worker/pkg/handler/cluster/status.go`) loads the cluster's
active node pools (reusing the sync path's query) and classifies each pool:

| Stored status | When |
|---|---|
| `ready` | Shoot settled and nothing names the pool's machine deployment |
| `progressing` | A reconcile is running and nothing complains about this pool |
| `waiting` | `EveryNodeReady` message or `lastErrors` name the pool's machine deployment; Gardener's text is the reason |
| `error` | `lastOperation` is `Failed` and the failure names the pool |

Machine deployment names embed the pool name; matching uses the `-<pool-name>-z` fragment to
avoid prefix collisions (`web` vs `web2`), covered by unit tests.

Storage: three new columns on `tenant.node_pools` — `status text`, `status_message text`,
`status_updated timestamptz` — with a CHECK constraint for the values (no postgres enums).
Schema edited in `db/fundament.dbm`; migration 046 (on top of #494's 045) is generated with
trek by the user, per workflow. `db/fundament.sql` regenerated via pgmodeler
(`--pgsql-ver 18.0`).

Events: pool status transitions write `tenant.cluster_events` rows with new event types
`nodepool_waiting`, `nodepool_error`, `nodepool_ready` (added to the CHECK constraint;
`common/dbconst` regenerated). The message carries the pool name and Gardener's reason.
`progressing` records no event. Like #494's `status_warning`, a `waiting`/`error` event is
recorded once per distinct message, not per poll.

## 3. organization-api: one derivation for all clients

Proto (`organization-api/pkg/proto/v1`):

- `NodePool` gains `status_message`.
- `NodePoolStatus` gains `NODE_POOL_STATUS_PROVISIONING`, `NODE_POOL_STATUS_WAITING_FOR_MACHINES`,
  `NODE_POOL_STATUS_FAILED`.
- `ClusterStatus` gains `CLUSTER_STATUS_UNHEALTHY`.

Serving (`node_pool_get.go` / `node_pool_list.go`): while the stored pool status is not
`ready`, it wins over the metrics-derived runtime — `progressing` → PROVISIONING, `waiting` →
WAITING_FOR_MACHINES, `error` → FAILED, each with `status_message`. Once `ready`, the
existing metrics health (HEALTHY/DEGRADED/UNHEALTHY) takes over. NULL stored status (pools
created before this change) behaves as `ready`.

Cluster status (`cluster_convert.go`), evaluated for shoots stored `ready`:

1. `shoot_health = unhealthy` → `CLUSTER_STATUS_UNHEALTHY`.
2. else any active pool `progressing`/`waiting` → `CLUSTER_STATUS_UPGRADING` (console label
   becomes "Updating").
3. else `CLUSTER_STATUS_RUNNING`.

Consumers updated: console labels/badges, functl `formatClusterStatus`, Terraform provider
status handling (new enum value handled; exhaustive switches panic in default per
convention).

## 4. Console and functl

Console (`cluster-details`):

- Pool cards: labels/badges for "Creating machines" (PROVISIONING, progress color),
  "Waiting for machines" (warning) and "Failed" (critical), showing `status_message`.
- Event labels/colors for `nodepool_waiting` (warning), `nodepool_error` (critical),
  `nodepool_ready` (success).
- Cluster badge: "Updating" (moving), "Unhealthy" (critical).
- Use existing predefined classes from `console-frontend/src/styles.css` where applicable.

functl: `cluster get` output gains a node pools table — name, machine type, nodes
(current/min/max), status, reason.

## 5. Tests

- **Unit (cluster-worker):** extend the pure `nextShootStatus` table tests for the changed
  ready+Reconcile rule; new pool-classification tests with real-shaped `EveryNodeReady`
  messages and `lastErrors` fixtures, including the `-<pool>-z` prefix-collision case.
- **Integration (cluster-worker, embedded PG + mock Gardener):** extend the mock's
  `StatusOverride` with per-pool condition data; scenarios: ready cluster + stuck pool →
  pool `waiting` with reason, `nodepool_waiting` event written once per message, cluster
  health unhealthy; recovery → `nodepool_ready` event, health healthy.
- **Integration (organization-api):** pool status precedence (stored non-ready wins over
  metrics), cluster UNHEALTHY/UPGRADING derivation.
- **E2E (real Gardener only, manual):** new feature following the issue's repro — cordon the
  seed node, add pool via console, assert the pool shows waiting with Gardener's reason, the
  event appears, the cluster shows Updating/Unhealthy, functl reports it; uncordon, assert
  recovery. Tagged so the mock-Gardener PR preview run excludes it.

## 6. Risks and notes

- **Message parsing fragility:** pool attribution relies on machine deployment naming; on
  metal-stack (poc) the error text differs from the local simulation. Mitigation: when no
  pool can be attributed, the reason lands on cluster health (already surfaced by #494's
  events) rather than being dropped; parsing is isolated in one tested function.
- **EveryNodeReady flapping:** the condition may dip false briefly during a normal scale-up.
  If local testing shows churn, add a grace: only report unhealthy when the condition stays
  false while no reconcile progress is being made.
- **Stacking:** until #494 merges, this PR targets `fix/shoot-status-freshness` and rebases
  with it; migration numbering (046) assumes #494's 045 lands first.
- The pre-existing "Prometheus unreachable looks like Unknown status" collapse for *ready*
  pools is unchanged (separate bug).
