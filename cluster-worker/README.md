# cluster-worker

A background worker service that synchronizes cluster state from PostgreSQL to Gardener by creating, updating, and deleting Shoot cluster manifests.

## Terms

| Term | Description |
|------|-------------|
| **Gardener** | Kubernetes cluster management platform that provisions and manages clusters across cloud providers |
| **Shoot** | Gardener's term for a managed Kubernetes cluster (the workload cluster where applications run) |
| **Reconciliation** | Gardener's process of making the actual cluster state match the desired Shoot manifest |
| **Sync** | Pushing local database state (cluster definition) to Gardener as a Shoot manifest |

## What

The cluster-worker watches for changes to the `tenant.clusters` table and ensures that each cluster has a corresponding Shoot manifest in Gardener. It handles:

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

### Sequence Diagram

```mermaid
sequenceDiagram
    participant User
    participant Frontend as Console Frontend
    participant API as Fundament API
    participant DB as PostgreSQL
    participant Worker as cluster-worker
    participant Gardener

    User->>Frontend: Create/Update/Delete cluster
    Frontend->>API: POST/PUT/DELETE /clusters
    API->>DB: INSERT/UPDATE tenant.clusters

    Note over DB: Trigger fires
    DB->>DB: SET synced = NULL in cluster_sync
    DB-->>Worker: NOTIFY cluster_sync

    Worker->>DB: SELECT ... FOR UPDATE SKIP LOCKED
    DB-->>Worker: Claimed cluster row

    alt Cluster created/updated
        Worker->>Gardener: ApplyShoot(manifest)
        Gardener-->>Worker: OK / Error
    else Cluster deleted
        Worker->>Gardener: DeleteShoot(name)
        Gardener-->>Worker: OK / Error
    end

    alt Success
        Worker->>DB: synced = now(), sync_error = NULL
    else Error
        Worker->>DB: sync_error = msg, sync_attempts++
    end

    Note over Worker,Gardener: Status Poller (separate goroutine)

    loop Every 30s
        Worker->>Gardener: GetShootStatus(clusters...)
        Gardener-->>Worker: Status (progressing/ready/error)
        Worker->>DB: UPDATE shoot_status, shoot_status_message
    end

    User->>Frontend: View cluster status
    Frontend->>API: GET /clusters/{id}
    API->>DB: SELECT cluster + sync status
    DB-->>API: Cluster with shoot_status
    API-->>Frontend: Cluster response
    Frontend-->>User: Show status (provisioning/running/error)
```

### State Diagram

The cluster-worker has two goroutines managing related but distinct state machines:

- **Sync Worker**: Pushes local database changes to Gardener (create/update/delete shoots)
- **Status Poller**: Observes Gardener and writes shoot status back to the database

```mermaid
stateDiagram-v2
    direction TB

    state "Sync Worker → cluster_sync table" as db {
        [*] --> Unsynced: User creates cluster
        Synced --> Unsynced: User modifies/deletes cluster

        state Unsynced {
            [*] --> Pending
            Pending --> BackingOff: sync failed
            BackingOff --> Pending: backoff elapsed
        }

        Unsynced --> Synced: Worker succeeds

        Unsynced: synced = NULL
        Synced: synced = timestamp
    }

    state "Status Poller → shoot_status column" as poller {
        [*] --> Pending2: Worker applied manifest
        Pending2 --> Progressing: Poller observes Gardener
        Progressing --> Ready: Reconciliation complete
        Progressing --> Error: Reconciliation failed
        Error --> Progressing: Gardener retrying
        Ready --> Progressing: Update in progress
        Ready --> Deleting: Worker deleted shoot
        Deleting --> Deleted: Poller confirms removal

        Pending2: pending
        Progressing: progressing
        Ready: ready
        Error: error
        Deleting: deleting
        Deleted: deleted
    }
```

**Sync Worker** pushes local changes to Gardener:

| Database State | Columns | Meaning |
|----------------|---------|---------|
| Unsynced (pending) | `synced = NULL`, `sync_attempts = 0` | Ready for first sync attempt |
| Unsynced (backing off) | `synced = NULL`, `sync_attempts > 0` | Waiting for backoff before retry |
| Synced | `synced = timestamp`, `sync_error = NULL` | Successfully synced to Gardener |

The worker claims unsynced clusters with `SELECT ... FOR UPDATE SKIP LOCKED` (transient lock, not stored).
Backoff formula: `30s × 2^attempts`, capped at 15 minutes.

