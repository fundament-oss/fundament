---
title: "Runbook: verifying the Ceph/Rook plugin"
sidebar:
  label: Ceph/Rook runbook
  order: 9
---

End-to-end verification of the `ceph-rook` plugin against the local `k3d-fundament-plugin`
sandbox cluster, from an empty machine to a pod writing to a Ceph-backed volume.

**Advanced.** This is the longest path in the repo. For the standard plugin flow, start with
[Install a plugin](./install-a-plugin.md).

The unit tests cover the plugin's decision logic and its reconcilers against a fake
client. They cannot tell you whether Rook actually builds an OSD out of a real block
device, whether the CSI driver can map an RBD image, or whether the console pages render.
That is what this runbook is for.

Budget 45–60 minutes for a first run on a warm Docker cache; most of it is Ceph pulling
images and waiting for OSDs. From no images at all, closer to two hours — the platform
cluster and its 14 images account for most of the difference.

New to the two-cluster setup? Read
[The two development clusters](./dev-environment.md) first, and ideally do the shorter
[Install a plugin](./install-a-plugin.md) walkthrough before this one. This runbook assumes both
clusters already exist.

## What each phase proves

| Phase | Proves |
|---|---|
| 1 · Environment | The node has real block devices and a live `/dev` |
| 2 · Baseline | Rook + Ceph work here **without** any Fundament code |
| 3 · Publish | The image builds and the definition reaches organization-api |
| 4 · Install | The plugin's install path runs: Helm, CRDs, CephCluster bootstrap |
| 5 · Discovery | The DiskInventory reconciler turns real probes into `Disk` CRs |
| 6 · Pool | The StoragePool reconciler produces OSDs, a CephBlockPool and a StorageClass |
| 7 · PVC | The whole chain actually stores data |
| 8 · Regressions | The specific behaviours fixed in review hold on a real cluster |

Phase 2 is the one people skip and shouldn't. If it fails, the environment is broken and
nothing you learn in phases 4–8 means anything.

## Before you start

- **Docker host must be a VM.** `storage-disks.sh` refuses on bare metal, because a stuck
  RBD mapping is an unkillable kernel thread and the privileged node enumerates your real
  drives. On macOS the Docker host is already a VM. See
  [Block devices for k3d](../fundament/k3d-block-devices.md).
- **At least 8 GiB of VM memory.** Ceph needs roughly 2 GiB per OSD plus its daemons. On
  colima: `colima start --cpus 4 --memory 8`. Memory cannot be changed on a running VM.
- **The `k3d-fundament` platform cluster must be up**, because publishing goes through
  organization-api. The `k3d-fundament-plugin` sandbox cluster is where the plugin runs.
- **An activated mise environment.** `just`, `skaffold`, `jq` and `yq` come from mise and are
  otherwise not on `PATH`.

:::caution[Watch the `plugins` prefix]
Every command below runs from the **repository root**. `just dev` and `just plugins dev` are
different recipes deploying different things to different clusters — the root one deploys the
platform to `k3d-fundament`, the `plugins` module one deploys the plugin-controller to
`k3d-fundament-plugin`. `cd`-ing into `plugins/` does not select the latter: `mod.just` is not a
`Justfile`, so `just` walks up and finds the root recipe anyway.

Your kubectl context does not disambiguate them: both skaffold configurations pin
`kubeContext`, so `kubectl config use-context` has no effect on where they deploy. Every
`kubectl` command below passes `--context` explicitly for the same reason.
:::

## Phase 0 · Clean slate

Two things commonly survive from an earlier session and quietly poison a run: a sandbox
cluster created **without** the block-device binds, and a leftover `rook-ceph` namespace.

```bash
# Does the sandbox node have the binds? Empty output means no.
docker inspect k3d-fundament-plugin-server-0 \
  --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' | grep -E '^/dev$|^/run/udev$'

# Is there leftover Rook state?
kubectl --context k3d-fundament-plugin -n rook-ceph get cephcluster,cephblockpool,pod
```

