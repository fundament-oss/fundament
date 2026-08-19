---
title: "Example: Ceph Storage (Rook)"
sidebar:
  label: Ceph/Rook plugin
  order: 8
---

The ceph-rook plugin provides block storage for in-cluster workloads by installing and managing the Rook-Ceph operator and a singleton CephCluster.

**Advanced.** Storage is the most demanding plugin to run locally — it needs raw block devices
and a virtualised Docker host. It is not part of the standard
[plugin walkthrough](./install-a-plugin.md).

:::tip[Verifying it works]
This page describes the design. To actually exercise the plugin against a local
cluster — from empty machine to a pod writing to a Ceph-backed volume — follow the
[verification runbook](./ceph-rook-runbook.md).
:::

## What it does

1. **Install**: Installs the Rook-Ceph operator via Helm (from `charts.rook.io/release`) and bootstraps a single `CephCluster` resource
2. **Discover**: Rook discovers available block devices on cluster nodes and publishes them as `Disk` CRs (cluster-scoped inventory)
3. **Provision**: Operators select disks in the console and create a `StoragePool` CR, specifying which disks to use and the replication strategy
4. **Reconcile**: The plugin folds selected disks into the singleton `CephCluster` as OSDs, creates a `CephBlockPool`, and produces a `StorageClass` for in-cluster PVC provisioning
5. **Console**: Serves list and detail HTML for `Disk` and `StoragePool` resources, allowing operators to inspect discovered devices and manage storage pools

## File structure

Decision logic lives in its own file next to its test, so the parts worth getting
right (replication, discovery parsing, claim precedence, rendering) are unit
testable without a cluster. The reconcilers are glue over those functions.

```
plugins/storage/ceph-rook/
├── main.go                  # Entry point: NewPlugin, call pluginruntime.Run()
├── plugin.go                # Start: scheme, install, manager, reconciler registration
├── console.go               # Embeds console/ as http.FileSystem (ConsoleProvider)
├── config.go                # FUNP_-prefixed environment configuration
├── install.go               # Helm install, CRD apply, CephCluster bootstrap
├── definition.yaml          # Metadata, permissions, menu, customComponents, allowedResources
├── api/v1alpha1/
│   ├── disk_types.go        # Disk CR: discovered block device inventory
│   ├── storagepool_types.go # StoragePool CR: operator's desired pool
│   ├── groupversion_info.go # Scheme registration + go:generate directives
│   └── zz_generated.deepcopy.go
├── crds/                    # Generated CRD YAML, embedded and applied at install
├── diskinventory_controller.go  # Rook discovery ConfigMaps -> Disk CRs
├── storagepool_controller.go    # StoragePool -> CephCluster OSDs, CephBlockPool, StorageClass
├── claims.go                # Derived names + which pool owns a contested disk
├── replication.go           # "auto" -> replica count and CRUSH failure domain
├── discovery.go             # Parses Rook's device JSON; deterministic Disk names
├── cephcluster.go           # Disk statuses -> spec.storage.nodes
├── rookvalues.go            # Helm values + the CephCluster bootstrap object
├── blockpool.go             # Renders CephBlockPool
├── storageclass.go          # Renders the RBD StorageClass
├── console/                 # Hand-written console pages (no build step)
│   ├── _shared.js           # SDK loader, escaping, navigation helpers
│   ├── disks-list.{html,js}
│   ├── storagepools-list.{html,js}
│   ├── storagepools-detail.{html,js}
│   └── storagepools-create.{html,js}
├── test-resources.yaml      # Sample StoragePool for sandbox verification
└── Dockerfile               # Multi-stage build (Go build + alpine with helm)
```

Every `*.go` above has a matching `*_test.go`.

### What the console can do

| Resource | Pages |
|---|---|
| `StoragePool` | list, detail, create, **edit**, **delete** |
| `Disk` | list, **detail** |

**Editing** lives on the StoragePool detail page rather than a page of its own:
`ComponentMapping` has `list`, `detail` and `create` slots but no `edit`, so an
Edit button swaps the read-only view for the same disk picker the create form
uses. It saves a merge-patch of `spec` only, so `status` is never clobbered and
the disk array is replaced wholesale rather than merged element-wise.

