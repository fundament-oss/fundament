# cluster-worker

A background worker service that synchronizes cluster state from PostgreSQL to Gardener by creating, updating, and deleting Shoot cluster manifests.

## What

The cluster-worker watches for changes to the `organization.clusters` table and ensures that each cluster has a corresponding Shoot manifest in Gardener. It handles:

- **Creation**: When a new cluster is added to the database, create a Shoot in Gardener
- **Updates**: When cluster configuration changes, update the Shoot (future scope)
- **Deletion**: When a cluster is soft-deleted, delete the Shoot from Gardener

The worker also monitors Gardener to track the reconciliation status of each Shoot (pending, progressing, ready, error) and stores this in the `shoot_status` column.

## Why

### Why not sync directly from the API?

Synchronous API calls to Gardener would make the user-facing API slow and fragile. Gardener operations can take minutes. By decoupling via a background worker:

- API responses are fast (just database writes)
- Retries happen automatically without user intervention
- Multiple workers can process clusters in parallel
- The system is resilient to Gardener downtime

### Why PostgreSQL LISTEN/NOTIFY?

We use PostgreSQL's built-in pub/sub mechanism instead of a separate message queue (Redis, RabbitMQ, Kafka) because:

1. **No additional infrastructure** - PostgreSQL is already required
2. **Transactional guarantees** - Notifications are sent only when transactions commit
3. **Proven at scale** - This pattern handles hundreds of thousands of syncs per day at production systems like Printeers
4. **Simplicity** - One less system to operate, monitor, and secure

### Why SKIP LOCKED?

The `SELECT ... FOR UPDATE SKIP LOCKED` pattern enables multiple workers to process clusters concurrently without conflicts:

- Workers grab available work without blocking each other
- No risk of processing the same cluster twice
- Natural load distribution across workers
- No coordinator needed

### Why a separate status poller?

Gardener Shoot reconciliation is asynchronous - applying a manifest returns immediately, but the actual cluster creation takes minutes. A separate goroutine polls Gardener for status updates because:

- The main sync loop stays fast (just applies manifests)
- Users can see `shoot_status` to know if their cluster is actually ready
- We can detect and alert on failed reconciliations
- Deletion verification confirms Shoots are actually gone

## How

### Sync Flow

1. **Trigger**: A database trigger sets `synced = NULL` when a cluster needs syncing (insert, update, or soft-delete)
2. **Notify**: The trigger sends `NOTIFY cluster_sync` to wake up workers
3. **Claim**: Worker runs `SELECT ... FOR UPDATE SKIP LOCKED` to claim one cluster
4. **Sync**: Worker applies or deletes the Shoot manifest in Gardener
5. **Mark**: Worker sets `synced = now()` on success, or records error and increments `sync_attempts` on failure
6. **Repeat**: Worker processes next pending cluster

### Backoff Strategy

Failed syncs use exponential backoff to avoid hammering Gardener:

| Attempt | Backoff |
|---------|---------|
| 1 | 30s |
| 2 | 1m |
| 3 | 2m |
| 4 | 4m |
| 5 | 8m |
| 6+ | 15m (cap) |

After 5 consecutive failures, an alert is logged.

### Database Schema

Sync state is stored in a separate `cluster_sync` table (1:1 with clusters):

```sql
-- organization.cluster_sync (separate from clusters table)
cluster_id uuid PRIMARY KEY  -- FK to clusters.id, CASCADE delete
synced timestamptz           -- NULL = needs sync, timestamp = last successful sync
sync_error text              -- Last error message (NULL if no error)
sync_attempts int            -- Consecutive failed attempts (reset on success)
sync_last_attempt timestamptz-- Timestamp of last attempt (for backoff)
shoot_status text            -- Gardener status: pending, progressing, ready, error, deleting, deleted
shoot_status_message text    -- Last status message from Gardener
shoot_status_updated timestamptz -- Timestamp of last status check
```

This separation provides:
- Clean separation of concerns (cluster definition vs. sync state)
- Less write contention (sync updates don't touch cluster row)
- Future extensibility for multiple sync targets

### Client Modes

The worker supports three Gardener client implementations:

| Mode | Use Case | Backend |
|------|----------|---------|
| `mock` | Unit/integration tests | In-memory map |
| `local` | Local development with k3d | ConfigMaps (Phase 2) |
| `real` | Production | Gardener API (Phase 3) |

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `GARDENER_MODE` | No | `mock` | Client mode: `mock`, `local`, or `real` |
| `GARDENER_KUBECONFIG` | When `real` | - | Path to Garden cluster kubeconfig |
| `GARDENER_NAMESPACE` | No | `garden-fundament` | Gardener project namespace |
| `LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `POLL_INTERVAL` | No | `30s` | LISTEN timeout / fallback poll interval |
| `RECONCILE_INTERVAL` | No | `5m` | Full reconciliation interval |
| `STATUS_POLL_INTERVAL` | No | `30s` | How often to poll Gardener for status |
| `STATUS_POLL_BATCH_SIZE` | No | `50` | Max clusters to check per poll cycle |
| `HEALTH_PORT` | No | `8097` | Port for health check endpoints |
| `SHUTDOWN_TIMEOUT` | No | `30s` | Max time to wait for graceful shutdown |

## Running

```bash
# Development with mock client
GARDENER_MODE=mock \
DATABASE_URL=postgres://user:pass@localhost:5432/fundament \
go run ./cluster-worker/cmd/cluster-worker

# Run tests
go test ./cluster-worker/...

# Run tests with database integration
DATABASE_URL=postgres://... go test ./cluster-worker/... -v
```

## Health Endpoints

- `GET /healthz` - Liveness probe (always 200 if process is running)
- `GET /readyz` - Readiness probe (200 when LISTEN connection is established)

## Project Structure

```
cluster-worker/
├── cmd/cluster-worker/
│   └── main.go              # Entry point, config, health server
├── pkg/
│   ├── worker/
│   │   ├── worker.go        # Main sync loop with LISTEN/NOTIFY
│   │   ├── worker_test.go   # Unit tests
│   │   ├── status_poller.go # Gardener status polling
│   │   └── status_poller_test.go
│   ├── gardener/
│   │   ├── client.go        # Interface and types
│   │   └── mock.go          # MockClient for testing
│   └── db/
│       ├── queries.sql      # sqlc queries
│       ├── sqlc.yaml        # sqlc config
│       └── gen/             # Generated code
└── README.md
```

## Implementation Phases

- **Phase 1** (current): MockClient + automated tests
- **Phase 2**: LocalClient (ConfigMaps in k3d) + manual testing
- **Phase 3**: RealClient (Gardener API) + production deployment

## References

- [PostgreSQL SKIP LOCKED](https://www.2ndquadrant.com/en/blog/what-is-select-skip-locked-for-in-postgresql-9-5/)
- [LISTEN/NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html)
- [Gardener Shoots](https://gardener.cloud/docs/getting-started/shoots/)
- [testing/synctest](https://pkg.go.dev/testing/synctest) - Go 1.25 fake clock for tests