Docker fixes a container's mounts at creation, so a cluster without the binds **cannot be
fixed in place** — it has to be recreated. Loop devices in the node's `/dev` do not prove
otherwise: without the bind that directory is a private tmpfs snapshot, and the entries in
it are stale artifacts.

If a previous Ceph cluster is still running, take it down in Rook's order *before*
deleting the k3d cluster, so its finalizers get cleared:

```bash
./deploy/k3d/rook-smoke.sh down     # if the leftovers came from the smoke script
just plugins uninstall system--ceph-rook      # if they came from the plugin
just plugins cluster-delete
```

`cluster-delete` drains Ceph consumers first, so it is safe even mid-experiment.

## Phase 1 · Environment

```bash
just plugins cluster-create-storage    # binds /dev and /run/udev into the node
just plugins storage-disks doctor      # inspect the kernel Docker actually runs on
just plugins storage-disks attach      # 3 × 20 GiB sparse images -> /dev/loop0p1 .. /dev/loop2p1
just plugins dev                       # plugin-controller into the sandbox; leave it running
```

`just plugins dev` deploys the plugin-controller to `k3d-fundament-plugin` — note the prefix; a
bare `just dev` deploys the platform instead. Without it the sandbox has the
`PluginInstallation` CRD but nothing that reconciles it, and phase 4 will apply a CR that never
progresses.

`doctor` is worth reading rather than skimming. The line that matters most:

```
  rbd      PRESENT (loaded or built-in)
```

If `rbd` is **MISSING**, `csi-rbdplugin` will CrashLoopBackOff and no RBD PVC will ever
mount. It fatals at startup, before it reads a StorageClass, so `mounter: rbd-nbd` does
not rescue it. Ceph itself still runs — mons, mgr and OSDs do not use the module — so
phases 4–6 remain testable and only phase 7 is blocked. On macOS, colima provides it.

Confirm the devices reached the node:

```bash
just plugins storage-disks status
```

Expect `/dev/loop0p1`, `/dev/loop1p1`, `/dev/loop2p1` both on the host and in the node.
Three is the useful floor — it is what lets a pool hold 3 replicas across OSDs.

:::note[After a reboot]
The backing files survive reboots; the loop devices do not, because those are kernel
state. Recovery is `just plugins storage-disks attach` and nothing else.
:::

## Phase 2 · Baseline without the plugin

```bash
./deploy/k3d/rook-smoke.sh up      # upstream chart + a hand-written CephCluster
./deploy/k3d/rook-smoke.sh status  # expect HEALTH_OK (or HEALTH_WARN about redundancy)
./deploy/k3d/rook-smoke.sh test    # 1Gi PVC + a pod that writes and verifies -> SMOKE-OK
```

`rook-smoke.sh` contains no Fundament code, so it cleanly separates "the environment is
broken" from "the plugin is broken". `SMOKE-OK` means disks, OSDs, the CSI driver and the
kernel are all fine, and any later failure is the plugin's.

:::note[CephFS is skipped on kernels without the `ceph` module]
Mapping a block device needs the `rbd` module; mounting CephFS needs `ceph`. A kernel can
ship one without the other — OrbStack has `rbd` but not `ceph` — so the test drops CephFS
and says so rather than failing:

```
/!\ no 'ceph' kernel module on the Docker host: CephFS cannot mount here.
    Testing RBD only. Block storage -- all the ceph-rook plugin provides -- is
    unaffected, so this is not a failure of the environment.
```

This plugin is block-only, so RBD is the whole story. `just plugins storage-disks has-module ceph`
answers the question on its own.
:::

:::caution[One `FailedMount` on first attach is normal]
ceph-csi maps with `--options noudev`, so `rbd map` does not wait for udev to create the
device node and the first attempt can lose the race:

```
rbd: mapping succeeded but /dev/rbd0 is not accessible, is host /dev mounted?
```

kubelet retries and it succeeds. Only treat it as real if it **repeats** — a genuinely
missing `/dev` bind fails every time. Confirm with
`docker exec k3d-fundament-plugin-server-0 ls -l /dev/rbd0`.
:::

Then free the namespace — the plugin wants the same one:

