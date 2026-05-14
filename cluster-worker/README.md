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

The worker also monitors Gardener to track the reconciliation status of each Shoot (pending, progressing, ready, error) and stores this in the `shoot_status` column. When a shoot becomes ready, the worker triggers user sync to create per-user service accounts on the cluster.

The `tenant.cluster_outbox` table also tracks changes to `organizations_users` and `project_members` via database triggers, laying the groundwork for a future UserSyncHandler that will reconcile service accounts and RBAC on shoot clusters.

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

### Why SKIP LOCKED + Visibility Timeout?

The `SELECT ... FOR UPDATE SKIP LOCKED` pattern combined with a visibility timeout enables multiple workers to process clusters concurrently without conflicts:

- Workers grab available work without blocking each other
- Natural load distribution across workers
- No coordinator needed
- **Crash recovery**: If a worker dies mid-sync, the visibility timeout (10 min) allows another worker to reclaim the work
- **Exponential backoff**: Failed syncs wait 30s × 2^(attempts-1) before retry, capped at 15 minutes
- Each claim is tracked with `sync_claimed_at` and `sync_claimed_by` for debugging

### Why a separate status poller?

Gardener Shoot reconciliation is asynchronous - applying a manifest returns immediately, but the actual cluster creation takes minutes. A separate goroutine polls Gardener for status updates because:

- The main sync loop stays fast (just applies manifests)
- Users can see `shoot_status` to know if their cluster is actually ready
- We can detect and alert on failed reconciliations
- Deletion verification confirms Shoots are actually gone
- Connection data (API server URL, CA cert) is extracted when a shoot becomes ready, enabling kubeconfig generation

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
    DB->>DB: INSERT into cluster_outbox
    DB-->>Worker: NOTIFY cluster_sync

    Worker->>DB: Claim outbox row (SKIP LOCKED)
    DB-->>Worker: Claimed outbox row

    alt Cluster created/updated
        Worker->>Gardener: ApplyShoot(manifest)
        Gardener-->>Worker: OK / Error
    else Cluster deleted
        Worker->>Gardener: DeleteShoot(clusterID)
        Gardener-->>Worker: OK / Error
    end

    alt Success
        Worker->>DB: outbox_status = completed
        Worker->>DB: INSERT cluster_events (sync_succeeded)
    else Error
        Worker->>DB: outbox_status = retrying, retries++
        Worker->>DB: INSERT cluster_events (sync_failed)
    end

    Note over Worker,Gardener: Status Poller (separate goroutine)

    loop Every 30s
        Worker->>Gardener: GetShootStatus(clusters...)
        Gardener-->>Worker: Status + API server URL
        Worker->>DB: UPDATE shoot_status, shoot_status_message
        opt Status changed
            Worker->>DB: INSERT cluster_events
        end
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

    state "Sync Worker → cluster_outbox table" as db {
        [*] --> Pending: Trigger fires on clusters/org_users/project_members
        Completed --> Pending: New change detected

        Pending --> InProgress: Worker claims (SKIP LOCKED)
        InProgress --> Completed: sync succeeded
        InProgress --> Retrying: sync failed
        Retrying --> Pending: backoff elapsed
        InProgress --> Pending: visibility timeout (worker died)

        Pending: status = pending
        InProgress: status = pending, claimed
        Retrying: status = retrying
        Completed: status = completed
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
        Ready: ready (+ extract API server URL, CA data)
        Error: error
        Deleting: deleting
        Deleted: deleted
    }
```

### Client Modes

The worker supports two Gardener client implementations:

| Mode | Use Case | Backend |
|------|----------|---------|
| `mock` | Unit/integration tests | In-memory map |
| `real` | Production + local Gardener | Gardener API |

### Event History

All sync and status changes are recorded in the `cluster_events` table for debugging and auditing:

| Event Type | Description |
|------------|-------------|
| `sync_requested` | Cluster created/updated/deleted via API, needs sync |
| `sync_claimed` | Worker claimed the cluster for processing |
| `sync_succeeded` | Gardener accepted the Shoot manifest |
| `sync_failed` | Sync failed (with error message and attempt count) |
| `status_progressing` | Shoot reconciliation in progress |
| `status_ready` | Shoot reconciliation completed successfully |
| `status_error` | Shoot reconciliation failed |
| `status_deleted` | Shoot confirmed deleted from Gardener |

### Outbox Sources

The `cluster_outbox` table tracks changes from multiple sources:

| Source | Trigger |
|--------|---------|
| `trigger` | Database trigger on `clusters`, `organizations_users`, or `project_members` |
| `reconcile` | Periodic reconciliation loop |
| `manual` | Manual intervention |
| `node_pool` | Node pool configuration change |
| `status` | Status poller detected a state change |

## Quick Start: Full Local Development

Start with preparing your setup by installing all Gardener dependencies and tools.
See https://github.com/gardener/gardener/blob/master/docs/development/local_setup.md#preparing-the-setup

Run the complete stack with local Gardener (gardener-operator path):

```bash
# 1. Start k3d cluster
just cluster-start

# 2. Start local Gardener via gardener-operator (first time ~15 min)
just cluster-worker gardener-up

# 3. Deploy all services with local Gardener mode
just dev -p local-gardener

# 4. Access the console frontend
open http://console.fundament.localhost:8080

# 5. Create a test cluster via console or CLI:
just cluster-worker create-test-cluster t1

# Watch progress:
just cluster-worker shoots    # shoots in Gardener
just cluster-worker logs      # cluster-worker logs
just cluster-worker gardener-status # overall status
```

**Troubleshooting:**
```bash
# Re-connect Docker networks (if k3d can't reach Gardener after restart)
just cluster-worker gardener-connect

# Re-create the kubeconfig secret (if cluster-worker can't authenticate to Gardener)
just cluster-worker gardener-secret
```

**Prerequisites:**
- Docker with 8+ CPUs and 8+ GB memory
- `mise trust && mise install` (installs all tools)
- macOS only: GNU tools (`brew install gnu-sed gnu-tar iproute2mac`)

**Pinned versions** (for team consistency):
- Gardener: `v1.138.0` (see `GARDENER_VERSION` in mod.just)
- Other tools: see `mise.toml`

**Skaffold profiles:**
- `just dev` → mock mode (no Gardener needed)
- `just dev -p local-gardener` → real local Gardener (requires step 2 first)

First Gardener run takes ~15 minutes to build. Subsequent runs are instant.
