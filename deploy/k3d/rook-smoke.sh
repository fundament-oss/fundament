#!/usr/bin/env bash
# Rook Ceph on the loop devices from storage-disks.sh, using the upstream chart
# and a hand-written CephCluster. No fundament plugin involved, so it separates
# a broken environment from a broken plugin.
#
# Run without arguments for the commands.
set -euo pipefail

CLUSTER="${CLUSTER:-fundament-plugin}"
KUBE_CONTEXT="${KUBE_CONTEXT:-k3d-${CLUSTER}}"
NS="${NS:-rook-ceph}"
ROOK_VERSION="${ROOK_VERSION:-v1.16.0}"
# reef (v18.x) arm64 images segfault immediately -- `ceph-osd --version` exits
# 139 in a plain docker run. Squid (v19) is the oldest line that runs on Apple
# Silicon, and Rook v1.16 supports it.
CEPH_IMAGE="${CEPH_IMAGE:-quay.io/ceph/ceph:v19.2.3}"

HERE="$(cd "$(dirname "$0")" && pwd)"
KUBECTL=(kubectl --context "$KUBE_CONTEXT")

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m /!\\\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERR\033[0m %s\n' "$*" >&2; exit 1; }

# The server node specifically: /dev is bound there (nodeFilters [server:0]), so
# that is the only node whose OSDs can see the loop devices. Picking items[0]
# would follow node ordering and can land on a stale or unrelated node.
node_name() {
    local n="k3d-${CLUSTER}-server-0"
    "${KUBECTL[@]}" get node "$n" >/dev/null 2>&1 || die "node $n not found in $KUBE_CONTEXT"
    printf '%s' "$n"
}

up() {
    local node devices
    node="$(node_name)"
    devices="$(CLUSTER="$CLUSTER" "$HERE"/storage-disks.sh devices)"
    [ -n "$devices" ] || die "no loop devices attached -- run: just storage-disks attach"
    log "node $node, devices: $(echo "$devices" | tr '\n' ' ')"

    log "installing the rook-ceph operator $ROOK_VERSION (allowLoopDevices=true)"
    helm --kube-context "$KUBE_CONTEXT" upgrade --install rook-ceph rook-ceph \
        --repo https://charts.rook.io/release --version "$ROOK_VERSION" \
        --namespace "$NS" --create-namespace \
        --set allowLoopDevices=true \
        --set enableDiscoveryDaemon=true \
        --wait --timeout 10m

    # Full paths, matching what the plugin's BuildStorageNodes emits.
    local device_yaml=""
    while read -r dev; do
        [ -n "$dev" ] && device_yaml="${device_yaml}
          - name: \"/dev/${dev}\""
    done <<<"$devices"

    log "creating the CephCluster"
    "${KUBECTL[@]}" apply -f - <<YAML
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: $NS
spec:
  dataDirHostPath: /var/lib/rook
  cephVersion:
    image: $CEPH_IMAGE
    allowUnsupported: true
  mon:
    count: 1
    allowMultiplePerNode: true
  mgr:
    count: 1
    allowMultiplePerNode: true
  dashboard:
    enabled: false
  crashCollector:
    disable: true
  skipUpgradeChecks: true
  monitoring:
    enabled: false
  storage:
    # A k3d node is privileged and enumerates the Docker host's real disks.
    # Never enable useAllDevices here.
    useAllNodes: false
    useAllDevices: false
    nodes:
      - name: "$node"
        devices:$device_yaml
  cephConfig:
    global:
      osd_pool_default_size: "1"
      mon_warn_on_pool_no_redundancy: "false"
      mon_data_avail_warn: "10"
      bdev_flock_retry: "20"
      osd_memory_target: "2147483648"
---
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: builtin-mgr
  namespace: $NS
spec:
  name: .mgr
  replicated:
    size: 1
    requireSafeReplicaSize: false
YAML

    log "waiting for an OSD (first run pulls the ceph image)"
    local i
    for i in $(seq 1 120); do
        # Ready, not phase==Running: a CrashLoopBackOff OSD is also Running.
        if "${KUBECTL[@]}" -n "$NS" get pod -l app=rook-ceph-osd \
            -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' \
            2>/dev/null | grep -q True; then
            log "OSD is up"
            break
        fi
        [ "$i" = 120 ] && {
            warn "no OSD after 10 minutes. Check the prepare job:"
            warn "  kubectl -n $NS logs -l app=rook-ceph-osd-prepare --tail=50"
            warn "'unsupported diskType loop' means allowLoopDevices did not take."
            return 1
        }
        sleep 5
    done

    log "creating replicapool + the rook-ceph-block StorageClass"
    "${KUBECTL[@]}" apply -f - <<YAML
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: $NS
spec:
  failureDomain: osd
  replicated:
    size: 1
    requireSafeReplicaSize: false
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-ceph-block
provisioner: ${NS}.rbd.csi.ceph.com
parameters:
  clusterID: $NS
  pool: replicapool
  imageFormat: "2"
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: $NS
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: $NS
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: $NS
  csi.storage.k8s.io/fstype: ext4
  # krbd retries OSD requests forever by default, so if the cluster goes away
  # while a volume is mounted the filesystem's journal thread blocks
  # uninterruptibly and the device can never be unmapped -- on bare metal that
  # costs a reboot. A timeout turns that into an I/O error the filesystem can
  # fail on. Dev only: it aborts all requests, not just at unmap time.
  mapOptions: "krbd:osd_request_timeout=90"
  unmapOptions: "krbd:force"
allowVolumeExpansion: true
reclaimPolicy: Delete
YAML

    log "creating the myfs filesystem + the rook-cephfs StorageClass"
    "${KUBECTL[@]}" apply -f - <<YAML
apiVersion: ceph.rook.io/v1
kind: CephFilesystem
metadata:
  name: myfs
  namespace: $NS
spec:
  metadataPool:
    replicated: { size: 1, requireSafeReplicaSize: false }
  dataPools:
    - name: replicated
      replicated: { size: 1, requireSafeReplicaSize: false }
  preserveFilesystemOnDelete: false
  metadataServer:
    activeCount: 1
    activeStandby: false
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: rook-cephfs
provisioner: ${NS}.cephfs.csi.ceph.com
parameters:
  clusterID: $NS
  fsName: myfs
  pool: myfs-replicated
  csi.storage.k8s.io/provisioner-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: $NS
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-cephfs-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: $NS
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-cephfs-node
  csi.storage.k8s.io/node-stage-secret-namespace: $NS
allowVolumeExpansion: true
reclaimPolicy: Delete
YAML

    "${KUBECTL[@]}" -n "$NS" apply -f \
        "https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/toolbox.yaml" \
        >/dev/null 2>&1 || warn "toolbox not applied (offline?)"

    log "done -- try: $0 status && $0 test"
}

status() {
    "${KUBECTL[@]}" -n "$NS" exec -it deploy/rook-ceph-tools -- ceph -s 2>/dev/null \
        || { warn "toolbox not ready; pod status instead"; "${KUBECTL[@]}" -n "$NS" get pod; }
}

smoke() {
    log "provisioning an RBD and a CephFS volume, then verifying what was written"
    "${KUBECTL[@]}" apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rook-smoke-rbd
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: rook-ceph-block
  resources: { requests: { storage: 1Gi } }
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rook-smoke-cephfs
spec:
  accessModes: [ReadWriteMany]
  storageClassName: rook-cephfs
  resources: { requests: { storage: 1Gi } }
---
apiVersion: v1
kind: Pod
metadata:
  name: rook-smoke
spec:
  restartPolicy: Never
  containers:
    - name: w
      image: alpine:3.21
      command: ["sh","-c"]
      args:
        - |
          for v in /rbd /cephfs; do
            dd if=/dev/urandom of=$v/data.bin bs=1M count=64 2>/dev/null
            sha256sum $v/data.bin | cut -d' ' -f1 > $v/data.sha256
            sync
            want=$(cat $v/data.sha256); have=$(sha256sum $v/data.bin | cut -d' ' -f1)
            [ "$want" = "$have" ] || { echo "MISMATCH on $v"; exit 1; }
            df -h $v | tail -1
          done
          echo SMOKE-OK
      volumeMounts:
        - { name: rbd, mountPath: /rbd }
        - { name: cephfs, mountPath: /cephfs }
  volumes:
    - { name: rbd, persistentVolumeClaim: { claimName: rook-smoke-rbd } }
    - { name: cephfs, persistentVolumeClaim: { claimName: rook-smoke-cephfs } }
YAML
    # The pod runs to completion, so it never reports Ready.
    if ! "${KUBECTL[@]}" wait --for=jsonpath='{.status.phase}'=Succeeded \
        pod/rook-smoke --timeout=5m; then
        "${KUBECTL[@]}" describe pod rook-smoke | tail -30
        "${KUBECTL[@]}" logs pod/rook-smoke || true
        return 1
    fi
    "${KUBECTL[@]}" logs pod/rook-smoke
    "${KUBECTL[@]}" delete pod/rook-smoke pvc/rook-smoke-rbd pvc/rook-smoke-cephfs --ignore-not-found
}

# Long-running counterpart to `test`: keeps volumes bound and re-verifies a
# checksum, so restarts of the cluster, the Docker VM or the machine can be
# checked for data loss. Survives restarts by being a Deployment.
soak() {
    log "starting the soak workload on an RBD and a CephFS volume"
    "${KUBECTL[@]}" apply -f - <<'YAML'
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: rook-soak-rbd }
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: rook-ceph-block
  resources: { requests: { storage: 2Gi } }
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: { name: rook-soak-cephfs }
spec:
  accessModes: [ReadWriteMany]
  storageClassName: rook-cephfs
  resources: { requests: { storage: 2Gi } }
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: rook-soak }
spec:
  replicas: 1
  strategy: { type: Recreate }
  selector: { matchLabels: { app: rook-soak } }
  template:
    metadata: { labels: { app: rook-soak } }
    spec:
      containers:
        - name: worker
          image: alpine:3.21
          command: ["sh","-c"]
          args:
            - |
              for v in /rbd /cephfs; do
                if [ ! -f $v/data.bin ]; then
                  dd if=/dev/urandom of=$v/data.bin bs=1M count=100 2>/dev/null
                  sha256sum $v/data.bin | cut -d' ' -f1 > $v/data.sha256
                  sync
                  echo "INIT $v sha=$(cat $v/data.sha256)"
                fi
              done
              n=0
              while true; do
                n=$((n+1))
                for v in /rbd /cephfs; do
                  want=$(cat $v/data.sha256 2>/dev/null)
                  have=$(sha256sum $v/data.bin 2>/dev/null | cut -d' ' -f1)
                  if [ -z "$have" ]; then echo "READFAIL $v pass=$n"
                  elif [ "$want" = "$have" ]; then echo "OK $v pass=$n"
                  else echo "MISMATCH $v pass=$n want=$want have=$have"; fi
                  echo "pass $n $(date -u +%FT%TZ)" >> $v/writes.log
                done
                sleep 20
              done
          volumeMounts:
            - { name: rbd, mountPath: /rbd }
            - { name: cephfs, mountPath: /cephfs }
      volumes:
        - { name: rbd, persistentVolumeClaim: { claimName: rook-soak-rbd } }
        - { name: cephfs, persistentVolumeClaim: { claimName: rook-soak-cephfs } }