```bash
./deploy/k3d/rook-smoke.sh down
just plugins storage-disks reset            # wipe stale OSD metadata, reattach
```

`reset` is not optional between installs. Stale BlueStore metadata on the images, or Rook
state under `dataDirHostPath`, is the usual reason a second install fails.

## Phase 3 · Build and publish

Publishing resolves the plugin's catalog id by name, so the appstore seed must be present in
the `k3d-fundament` platform cluster. The seed carries the `ceph-rook` catalog entry and its `Storage`
category, is idempotent (`ON CONFLICT DO UPDATE`), and is applied by the `db-migrations` Job. If
the database predates that entry, re-run the Job:

This is the root `dev`, without the `plugins` prefix — see the caution above:

```bash
just dev                            # or: just deploy local — re-runs db-migrations
```

Bridge the sandbox controller to organization-api (re-run after recreating either cluster,
since it resolves the node IP fresh):

```bash
just plugins sandbox-orgapi
```

Now build, push and publish:

:::caution[Publish as `platform-admin@fundament.io`, not your usual dev login]
`PutPluginDefinition` requires `can_edit` on the plugin, which resolves to **admin of the
owning organization**. First-party plugins are owned by the seeded `system` org, and
`alice@acme-corp.com` and friends are not members of it — publishing as them fails
authorization.

`db/seed/0100-system-org.sql` says the system org "has no members", and in production that
is true. Local dev is the exception: `db/testdata/033_0101-content.sql` adds
`platform-admin@fundament.io` (`…1000-…008`) as its admin precisely so first-party
definitions can be published, and `values-local.yaml` gives it a dex login with the same
shared password as the other static users. It is applied only when
`TREK_INSERT_TEST_DATA=true`.

Because testdata is keyed to migration `033`, a database that was already past 033 when
that change landed never receives the row, and `trek apply` will not backfill it. Check
before you start:

```bash
kubectl --context k3d-fundament -n fundament exec pod/db-1 -c postgres -- \
  psql -U postgres -d fundament -c "SELECT u.email, ou.permission, ou.status
    FROM tenant.users u JOIN tenant.organizations_users ou ON ou.user_id = u.id
    WHERE ou.organization_id = '019b4000-0000-7000-8000-000000000000';"
```

One `admin`/`accepted` row is what you need. If it is missing, re-run the migrations with
`db.reset: true`, or insert `db/testdata/033_0101-content.sql` by hand.
:::

