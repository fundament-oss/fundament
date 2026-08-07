---
title: "Example: Ceph Storage (Rook)"
sidebar:
  order: 5
---

The ceph-rook plugin provides block storage for in-cluster workloads by installing and managing the Rook-Ceph operator and a singleton CephCluster.

## What it does

1. **Install**: Installs the Rook-Ceph operator via Helm (from `charts.rook.io/release`) and bootstraps a single `CephCluster` resource
2. **Discover**: Rook discovers available block devices on cluster nodes and publishes them as `Disk` CRs (cluster-scoped inventory)
3. **Provision**: Operators select disks in the console and create a `StoragePool` CR, specifying which disks to use and the replication strategy
4. **Reconcile**: The plugin folds selected disks into the singleton `CephCluster` as OSDs, creates a `CephBlockPool`, and produces a `StorageClass` for in-cluster PVC provisioning
5. **Console**: Serves list and detail HTML for `Disk` and `StoragePool` resources, allowing operators to inspect discovered devices and manage storage pools

## File structure

```
plugins/storage/ceph-rook/
├── main.go             # Entry point: load definition, call pluginruntime.Run()
├── plugin.go           # Plugin implementation (Start, Install, Reconcile, etc.)
├── console.go          # Embeds console/ directory as http.FileSystem
├── config.go           # Configuration loader
├── definition.yaml     # Metadata, permissions, menu, customComponents, allowedResources
├── install.go          # Helm install and CephCluster bootstrap logic
├── api/v1alpha1/
│   ├── disk_types.go      # Disk CR: discovered block device inventory
│   ├── storagepool_types.go  # StoragePool CR: operator's desired pool
│   └── groupversion_info.go
├── controllers/
│   ├── disk_inventory_reconciler.go  # Publishes Disk CRs from Rook discovery
│   └── storagepool_reconciler.go     # Manages CephBlockPool and StorageClass
├── crds/                # Generated CRD YAML files
├── console/
│   ├── _shared.js              # SDK loader + shared helpers
│   ├── disks-list.html
│   └── storagepools-list.html
│   └── storagepools-detail.html
├── plugin_test.go      # Unit tests
└── Dockerfile          # Multi-stage build (Go build + alpine with helm)
```

The Disk and StoragePool CRs are defined in `api/v1alpha1/`. `_shared.js` contains the SDK loader (`loadSdk()` reads `?host=` from the query string and injects the `plugin-sdk.js` / `.css` tags) plus rendering helpers.

## Disk → StoragePool → StorageClass flow

The plugin follows a three-step workflow:

1. **Device Discovery** (`Disk` CR): Rook continually scans raw, unpartitioned disks on cluster nodes. The DiskInventoryReconciler publishes these as cluster-scoped `Disk` objects, annotated with node, device path, size, type (HDD/SSD/NVMe), and availability status.

2. **Pool Selection** (`StoragePool` CR): Operators use the console to select which disks to pool together. Creating a `StoragePool` specifies a list of disk names and a replication strategy (see [Replication](#replication-strategy) below). The StoragePoolReconciler watches the `StoragePool` and applies the changes.

3. **Storage Class** (`CephBlockPool` + `StorageClass`): For each `StoragePool`, the plugin:
   - Creates a `CephBlockPool` in the Rook namespace with the selected disks added as OSDs to the singleton `CephCluster`
   - Creates a `StorageClass` that in-cluster workloads can reference in their `PersistentVolumeClaim` spec, enabling dynamic RBD provisioning

## Single CephCluster, multiple StoragePool model

The plugin follows a **singleton CephCluster** pattern: only one `CephCluster` is deployed per cluster. Multiple `StoragePool` CRs contribute disks to the same cluster, creating separate `CephBlockPool`s (tiers/pools within the one cluster).

This design supports:
- **Multi-tenancy**: Different `StoragePool`s can have different replication or node selection constraints, allowing per-workload customization.
- **Tiering**: You can create pools for different device types (e.g., one pool for SSD disks, another for HDD), and operators can select which pools their applications use.
- **Shared infrastructure**: All storage in the cluster flows through a single Ceph cluster, simplifying backup, disaster recovery, and capacity planning.

## Replication strategy

The `StoragePool` spec includes a `replication` field that controls how many copies Ceph maintains:

- `"auto"` (default): Automatically selects the optimal replica count based on the number of nodes contributing disks:
  - **2+ nodes**: Uses `host` failure domain (replicas equal the number of contributing nodes), tolerating a single node failure.
  - **1 node**: Uses `osd` failure domain (replicas = 2), tolerating a single OSD/disk failure.
  - If this results in more replicas than available OSDs, the pool enters a degraded state until sufficient disks are added.

- `"1"`, `"2"`, `"3"`: Explicit replica count. Operators may set this to override auto behavior (e.g., to sacrifice redundancy for capacity on test clusters).

The resulting replica count and failure domain are recorded in the `StoragePool` status.

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
       ├─ DiskInventoryReconciler: reacts to Disk/ConfigMap changes
       ├─   → publish/update Disk CRs
       │
       └─ StoragePoolReconciler: reacts to StoragePool/Disk changes
           └─ For each StoragePool:
               ├─ Fold selected disks into CephCluster OSDs
               ├─ Create CephBlockPool
               ├─ Create StorageClass
               ├─ Update StoragePool status (phase, replicas, capacity)
               └─ Re-checks every 30 s while phase is Provisioning
                  (until CephBlockPool reports Ready)
```