YAML
    "${KUBECTL[@]}" rollout status deploy/rook-soak --timeout=5m
    log "watch it with: $0 soak-log"
}

soak_log() {
    "${KUBECTL[@]}" logs deploy/rook-soak --tail=20
    echo
    log "any failure so far:"
    "${KUBECTL[@]}" logs deploy/rook-soak 2>/dev/null | grep -cE 'MISMATCH|READFAIL' | sed 's/^/  failures: /'
}

# Takes one resource/name argument, so it accepts `kubectl get -o name` output
# directly. Passing kind and name separately alongside that form makes kubectl
# reject the call client-side.
unfinalize() {
    "${KUBECTL[@]}" -n "$NS" patch "$1" --type merge \
        -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
}

# The CephCluster must go before the operator, or Rook's cleanup job never runs.
# Rook's finalizers are cleared by the operator, so anything still holding one
# once the operator is gone strands the namespace in Terminating.
down() {
    "${KUBECTL[@]}" delete deploy/rook-soak --ignore-not-found >/dev/null 2>&1 || true
    "${KUBECTL[@]}" delete pod/rook-smoke --ignore-not-found >/dev/null 2>&1 || true
    "${KUBECTL[@]}" delete pvc/rook-smoke-rbd pvc/rook-smoke-cephfs pvc/rook-soak-rbd pvc/rook-soak-cephfs \
        --ignore-not-found >/dev/null 2>&1 || true
    "${KUBECTL[@]}" delete storageclass rook-ceph-block rook-cephfs --ignore-not-found
    log "deleting the CephCluster so Rook can clean up"
    if ! "${KUBECTL[@]}" -n "$NS" delete cephcluster rook-ceph --ignore-not-found --timeout=2m; then
        warn "delete timed out (no OSD ever came up?), clearing the finalizer"
        unfinalize cephcluster/rook-ceph
    fi
    # `get` fails when the CRD is absent -- a second run, or a run after `up`
    # failed before helm installed it -- and pipefail would abort the teardown.
    "${KUBECTL[@]}" -n "$NS" get cephblockpool -o name 2>/dev/null | while read -r p; do
        "${KUBECTL[@]}" -n "$NS" delete "$p" --ignore-not-found --timeout=1m || unfinalize "$p"
    done || true
    helm --kube-context "$KUBE_CONTEXT" -n "$NS" uninstall rook-ceph 2>/dev/null || true

    unfinalize configmap/rook-ceph-mon-endpoints
    unfinalize secret/rook-ceph-mon
    "${KUBECTL[@]}" delete ns "$NS" --ignore-not-found --timeout=3m || true
    log "done -- run 'just storage-disks reset' before reinstalling"
}

usage() {
    cat <<EOF
Rook Ceph on the loop devices from storage-disks.sh, using the upstream chart
and a hand-written CephCluster. No fundament plugin involved, so it separates
"the environment is broken" from "the plugin is broken".

  up        install the operator, a single-node CephCluster and both StorageClasses
  status    ceph -s
  test      provision an RBD and a CephFS volume, write and verify a checksum
  soak      run a workload that keeps re-verifying both volumes, for restart testing
  soak-log  recent soak output plus a failure count
  down      remove everything (leaves the disks alone)

Environment: CLUSTER=$CLUSTER NS=$NS ROOK_VERSION=$ROOK_VERSION
             CEPH_IMAGE=$CEPH_IMAGE

Occupies the $NS namespace, so run 'down' before installing the ceph-rook plugin.
EOF
}

case "${1:-}" in
    up)     up ;;
    status) status ;;
    test)     smoke ;;
    soak)     soak ;;
    soak-log) soak_log ;;
    down)     down ;;
    "")     usage ;;
    *)      printf 'unknown command: %s\n\n' "$1" >&2; usage >&2; exit 1 ;;
esac