**Status Poller** observes Gardener and updates `shoot_status`:

| Status | Meaning |
|--------|---------|
| `pending` | Shoot manifest applied, not yet visible in Gardener |
| `progressing` | Gardener is creating/updating the cluster |
| `ready` | Cluster is fully operational |
| `error` | Reconciliation failed (see `shoot_status_message`) |
| `deleting` | Shoot deletion in progress |
| `deleted` | Shoot confirmed removed from Gardener |

**Step-by-step:**

1. **Trigger**: Database triggers set `synced = NULL` when a cluster needs syncing (insert, update, or soft-delete)
2. **Notify**: The trigger sends `NOTIFY cluster_sync` to wake up workers
3. **Claim**: Worker runs `SELECT ... FOR UPDATE SKIP LOCKED` to claim one cluster
4. **Sync**: Worker applies or deletes the Shoot manifest in Gardener
5. **Mark**: Worker sets `synced = now()` on success, or records error and increments `sync_attempts` on failure
6. **Repeat**: Worker processes next pending cluster

### Database Schema

Sync state is stored in a separate `cluster_sync` table (1:1 with clusters):

```sql
-- tenant.cluster_sync (separate from clusters table)
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
- Tenant isolation via joins (users access sync state by joining through RLS-protected clusters table)
- Minimal worker privileges (cluster-worker has read-only access to clusters, write access only to sync state)

### Client Modes

The worker supports two Gardener client implementations:

| Mode | Use Case | Backend |
|------|----------|---------|
| `mock` | Unit/integration tests | In-memory map |
| `real` | Production + local Gardener | Gardener API |

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `GARDENER_MODE` | No | `mock` | Client mode: `mock` or `real` |
| `GARDENER_KUBECONFIG` | When `real` | - | Path to Garden cluster kubeconfig |
| `GARDENER_NAMESPACE` | No | `garden-fundament` | Gardener project namespace |
| `GARDENER_PROVIDER_TYPE` | No | `local` | Provider type: `local`, `metal`, `aws`, etc. |
| `GARDENER_CLOUD_PROFILE` | No | `local` | Cloud profile name |
| `GARDENER_CREDENTIALS_BINDING_NAME` | No | - | Credentials binding name (not needed for local provider) |
| `GARDENER_MAX_SHOOT_NAME_LEN` | No | `21` | Max shoot name length (21 for local, up to 63 for others) |
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

## Quick Start: Full Local Development

Run the complete stack with local Gardener:

```bash
# 1. Start k3d cluster
just cluster-start

# 2. Start local Gardener + configure secrets (first time ~15 min)
just local-gardener

# 3. Deploy all services with local Gardener mode
just dev -p local-gardener

# 4. Start console frontend (separate terminal)
cd console-frontend && npm start

# 5. Create a test cluster via console (http://localhost:4200) or CLI:
cd cluster-worker && just create-test-cluster t1

# Watch progress:
just logs-cluster-worker                  # cluster-worker logs
cd cluster-worker && just watch-shoots    # shoots in Gardener
cd cluster-worker && just gardener-status # overall status
```

**Prerequisites:**
- Docker with 8+ CPUs and 8+ GB memory
- `mise trust && mise install` (installs all tools)
- macOS only: GNU tools (`brew install gnu-sed gnu-tar iproute2mac`)

**Skaffold profiles:**
- `just dev` → mock mode (no Gardener needed)
- `just dev -p local-gardener` → real local Gardener (requires step 2 first)

First Gardener run takes ~15 minutes to build. Subsequent runs are instant.

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
│   │   ├── mock.go          # MockClient for testing
│   │   └── real.go          # RealClient for Gardener API
│   └── db/
│       ├── queries.sql      # sqlc queries
│       ├── sqlc.yaml        # sqlc config
│       └── gen/             # Generated code
└── README.md
```

## References

- [PostgreSQL SKIP LOCKED](https://www.2ndquadrant.com/en/blog/what-is-select-skip-locked-for-in-postgresql-9-5/)
- [LISTEN/NOTIFY](https://www.postgresql.org/docs/current/sql-notify.html)
- [Gardener Shoots](https://gardener.cloud/docs/getting-started/shoots/)
- [testing/synctest](https://pkg.go.dev/testing/synctest) - Go 1.25 fake clock for tests

