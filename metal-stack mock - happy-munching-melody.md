# Plan: Add `metalStackMode: mock` to cluster-worker

## Context

The organization-api serves workload and infra metrics from two Prometheus sources:
- `k8sPromClient` (K8s workload: `kube_*`, `container_*`)
- `metalPromClient` (metal-stack infra: `metal_machine_*`)

When neither is configured, both return empty data (via `StubClient`). For local development the metrics page shows nothing. The `gardenerMode: mock` pattern already exists as a reference for in-memory simulation.

The goal is: when `metalStackMode: mock` is set on the cluster-worker, it exposes a Prometheus-compatible HTTP API on its existing health port (8097). The organization-api `prometheusUrl` and `prometheusMetalUrl` are pointed at this endpoint, so all metrics pages show meaningful mock data derived from the actual clusters in the database.

---

## Architecture

```
cluster-worker (port 8097)
  ├── /healthz                  (existing)
  ├── /readyz                   (existing)
  ├── /api/v1/query             (NEW – only when metalStackMode=mock)
  └── /api/v1/query_range       (NEW – only when metalStackMode=mock)

organization-api
  ├── prometheusUrl     → http://fundament-cluster-worker:8097
  └── prometheusMetalUrl → http://fundament-cluster-worker:8097
```

The mock Prometheus handler generates metric values on the fly from node pool data stored in the gardener `MockClient`. It pattern-matches the PromQL query string (queries are hardcoded in metrics.go, so the full set is known) to return the right shape of data, with values that vary sinusoidally over time.

---

## Changes

### 1. `cluster-worker/pkg/gardener/mock.go`

Add a method to expose active cluster data (needed by mock Prometheus):

```go
// ListActiveClusters returns the ClusterToSync for each active (non-deleted) shoot.
func (m *MockClient) ListActiveClusters() []ClusterToSync
```

### 2. `cluster-worker/pkg/mockprom/handler.go` (NEW FILE)

`Handler` implements `http.Handler`, serving `/api/v1/query` and `/api/v1/query_range`.

- Receives a `func() []gardener.ClusterToSync` to get current clusters
- Converts each `ClusterToSync` to a set of mock nodes based on node pools:
  - Node count per pool: `(AutoscaleMin + AutoscaleMax) / 2` (minimum 1), varied with sin() over time
  - Default resources per node: **2 CPU cores, 4 GiB RAM, 110 max pods** (simple fixed values)
  - Node names: `<pool-name>-<index>`
  - Machine ID for metal-stack: deterministic UUID derived from `clusterID + nodeIndex`

**Instant query (`/api/v1/query`)**: pattern-match the query string to detect:

| Query pattern | Return shape | Value |
|---|---|---|
| `container_cpu_usage` | scalar or per-node | 30-50% of capacity, sin-varied |
| `kube_node_status_capacity{resource="cpu"}` | scalar or per-node | cores × nodes |
| `container_memory_working_set_bytes` | scalar or per-node | 30-50% of capacity |
| `kube_node_status_capacity{resource="memory"}` | scalar or per-node | GiB × nodes |
| `count(kube_pod_info)` | scalar or per-node | ~15 pods/node, sin-varied |
| `kube_node_status_capacity{resource="pods"}` | scalar or per-node | 110 × nodes |
| `kube_pod_container_resource_requests/limits` | per-namespace | derived from pods |
| `container_network_*_bytes_total` | per-namespace | constant ~10 MB/s |
| `metal_machine_allocation_info` | per-machine | labels: machineid, machinename, size, state, clusterTag |
| `metal_machine_power_usage` | per-machine | 150–250 W, sin-varied |

Cluster filter: extract `cluster="X"` from query to return data only for that cluster.
Namespace filter: extract `namespace=~"..."` and generate per-namespace data.
`by (node)` / `by (namespace)` / `by (cluster)`: detect from query to determine return shape.

**Range query (`/api/v1/query_range`)**: same logic, but loop over start→end with step, applying `sin(2π·t/3600)` for time variation.

JSON response format matches the Prometheus HTTP API:
```json
{"status":"success","data":{"resultType":"vector","result":[{"metric":{...},"value":[timestamp,"value"]}]}}
```

### 3. `cluster-worker/cmd/main.go`

Add to `config` struct:
```go
MetalStackMode string `env:"METAL_STACK_MODE"` // mock (empty = disabled)
```

In `run()`, after creating the health server mux, when `cfg.MetalStackMode == "mock"`:
- Assert `gardenerClient` is `*gardener.MockClient` (log a warning and skip if not)
- Create `mockprom.New(mockClient.ListActiveClusters)`
- Register on health mux: `healthMux.Handle("/api/v1/", mockPromHandler)`

Log a startup message: `"mock Prometheus endpoint enabled"`.

### 4. `charts/fundament/values.yaml`

Under `clusterWorker`, add:
```yaml
metalStackMode: mock  # mock (empty to disable mock Prometheus)
```

### 5. `charts/fundament/values-local.yaml`

Under `clusterWorker`, add:
```yaml
metalStackMode: mock
```

Under `organizationApi`, add:
```yaml
prometheusUrl: "http://fundament-cluster-worker:8097"
prometheusMetalUrl: "http://fundament-cluster-worker:8097"
```

### 6. `charts/fundament/templates/cluster-worker.yaml`

Add env var to the deployment container:
```yaml
- name: METAL_STACK_MODE
  value: {{ $.Values.clusterWorker.metalStackMode | default "" }}
```

Add a Kubernetes `Service` resource (needed so organization-api can reach the cluster-worker):
```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $.Release.Name }}-cluster-worker
spec:
  type: ClusterIP
  ports:
    - port: 8097
      targetPort: health
      name: health
  selector:
    app.kubernetes.io/component: cluster-worker
```

---

## Key Files

| File | Change type |
|---|---|
| `cluster-worker/pkg/gardener/mock.go` | Add `ListActiveClusters()` method |
| `cluster-worker/pkg/mockprom/handler.go` | **NEW** – mock Prometheus handler |
| `cluster-worker/cmd/main.go` | Add `MetalStackMode` config, wire handler |
| `charts/fundament/values.yaml` | Add `clusterWorker.metalStackMode` |
| `charts/fundament/values-local.yaml` | Enable mock mode + configure org-api URLs |
| `charts/fundament/templates/cluster-worker.yaml` | Add env var + Service |

---

## Verification

1. Deploy with `just helm-upgrade` or equivalent using `values-local.yaml`
2. Create a cluster with at least one node pool via the Console UI
3. Navigate to the Usage/Metrics page — should show non-zero CPU, memory, pod data
4. Wait a few minutes — values should change (sinusoidal variation)
5. Check infra metrics tab — should show machines with IDs, sizes, power readings
6. Delete the cluster — metrics for that cluster should disappear