Set the four publish variables exactly as in
[Install a plugin, step 3](./install-a-plugin.md#3-publish-the-plugin-definition) — that page is
canonical for them — then:

```bash
just plugins publish storage/ceph-rook
```

Authorization also depends on two OpenFGA tuples, written by the authz-worker from the
database outbox. If publishing fails with a permission error while the SQL above looks
right, confirm the sync landed:

```bash
kubectl --context k3d-fundament -n fundament exec pod/db-1 -c postgres -- psql -U postgres -d openfga \
  -c "SELECT object_type, object_id, relation, _user FROM tuple
      WHERE (object_type='plugin' AND object_id='019b4000-3000-7000-8000-000000000011')
         OR (object_type='organization' AND object_id='019b4000-0000-7000-8000-000000000000');"
```

Expect `plugin:…011 owner organization:…000` and `organization:…000 admin user:…008`.

Expected last line — **copy the hash, phase 4 needs it**:

```
published plugin=ceph-rook version=v0.1.0 hash=sha256:... id=... definition_id=...
```

Republishing the same `v0.1.0` needs `--replace`, which soft-deletes the previous
definition:

```bash
just plugins publish storage/ceph-rook --replace
```

If it fails with `no catalog entry for "ceph-rook"`, the seed did not reach the database —
go back and re-run the migrations job.

## Phase 4 · Install

The plugin needs local-development config: without it the default 3 mons never reach
quorum on a single node, and the real-disk filter would ignore the loop devices.

```bash
kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: system--ceph-rook
spec:
  definitionRef:
    organizationName: system
    pluginName: ceph-rook
    pluginVersion: v0.1.0
    definitionHash: sha256:PASTE_THE_HASH_FROM_PHASE_3
  config:
    DEV_LOOP_DEVICES: "true"        # discover ONLY /dev/loopNpN; ignore the host's real disks
    MON_COUNT: "1"
    MGR_COUNT: "1"
    ALLOW_MULTIPLE_PER_NODE: "true"
YAML
```

`CEPH_IMAGE` already defaults to the build `rook-smoke.sh` validates, so leave it unless
you are testing a different one. `ALLOW_UNSUPPORTED_CEPH` defaults to `false` and should
stay there: it turns off Rook's check that it knows how to drive the Ceph release it is
given, and Rook v1.16 supports the default image's Squid (v19) line. Set it only when
pairing the plugin with a Ceph build outside that table.

:::caution[`DEV_LOOP_DEVICES` is a safety control, not a convenience]
A k3d node is privileged and enumerates the Docker host's real disks, and they *do* reach
the discovery ConfigMap. `ParseDiscoveredDevices` is the only thing keeping them out of an
OSD. With this flag the filter is *replaced* by loop-partitions-only, not extended.
:::

Watch it come up:

```bash
just plugins status
just plugins logs system--ceph-rook
```

Expect `PHASE=Running`, `READY=true`, and a log line `rook-ceph storage plugin running`.
The first install pulls the Ceph image and takes several minutes.

```bash
kubectl --context k3d-fundament-plugin -n rook-ceph get pods
kubectl --context k3d-fundament-plugin get crd | grep storage.fundament.io
kubectl --context k3d-fundament-plugin -n rook-ceph get cephcluster
```

Expect the operator, the discover DaemonSet, the CSI pods, both CRDs, and a `rook-ceph`
CephCluster whose `spec.storage.nodes` is still **empty** — nothing is consumed until an
operator opts disks in.

## Phase 5 · Disk discovery

```bash
kubectl --context k3d-fundament-plugin get disks
```

Expect exactly three, one per loop partition:

```
NAME                                      NODE                            PATH           SIZE          AVAILABLE
k3d-fundament-plugin-server-0-2e7309e042  k3d-fundament-plugin-server-0   /dev/loop0p1   21472739328   true
...
```

Two things to check, both of which have bitten this plugin:

```bash
# 1. No real host disks leaked in. Every path must be /dev/loopNpN.
kubectl --context k3d-fundament-plugin get disks \
  -o jsonpath='{range .items[*]}{.status.path}{"\n"}{end}'

# 2. Nothing is claimed yet.
kubectl --context k3d-fundament-plugin get disks \
  -o jsonpath='{range .items[*]}{.metadata.name}{" claimedBy="}{.status.claimedBy}{"\n"}{end}'
```

If the disk list is empty, check the discovery ConfigMaps the reconciler reads:

```bash
kubectl --context k3d-fundament-plugin -n rook-ceph get cm -l app=rook-discover
kubectl --context k3d-fundament-plugin -n rook-ceph get cm local-device-k3d-fundament-plugin-server-0 \
  -o jsonpath='{.data.devices}' | jq .
```

An empty `devices` list means the discover daemon saw nothing — go back to phase 1.
Devices present but no `Disk` CRs means the filter rejected them: with
`DEV_LOOP_DEVICES: "true"` only entries with `"type": "part"` on a `/dev/loopNpN` path
survive.

## Phase 6 · StoragePool → StorageClass

Disk names are cluster-specific (node name + a hash of the device's stable identity), so they cannot be
hard-coded. Build the pool from whatever was discovered:

```bash
DISKS=$(kubectl --context k3d-fundament-plugin get disks -o jsonpath='{range .items[*]}      - {.metadata.name}{"\n"}{end}')
kubectl --context k3d-fundament-plugin apply -f - <<YAML
apiVersion: storage.fundament.io/v1alpha1
kind: StoragePool
metadata:
  name: test-pool
spec:
  replication: auto
  disks:
$DISKS
YAML
```

Watch it provision — the first OSD takes a few minutes:

```bash
kubectl --context k3d-fundament-plugin get storagepool test-pool -w
```

The pool stays `Provisioning` for another 60–90 seconds after its `CephBlockPool` reports
`Ready`; the reconciler picks the change up on its next pass. That wait is normal, not a
stall.

```bash
kubectl --context k3d-fundament-plugin get storagepool test-pool -o yaml | yq .status
```

Expected, on this single-node cluster:

```yaml
phase: Ready
storageClassName: ceph-test-pool     # note the ceph- prefix
replicas: 1                          # auto = min(3, cluster nodes with disks); one node -> 1
failureDomain: osd                   # host domain needs >=2 replicas across >=2 nodes
selectedDiskCount: 3
rawCapacityBytes: 64418217984        # this pool's contribution, before replication
```

`rawCapacityBytes` is the raw size of the disks this pool contributes, **not** its
capacity — all pools share one OSD set and Ceph places data across every OSD in it. Ask
Ceph for real free space (`ceph df` in the toolbox). A pool that resolves *no* disks
reports `Degraded` with the reason in `status.message` and creates no `StorageClass`.

`selectedDiskCount` is how many of `spec.disks` resolved, **not** how many OSDs are
running. Check the OSDs separately:

```bash
kubectl --context k3d-fundament-plugin -n rook-ceph get pods -l app=rook-ceph-osd
kubectl --context k3d-fundament-plugin -n rook-ceph get cephcluster rook-ceph \
  -o jsonpath='{.spec.storage.nodes}' | jq .
```

Expect three OSD pods and all three devices listed under the node. Then confirm the
derived objects and their ownership:

```bash
kubectl --context k3d-fundament-plugin get storageclass ceph-test-pool
kubectl --context k3d-fundament-plugin -n rook-ceph get cephblockpool ceph-test-pool
kubectl --context k3d-fundament-plugin get storageclass ceph-test-pool \
  -o jsonpath='{.metadata.ownerReferences}' | jq .
```

The owner reference must name `test-pool` with `controller: true` — that is what makes
deletion cascade.

## Phase 7 · Bind a PVC

This is the only step that proves data actually reaches a disk.

```bash
kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ceph-rook-verify
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: ceph-test-pool
  resources: { requests: { storage: 1Gi } }
---
apiVersion: v1
kind: Pod
metadata:
  name: ceph-rook-verify
spec:
  restartPolicy: Never
  containers:
    - name: w
      image: alpine:3.21
      command: ["sh","-c"]
      args:
        - |
          dd if=/dev/urandom of=/data/blob bs=1M count=64 2>/dev/null
          sha256sum /data/blob | cut -d' ' -f1 > /data/sum
          sync
          want=$(cat /data/sum); have=$(sha256sum /data/blob | cut -d' ' -f1)
          [ "$want" = "$have" ] || { echo MISMATCH; exit 1; }
          df -h /data | tail -1
          echo PLUGIN-VERIFY-OK
      volumeMounts: [{ name: v, mountPath: /data }]
  volumes:
    - { name: v, persistentVolumeClaim: { claimName: ceph-rook-verify } }
YAML

kubectl --context k3d-fundament-plugin wait --for=jsonpath='{.status.phase}'=Succeeded \
  pod/ceph-rook-verify --timeout=5m
kubectl --context k3d-fundament-plugin logs pod/ceph-rook-verify
```

`PLUGIN-VERIFY-OK` is the end-to-end pass. If the PVC stays `Pending`:

```bash
kubectl --context k3d-fundament-plugin describe pvc ceph-rook-verify | tail -20
kubectl --context k3d-fundament-plugin -n rook-ceph logs -l app=csi-rbdplugin-provisioner --tail=50
```

Clean up before phase 8, so the pool can be deleted later:

```bash
kubectl --context k3d-fundament-plugin delete pod/ceph-rook-verify pvc/ceph-rook-verify
```

## Phase 8 · Regression checks

These target behaviours that were wrong before review and are covered by unit tests. Run
them to confirm they hold against a real API server.

### 8a · The console is actually served

Previously the pages were embedded but never routed, and the iframe 404'd.

```bash
kubectl --context k3d-fundament-plugin -n plugin-system--ceph-rook \
  port-forward deploy/plugin 8080:8080 &
curl -sS -o /dev/null -w '%{http_code}\n' localhost:8080/console/storagepools-list.html
curl -sS -o /dev/null -w '%{http_code}\n' localhost:8080/console/storagepools-create.html
kill %1
```

Both must return `200`. Then open the console UI and confirm the pages render **styled**
and that clicking a pool row navigates — the previous version's inline styles and
`onclick` were silently dropped by the plugin CSP:

```
https://console.fundament.localhost:8443/
```

The console reaches the plugin through plugin-proxy in the `k3d-fundament` platform cluster,
which needs a
kubeconfig for the sandbox. Create it once per cluster pair, with the root recipe:

```bash
just plugin-sandbox-kubeconfig
```

The recipe restarts plugin-proxy and kube-api-proxy and waits until the switch is confirmed,
ending with `- kube-api-proxy is proxying to the sandbox (mock disabled)`.

The Secret is mounted optionally, so plugin-proxy starts and reports healthy without it and
fails only when a plugin page is opened. A blank or erroring plugin iframe with a healthy
plugin pod usually means this step is missing, or that the sandbox cluster was recreated
since it last ran.

Check the browser console for CSP violations. There should be none.

### 8a-2 · Pool editing, deletion and disk details

These exercise the console CRUD pages. Keep devtools open throughout — a CSP
violation anywhere here is a failure, not cosmetic.

1. Open a StoragePool → **Edit** → change replication → **Save**. The detail view
   returns and `status.replicas` reflects the new value.
2. **Edit** again → uncheck a disk. The OSD-retirement warning must be visible.
   Save, then confirm `status.selectedDiskCount` drops and the device leaves
   `spec.storage.nodes`:

   ```bash
   kubectl --context k3d-fundament-plugin -n rook-ceph get cephcluster rook-ceph \
     -o jsonpath='{.spec.storage.nodes}' | jq .
   ```

   The OSD pod for that device is still `Running`. That is correct, not a bug.
3. **Edit** → uncheck every disk → Save is refused with "Select at least one disk."
4. Bind a PVC to the pool's StorageClass, then **Delete**. It must be refused,
   naming the volume, with only a Close button.
5. Delete the PVC, then **Delete** → type the pool name → confirm. The pool, its
   StorageClass and its CephBlockPool all disappear.
6. Open **Disks** → click a path. The detail page shows every field, and
   `Claimed by` is set for a pooled disk (plain text — cross-kind links are not
   expressible in the host contract).

### 8b · `claimedBy` updates immediately

It used to wait for the discovery daemon's next sweep (~60m), long enough to hand one disk
to two pools through the UI.

```bash
kubectl --context k3d-fundament-plugin get disks \
  -o jsonpath='{range .items[*]}{.metadata.name}{" claimedBy="}{.status.claimedBy}{"\n"}{end}'
```

Every disk in `test-pool` must already show `claimedBy=test-pool` — no waiting. This is
also what makes the create form's picker correct, so re-open it and confirm it offers no
disks.

### 8c · Deleting a pool shrinks the CephCluster

Nothing else recomputes this: the CephCluster carries no owner reference.

```bash
kubectl --context k3d-fundament-plugin -n rook-ceph get cephcluster rook-ceph \
  -o jsonpath='{.spec.storage.nodes}' | jq .      # 3 devices

kubectl --context k3d-fundament-plugin delete storagepool test-pool

kubectl --context k3d-fundament-plugin -n rook-ceph get cephcluster rook-ceph \
  -o jsonpath='{.spec.storage.nodes}' | jq .      # now empty
kubectl --context k3d-fundament-plugin get storageclass ceph-test-pool   # NotFound (cascade)
kubectl --context k3d-fundament-plugin -n rook-ceph get cephblockpool ceph-test-pool  # NotFound
```

:::caution[The OSDs stay]
The devices leave the CephCluster, but Rook does **not** retire the OSDs — that needs an
explicit Ceph purge. `kubectl -n rook-ceph get pods -l app=rook-ceph-osd` will still show
them. This is expected and documented; it is why disk removal is a two-step operation.
:::

### 8d · Foreign objects are not adopted

The old code would adopt a same-named StorageClass and delete it when the pool went away.

This check runs last on purpose. The guard only fires once the reconciler actually reaches
the StorageClass write, which needs the pool to resolve at least one disk — a pool with
`disks: []` goes `Degraded` with `no disks selected` and never gets that far. Step 8c freed
the disks, so they can be reused here.

```bash
kubectl --context k3d-fundament-plugin apply -f - <<'YAML'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ceph-squatter
provisioner: rancher.io/local-path
YAML

DISKS=$(kubectl --context k3d-fundament-plugin get disks -o jsonpath='{range .items[*]}      - {.metadata.name}{"\n"}{end}')
kubectl --context k3d-fundament-plugin apply -f - <<YAML
apiVersion: storage.fundament.io/v1alpha1
kind: StoragePool
metadata:
  name: squatter
spec:
  replication: auto
  disks:
$DISKS
YAML

kubectl --context k3d-fundament-plugin get storagepool squatter -o jsonpath='{.status.phase}{"\n"}{.status.message}{"\n"}'
```

Expect `Degraded`, and a message naming the conflict:

```
apply StorageClass ceph-squatter: terminal error: StorageClass "ceph-squatter" already
exists and is not owned by this StoragePool; refusing to adopt it (deleting the pool would
then delete that object). Rename the StoragePool or remove the conflicting StorageClass
```

Critically, the foreign object must be untouched:

```bash
kubectl --context k3d-fundament-plugin get storageclass ceph-squatter \
  -o jsonpath='{.provisioner}{" owners="}{.metadata.ownerReferences}{"\n"}'
# rancher.io/local-path owners=      <- unchanged, no owner reference

kubectl --context k3d-fundament-plugin delete storagepool squatter
kubectl --context k3d-fundament-plugin get storageclass ceph-squatter   # must still exist
kubectl --context k3d-fundament-plugin delete storageclass ceph-squatter
```

## Phase 9 · Teardown

Order matters. Ceph consumers must release volumes while Ceph is still running to service
the unmounts:

```bash
just plugins uninstall system--ceph-rook
just plugins cluster-delete    # drains first
just plugins storage-disks purge       # detach and delete the backing images, freeing the space
```

To keep the disks for a future run, use `just plugins storage-disks reset` instead of `purge`.

`cluster-stop` and `cluster-delete` drain the node first, which evicts pods — a bare
verification pod is deleted, not restarted. Only its PersistentVolumeClaim survives a
stop/start cycle.

## When the platform cluster changes under you

Both failure modes below are described in full, with recovery, under
[Install a plugin → Known issues on this path](./install-a-plugin.md#known-issues-on-this-path).
That page is canonical; this section adds only what is specific to a mid-runbook redeploy.

A `just dev` at the repository root re-seeds the platform database and drops every published
`PluginDefinition`, so it invalidates what phase 3 published. `just plugins status` will not tell
you — the CR keeps reporting `Running`/`READY=true`. The controller log is where it shows:

```bash
kubectl --context k3d-fundament-plugin -n fundament logs deploy/fundament-plugin-controller | tail
# fetch definition: GetPluginDefinition RPC: not_found: plugin definition ceph-rook@v0.1.0 not found
```

Redo phase 3, then delete and re-apply the `PluginInstallation` with the new hash —
`definitionRef` is immutable, so it cannot be edited in place. Deleting runs the uninstall path,
which for `ceph-rook` tears down the CephCluster; on a verification run that is usually what you
want anyway, but do not do it to a pool holding data you care about.

If publishing then fails with `permission_denied` while both checks in phase 3 look right —
the `admin`/`accepted` row present and both OpenFGA tuples present — the OpenFGA store was
recreated during the reset and the tuples belong to the previous one:

```bash
kubectl --context k3d-fundament -n fundament exec db-1 -c postgres -- \
  psql -U postgres -d openfga -c "SELECT store, count(*) FROM tuple GROUP BY 1;"
kubectl --context k3d-fundament -n fundament exec db-1 -c postgres -- \
  psql -U postgres -d openfga -c "SELECT id FROM store;"
```

The two store ids must match. When they do not, every authorization decision resolves to
false while every pod looks healthy. Restarting `organization-api`, `kube-api-proxy` or
`authz-worker` does not repair it — the tuples are already written and the outbox is
drained. Another platform redeploy does.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `REFUSING: the Docker host is not a virtual machine`, `Detected: unknown`, on macOS | VM probes could not answer | Fixed — an aarch64 guest has no DMI and often no systemd, so detection now also reads the device tree and the virtio bus. If it recurs, `just plugins storage-disks doctor` prints what was detected |
| `losetup: unrecognized option: show` (or `: j`) | Docker host ships BusyBox, not util-linux | Fixed — association no longer uses `--show`/`-j`. OrbStack and colima both ship BusyBox |
| `attach` exits 1 printing nothing | A failing test as a loop body's last command, under `set -e` | Fixed. If a similar silent exit appears, re-run with `bash -x` |
| Smoke test times out; events show `modprobe args: [ceph]` repeating | No `ceph` kernel module — CephFS cannot mount | Not a failure. The plugin is block-only; the smoke test now skips CephFS automatically |
| `rbd: mapping succeeded but /dev/rbd0 is not accessible` **once** | udev race under `--options noudev` | Ignore — kubelet retries. Only a concern if it repeats |
| `no catalog entry for "ceph-rook"` | Appstore seed not applied | Re-run `db-migrations` against `k3d-fundament` |
| Publish fails with a permission error | Publishing as a user who is not an admin of the owning (`system`) org | Use a `platform-admin@fundament.io` token — see the caution in phase 3 |
| No `Disk` CRs, ConfigMap has devices | Filter rejected them | `DEV_LOOP_DEVICES: "true"` needs `type: part` on `/dev/loopNpN` |
| OSD prepare job: `unsupported diskType loop` | `allowLoopDevices` did not take | Confirm `DEV_LOOP_DEVICES` is set on the PluginInstallation |
| Mons never reach quorum | 3 mons on one node | Set `MON_COUNT: "1"` and `ALLOW_MULTIPLE_PER_NODE: "true"` |
| CephCluster rejected on version | Ceph release outside Rook's table | Only if you overrode `CEPH_IMAGE`: set `ALLOW_UNSUPPORTED_CEPH: "true"` (default `false`) |
| `csi-rbdplugin` CrashLoopBackOff | `rbd` kernel module missing | `just plugins storage-disks doctor`; use colima |
| PVC Pending, provisioner logs quiet | No OSD is up | `kubectl -n rook-ceph get pods -l app=rook-ceph-osd` |
| `rbd: mapping succeeded but /dev/rbd0 is not accessible` | No `/dev` bind | Recreate with `just plugins cluster-create-storage` |
| Second install fails after a first attempt | Stale OSD metadata | `just plugins storage-disks reset` |
| `PluginInstallation` never leaves `Deploying`, no plugin namespace appears | No plugin-controller in the sandbox | `just plugins dev` (phase 1) |
| `GetPluginDefinition … not found` after a platform redeploy | `db.reset: true` wiped the published definition | Redo phase 3 — see "When the platform cluster changes under you" |
| `permission_denied` on publish, but both phase 3 checks look right | OpenFGA store recreated; tuples in the old store | Redeploy the platform again — same section |
| Plugin iframe blank in the console, plugin pod healthy | `plugin-sandbox-kubeconfig` Secret missing or stale | `just plugin-sandbox-kubeconfig` from the root, then restart plugin-proxy |
| Node container will not stop | Stale RBD mapping | `just plugins storage-disks unmap-stale` |
| Pool stuck `Provisioning` | CephBlockPool not Ready | `kubectl -n rook-ceph describe cephblockpool ceph-<pool>` |
| Pool `Degraded` | Conflict or drift | Read `status.message` — it names the action |

## Related

- [Block devices for k3d](../fundament/k3d-block-devices.md) — how the loop devices work, persistence, cleanup
- [Example: Ceph Storage (Rook)](./example-ceph-rook.md) — architecture, replication, RBAC rationale