Unchecking a disk warns what the reconciler will and will not do: the device
leaves the CephCluster list, but **its OSD keeps running** until it is purged
from Ceph manually, and data may rebalance in the meantime.

**Deleting** is refused while any PersistentVolume still binds the pool's
StorageClass — the pool's deletion cascades to that StorageClass through owner
references, which would strand those volumes. The blocked view names each one.
An unused pool goes through type-to-confirm, and the confirmation states that
the CephBlockPool goes too and that OSDs survive.

The host has its own delete for plugin resources, but
`resource-detail.component.html` renders either a custom detail component **or**
the generic view, never both — so a plugin with a custom detail page supplies its
own. That is just as well here: the bound-volume guard is ceph-rook domain
knowledge the generic modal has no way to express.

### Console pages

`console.go` is what makes the pages reachable: `pluginruntime.Run` only serves
`/console/` for a plugin that implements `ConsoleProvider`, and each page must
also be named under `spec.customComponents` in `definition.yaml` or the host has
no route to it. `definition_test.go` asserts both directions — every referenced
file exists, and every `.html` file is referenced.

Pages are served under the plugin CSP (`script-src 'self'; style-src 'self'`,
no `unsafe-inline`), so they carry **no inline `onclick` handlers and no inline
`style` attributes** — both are blocked. Events are wired with
`addEventListener` and styling comes from the `.plugin-*` classes in
`plugin-sdk.css`. Navigation goes through `_shared.js`'s `navigateToDetail()` /
`navigateBack()`, which post to `window.fundament.parentOrigin` rather than `*`.

## Disk → StoragePool → StorageClass flow

The plugin follows a three-step workflow:

1. **Device Discovery** (`Disk` CR): Rook continually scans raw, unpartitioned disks on cluster nodes. The DiskInventoryReconciler publishes these as cluster-scoped `Disk` objects, annotated with node, device path, size, type (HDD/SSD/NVMe), and availability status.

### Device identity

A `Disk` reports two paths. `status.path` is the kernel name (`/dev/sdb`) — what an operator
matches against `lsblk`, but the kernel is free to reassign it on the next boot.
`status.stablePath` is the `/dev/disk/by-id/` link, picked from rook's `devLinks` in
preference order (`wwn-`, `nvme-eui.`, `scsi-`, `ata-`, …); links naming a *logical* layer
(`lvm-pv-uuid-`, `dm-`, `md-uuid-`) are ignored, because those can be rebuilt on top of a
different physical device.

The stable path is what goes into `CephCluster.spec.storage.nodes[].devices[].name` — Rook
documents the by-id form there as the one that "will not change after reboots" — and it is
what the Disk CR's name is hashed from (`DeviceKey`: stable path, else WWN, else serial,
else kernel path). That matters because a `Disk` whose *name* moves drops out of every
`StoragePool` that lists it: the old name resolves to nothing, and the device silently
leaves the CephCluster.

Loop devices, and some virtual disks, expose no stable identity at all. They fall back to
the kernel path and are only as stable as the kernel's ordering — acceptable for the k3d
dev flow, which is the only place they occur.

2. **Pool Selection** (`StoragePool` CR): Operators use the console to select which disks to pool together. Creating a `StoragePool` specifies a list of disk names and a replication strategy (see [Replication](#replication-strategy) below). The StoragePoolReconciler watches the `StoragePool` and applies the changes.

3. **Storage Class** (`CephBlockPool` + `StorageClass`): For each `StoragePool`, the plugin:
   - Creates a `CephBlockPool` in the Rook namespace with the selected disks added as OSDs to the singleton `CephCluster`
   - Creates a `StorageClass` that in-cluster workloads can reference in their `PersistentVolumeClaim` spec, enabling dynamic RBD provisioning

### Derived object names

Both derived objects are named `ceph-<pool>`, not `<pool>`. A `StoragePool` name is
operator-chosen and a `StorageClass` is cluster-scoped, so an unprefixed name
would collide with whatever else is on the cluster — a pool called `local-path`
would otherwise take over k3d's default `StorageClass` and garbage-collect it
when the pool was deleted. `status.storageClassName` reports the real name, which
is what a `PersistentVolumeClaim` should reference.

The prefix reduces collisions; ownership is what prevents damage. Before writing
either object the reconciler checks for a controller reference back to this pool,
and refuses to adopt anything else — the pool goes `Degraded` with the conflicting
object named, rather than silently taking it over.

### Contested disks

Nothing stops two `StoragePool`s from listing the same disk. When that happens the
older pool keeps it (ties break on name), the younger pool skips it and says so in
`status.message`, and the disk is counted once. The same precedence populates
`Disk.status.claimedBy`, which is what the console's disk picker filters on, so the
inventory and the reconciler never disagree about who owns what.

## Single CephCluster, multiple StoragePool model

The plugin follows a **singleton CephCluster** pattern: only one `CephCluster` is deployed per cluster. Multiple `StoragePool` CRs contribute disks to the same cluster, creating separate `CephBlockPool`s (tiers/pools within the one cluster).

This design supports:
- **Multi-tenancy**: Different `StoragePool`s can have different replication or node selection constraints, allowing per-workload customization.
- **Tiering**: You can create pools for different device types (e.g., one pool for SSD disks, another for HDD), and operators can select which pools their applications use.
- **Shared infrastructure**: All storage in the cluster flows through a single Ceph cluster, simplifying backup, disaster recovery, and capacity planning.

## Replication strategy

The `StoragePool` spec includes a `replication` field that controls how many copies Ceph maintains:

- `"auto"` (default): derives the replica count from the number of nodes contributing disks, capped at 3 — `min(3, nodes)`. Two nodes give 2 replicas, five nodes still give 3.

- `"1"`, `"2"`, `"3"`: explicit replica count, e.g. to sacrifice redundancy for capacity on a test cluster.

An explicit count is **clamped down** to the number of nodes contributing disks to the cluster rather than leaving the pool unsatisfiable: asking for 3 replicas on a 2-node cluster yields 2, and the reason is recorded in `status.message`. The pool provisions either way; it does not sit degraded waiting for disks that may never arrive.

The node count is the cluster's, not the pool's — see [Pools share one OSD set](#pools-share-one-osd-set).

The failure domain follows from the result: `host` when the pool ends up with 2 or more replicas across 2 or more nodes, otherwise `osd`. A single-node cluster therefore still provisions — with `osd` domain and, at 1 replica, `requireSafeReplicaSize: false`, which Ceph needs to accept a size-1 pool at all.

The resulting replica count, failure domain and any clamping message are recorded in the `StoragePool` status.

### Status fields

`status` describes the pool's *contribution* to one shared Ceph cluster, not live Ceph state and not a slice of storage the pool owns:

- `selectedDiskCount` — how many of `spec.disks` resolved to a usable `Disk`. Not the number of OSDs Ceph is running; Rook creates those asynchronously.
- `rawCapacityBytes` — the summed size of those disks, **before** replication. This is **not the pool's capacity**: see [Pools share one OSD set](#pools-share-one-osd-set). Use `ceph df` for real free space.
- `phase` — `Provisioning` until the backing `CephBlockPool` reports `Ready`; `Degraded` when the pool cannot be reconciled without operator action (see below), including when it resolves no disks at all.

### Pools share one OSD set

Every `StoragePool` feeds the same `CephCluster`, and the `CephBlockPool` a pool derives carries **no CRUSH rule confining it to that pool's disks**. Ceph places a volume's data across every OSD in the cluster.

Three consequences, all of them load-bearing:

- **Multiple pools do not isolate or tier storage.** A second pool gives you a second `StorageClass` over the same disks. Real separation needs a device class plus a per-pool CRUSH rule, which this version does not implement.
- **`rawCapacityBytes / replicas` is not usable capacity.** With more than one pool it is not even an upper bound.
- **Replication is sized against the cluster, not the pool.** `auto` uses the number of nodes contributing disks cluster-wide. This is deliberate: sizing off a pool's own disks would cap a single-disk pool at `replicas: 1` — waiving Ceph's size-1 safety check to advertise no redundancy — while Ceph replicated that data across all hosts regardless.

### Removing disks

Taking a disk out of `spec.disks` removes it from the `CephCluster`'s device list, but **does not remove the OSD**: Rook needs an explicit purge job for that. Deleting a `StoragePool` behaves the same way. Treat disk removal as a two-step operation — update the pool, then purge the OSD through Ceph.

## Why it needs cluster-admin-equivalent RBAC

The Rook-Ceph operator requires broad, cluster-admin-level permissions to:
- Discover and manage block devices at the node/host level (privileged kernel access)
- Create and update Kubernetes resources across all namespaces (`StorageClass`, `PersistentVolume`, etc.)
- Install its own cluster-wide RBAC (ClusterRoles/Bindings with escalate+bind) and the Ceph CSI driver

The plugin declares this in its `definition.yaml` under `spec.permissions.rbac` (a
cluster-admin-equivalent wildcard rule). The plugin-controller materialises those rules into a
`ClusterRole` bound to the plugin's `ServiceAccount`, intersected with the installing admin's own
permissions (FUN-17). There is no separate `clusterRoles` field on the `PluginInstallation` — it
references a published definition:

```yaml
# Example PluginInstallation (references a published plugin version; see FUN-19)
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: ceph-rook
spec:
  definitionRef:
    pluginName: ceph-rook
    pluginVersion: v0.1.0
    definitionHash: sha256:<hash printed by `just plugin-publish storage/ceph-rook`>
```

## Host prerequisites

For Rook to discover and claim disks, cluster nodes must meet these requirements:

1. **Raw, unpartitioned block devices**: Rook will only claim empty disks with no filesystems or partition tables. If a disk is already in use or partitioned, it will be marked unavailable in the `Disk` CR.

2. **RBD kernel module**: The `rbd` module must be available on nodes. This is typically included in standard Linux distributions but may need to be loaded explicitly on some systems:
   ```bash
   modprobe rbd
   ```

3. **LVM2**: The `lvm2` package must be installed on nodes for OSD device management:
   ```bash
   # Ubuntu/Debian
   apt-get install lvm2

   # CentOS/RHEL
   yum install lvm2
   ```

Ensure these prerequisites are in place on all nodes before creating a `StoragePool`; otherwise, disks may remain unavailable or OSDs may fail to initialize.

## Plugin lifecycle

```
  Container starts
       │
       ▼
  pluginruntime.Run()
       │
       ├─ HTTP server on :8080
       │
       ▼
  Start()
       │
       ├─ Install Rook-Ceph Helm chart
       ├─ Bootstrap singleton CephCluster CR
       ├─ Create k8s client
       ├─ Register DiskInventoryReconciler and StoragePoolReconciler
       ├─ Start controller-runtime manager
       ├─ ReportReady()
       ├─ ReportStatus("running", "rook-ceph storage plugin running")
       └─ Block until SIGTERM
              │
              ▼
  Reconcile loops (event-driven)
       │
       ├─ DiskInventoryReconciler: reacts to rook-discover ConfigMaps
       │   │                       AND to StoragePool changes
       │   ├─ publish/update Disk CRs from the probed devices
       │   ├─ recompute claimedBy from the current pools
       │   └─ mark vanished disks unavailable (soft delete; never Delete)
       │
       └─ StoragePoolReconciler: reacts to StoragePool/Disk changes
           ├─ On delete: recompute the CephCluster union without this pool
           │             (owner refs GC the CephBlockPool and StorageClass)
           └─ Otherwise, for the pool:
               ├─ Resolve spec.disks, skipping missing and contested disks
               ├─ Fold every live pool's disks into CephCluster OSDs (deduped)
               ├─ Create/update CephBlockPool  ceph-<pool>  (refuse if not ours)
               ├─ Create StorageClass          ceph-<pool>  (refuse if not ours)
               ├─ Update status (phase, replicas, selected disks, raw capacity)
               └─ Re-check every 30 s while phase is Provisioning
                  (until CephBlockPool reports Ready)
```

A reconcile that fails writes `phase: Degraded` with the cause in
`status.message` before returning the error, so the reason shows up in the
console rather than only in the plugin's logs.
