#!/usr/bin/env bash
# Loop-backed block devices for a k3d cluster, so Rook Ceph has raw devices to
# consume: its only OSD backend is BlueStore, which writes to a block device.
#
#   file  --losetup-->  /dev/loopN  --GPT-->  /dev/loopNp1  -->  one Ceph OSD
#
#     /var/lib/docker/volumes/<vol>/_data/osdN.img   backing file
#       -> /dev/loopN, /dev/loopNp1                  kernel loop device
#         -> k3d node container (/dev is bind-mounted in, live)
#           -> Rook OSD pod (hostPath /dev)
#
# Run without arguments for the commands. See docs/k3d-block-devices.md for the
# workflow, persistence and cleanup.
#
# TODO: single-node only. The sandbox is servers:1/agents:0 and this assumes it:
#   1. `reset` clears /var/lib/rook on $NODE only, so agents would keep the
#      stale Rook state that reset exists to remove.
#   2. doctor/status/attach inspect $NODE only, so a missing /run/udev or an
#      invisible device on an agent goes unreported.
#   3. plugins/sandbox/k3d-config.yaml binds /dev and /run/udev with
#      nodeFilters [server:0], so a cluster created with agents:2 leaves them
#      unbound there (`k3d node create` does bind them, so the routes disagree).
# Fix: enumerate nodes with `docker ps --filter label=k3d.cluster=$CLUSTER`
# restricted to roles server/agent, loop over them, and widen the nodeFilters to
# `all`. Loop devices are global kernel objects, so every node sees every disk
# either way; a device-to-node convention would have to live in the plugin's
# disk inventory or the docs, not here.
set -euo pipefail

CLUSTER="${CLUSTER:-fundament-plugin}"
DISK_COUNT="${DISK_COUNT:-3}"
DISK_SIZE="${DISK_SIZE:-20G}"
DISK_VOLUME="${DISK_VOLUME:-fundament-k3d-disks}"
HELPER_IMAGE="${HELPER_IMAGE:-alpine:3.21}"

NODE="k3d-${CLUSTER}-server-0"
NATIVE="${NATIVE:-0}"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m /!\\\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERR\033[0m %s\n' "$*" >&2; exit 1; }

# Loop devices and kernel modules live in the kernel the Docker daemon runs on:
# the VM on macOS, the machine itself on Linux. --pid=host enters that kernel's
# PID 1 regardless of backend. --native is for rootless Docker, where the
# container's PID 1 is not the host's.
host_exec() {
    if [ "$NATIVE" = 1 ]; then
        sudo sh -s
    else
        docker run --rm -i --privileged --pid=host "$HELPER_IMAGE" \
            nsenter -t 1 -m -u -i -n sh -s
    fi
}

# Backing files go in a Docker volume so they land on the Docker data disk,
# which is where a backend puts its bulk storage. A path under /var/lib would
# instead land on the root filesystem, which is typically far smaller.
disk_dir() {
    docker volume create "$DISK_VOLUME" >/dev/null
    docker volume inspect "$DISK_VOLUME" --format '{{.Mountpoint}}'
}

require_docker() {
    command -v docker >/dev/null || die "docker not found"
    docker info >/dev/null 2>&1 || die "Docker daemon not reachable"
}

doctor() {
    require_docker
    local dir; dir="$(disk_dir)"
    log "Docker host kernel"
    local kernel
    kernel="$(printf '%s\n' "
        echo \"  kernel:  \$(uname -r)  \$(uname -m)\"
        for m in rbd ceph libceph loop; do
            if [ -d /sys/module/\$m ]; then s='PRESENT (loaded or built-in)'
            elif modprobe \$m 2>/dev/null; then s='OK (loaded just now)'
            else s='MISSING'; fi
            printf '  %-8s %s\n' \"\$m\" \"\$s\"
        done
        if [ -d /run/udev ]; then echo '  udev:    /run/udev present'
        else echo '  udev:    /run/udev MISSING (ceph-volume may warn)'; fi
        if command -v sfdisk >/dev/null 2>&1; then echo '  sfdisk:  present'
        else echo '  sfdisk:  MISSING (partitioning falls back to a container)'; fi
        echo '  --- space for the OSD images ---'
        df -h '$dir' 2>/dev/null | tail -1 | sed 's/^/  /'
    " | host_exec)"
    printf '%s\n' "$kernel"

    log "cluster $CLUSTER"
    if docker inspect "$NODE" >/dev/null 2>&1; then
        if docker exec "$NODE" test -d /run/udev; then
            echo "  /run/udev bound into the node: yes"
        else
            echo "  /run/udev bound into the node: NO -- add the volume to the k3d config"
        fi
        local devs; devs="$(docker exec "$NODE" sh -c 'ls /dev/loop* 2>/dev/null | tr "\n" " "' || true)"
        echo "  loop devices visible in the node: ${devs:-none}"
    else
        echo "  node container $NODE does not exist"
    fi

    if printf '%s\n' "$kernel" | grep -qE '^ +rbd +MISSING'; then
        cat <<'EOF'

  /!\ rbd is MISSING: csi-rbdplugin will CrashLoopBackOff and RBD PVCs will not
      mount. It fatals at startup, before reading a StorageClass, so
      `mounter: rbd-nbd` does not help. Ceph itself still runs -- mons, mgr and
      OSDs do not use the rbd module -- so the plugin's reconcile path stays
      testable. For working PVCs, use colima.
EOF
    fi
}

# Writes a single GPT partition and waits for the kernel to surface it.
partition() {
    local dev="$1"
    if printf '%s\n' "command -v sfdisk >/dev/null 2>&1" | host_exec; then
        printf '%s\n' "printf 'label: gpt\n,,\n' | sfdisk '$dev'" | host_exec >/dev/null \
            || die "sfdisk failed on $dev"
    else
        warn "no sfdisk on the Docker host; partitioning $dev in a container"
        docker run --rm --privileged -v /dev:/dev "$HELPER_IMAGE" sh -c \
            "apk add --no-cache sfdisk >/dev/null && printf 'label: gpt\n,,\n' | sfdisk $dev >/dev/null" \
            || die "could not partition $dev -- install util-linux on the Docker host"
    fi
    printf '%s\n' "
        n=0
        while [ ! -b '${dev}p1' ] && [ \$n -lt 10 ]; do sleep 1; n=\$((n+1)); done
        [ -b '${dev}p1' ]
    " | host_exec || die "$dev was partitioned but ${dev}p1 never appeared"
}

# The OSD device is /dev/loopNp1 (type=part), never the bare /dev/loopN:
# rook-discover does not report loop devices, so one never reaches the ConfigMap
# the plugin's disk inventory reads. See docs/k3d-block-devices.md for why the
# upstream allowance does not fix this.
attach() {
    require_docker
    local dir; dir="$(disk_dir)"
    log "backing files in $DISK_VOLUME ($DISK_COUNT x $DISK_SIZE, sparse)"

    # ceph-csi short-circuits its own modprobe when /sys/module/rbd exists, which
    # also sidesteps container-side modprobe failures (zstd, module signatures).
    local loops
    loops="$(printf '%s\n' "
        set -e
        modprobe rbd 2>/dev/null || true
        modprobe ceph 2>/dev/null || true
        mkdir -p '$dir'
        i=0
        while [ \$i -lt '$DISK_COUNT' ]; do
            img='$dir'/osd\$i.img
            [ -f \"\$img\" ] || truncate -s '$DISK_SIZE' \"\$img\"
            dev=\$(losetup -j \"\$img\" | cut -d: -f1 | head -1)
            # An association made without -P never surfaces partitions; redo it.
            if [ -n \"\$dev\" ] && [ ! -b \"\${dev}p1\" ]; then
                losetup -d \"\$dev\" 2>/dev/null && dev=''
            fi
            [ -n \"\$dev\" ] || dev=\$(losetup -P -f --show \"\$img\")
            if [ -b \"\${dev}p1\" ]; then echo \"\$dev ready\"; else echo \"\$dev raw\"; fi
            i=\$((i+1))
        done
    " | host_exec)"
    [ -n "$loops" ] || die "no loop devices were attached"

    # Herestring, not a pipe: this loop must run in the main shell so die works.
    local dev state
    while read -r dev state; do
        [ "$state" = raw ] && partition "$dev"
    done <<<"$loops"

    while read -r dev state; do
        if docker inspect "$NODE" >/dev/null 2>&1 && ! docker exec "$NODE" test -b "${dev}p1"; then
            warn "${dev}p1 is not visible in $NODE -- is /dev bound in the k3d config?"
        fi
        echo "  ${dev}p1"
    done <<<"$loops"
}

# Short kernel names of the OSD partitions, one per line. Rook must name devices
# this way: a k3d node has no /dev/disk/by-id, and rook-discover reports no
# by-id field at all.
devices() {
    require_docker
    local dir; dir="$(disk_dir)"
    printf '%s\n' "
        losetup -a 2>/dev/null | grep -F '$dir' | cut -d: -f1 | sort | while read -r d; do
            [ -b \"\${d}p1\" ] && echo \"\${d}p1\" | sed 's|/dev/||'
        done
        true
    " | host_exec
}

status() {
    require_docker
    local dir; dir="$(disk_dir)"
    log "attachments on the Docker host"
    printf '%s\n' "losetup -a 2>/dev/null | grep -F '$dir' || echo '  (none)'" | host_exec
    log "block devices in $NODE"
    if docker inspect "$NODE" >/dev/null 2>&1; then
        docker exec "$NODE" sh -c 'ls -l /dev/loop* 2>/dev/null || echo "  (none)"'
    else
        echo "  node container $NODE does not exist"
    fi
}

# Our devices that something still holds, by open fd or by mount. This has to be
# answered before detaching anything: BlueStore keeps its OSD device open, and
# losetup -d on a held device reports success while only flagging autoclear, so
# the kernel keeps both device and backing file alive. Trusting that exit status
# would detach the idle devices and leave the cluster on a half-dismantled set.
busy_devices() {
    local dir="$1"
    printf '%s\n' "
        devs=\$(losetup -a 2>/dev/null | grep -F '$dir' | cut -d: -f1)
        [ -n \"\$devs\" ] || exit 0
        held=\$( { for p in /proc/[0-9]*; do
                     for f in \"\$p\"/fd/*; do [ -L \"\$f\" ] && readlink \"\$f\"; done
                   done
                   cut -d' ' -f1 /proc/self/mounts
                 } 2>/dev/null | sort -u)
        for d in \$devs; do
            for dev in \"\$d\" \"\$d\"p1; do
                echo \"\$held\" | grep -qx \"\$dev\" && echo \"\$dev\"
            done
        done
        true
    " | host_exec
}

detach() {
    require_docker
    local dir; dir="$(disk_dir)"
    local busy; busy="$(busy_devices "$dir")"
    if [ -n "$busy" ]; then
        warn "still held by a running process or mount:"
        printf '%s\n' "$busy" | sed 's/^/      /' >&2
        warn "stop the Ceph cluster first: deploy/k3d/rook-smoke.sh down"
        return 1
    fi
    log "detaching loop devices for $dir"
    # The post-check is authoritative: it catches both a refused detach and a
    # deferred one.
    local out rc=0
    out="$(printf '%s\n' "
        losetup -a 2>/dev/null | grep -F '$dir' | cut -d: -f1 | while read -r dev; do
            losetup -d \"\$dev\" 2>/dev/null && echo \"  detached \$dev\" || echo \"  FAILED \$dev\"
        done
        losetup -a 2>/dev/null | grep -F '$dir' | sed 's/^/  still attached: /'
        losetup -a 2>/dev/null | grep -qF '$dir' && exit 1
        exit 0
    " | host_exec)" || rc=$?
    [ -n "$out" ] && printf '%s\n' "$out"
    if [ "$rc" != 0 ]; then
        warn "not everything detached"
        return 1
    fi
}

# Removes the disks for good, which nothing else does: reset recreates them and
# detach keeps the files. Detaching first is not optional -- deleting an image
# while its loop device is still attached frees no space, because the kernel
# keeps the deleted inode alive until the device goes away.
purge() {
    require_docker
    detach || die "refusing to delete the images while they are still in use"
    log "removing the docker volume $DISK_VOLUME"
    docker volume rm "$DISK_VOLUME" >/dev/null || die "could not remove $DISK_VOLUME"
    echo "  removed"
}

# Stale OSD metadata on the images, or Rook state under dataDirHostPath, is the
# usual reason a second install attempt fails.
reset() {
    require_docker
    detach || die "refusing to wipe the images while they are still in use"
    local dir; dir="$(disk_dir)"
    [ -n "$dir" ] || die "could not resolve the volume mountpoint"
    log "removing the backing files"
    printf '%s\n' "rm -f '$dir'/osd*.img; true" | host_exec
    if docker inspect "$NODE" >/dev/null 2>&1; then
        log "removing /var/lib/rook in $NODE"
        docker exec "$NODE" rm -rf /var/lib/rook || true
    fi
    attach
}

usage() {
    cat <<EOF
Loop-backed block devices for the k3d cluster '$CLUSTER'.

  doctor    check the kernel Docker actually runs on
  attach    create + attach the disks (also the post-reboot fix)
  devices   short kernel names, one per line, for scripts
  status    what is attached, and what the node can see
  detach    detach, keep the backing files
  reset     wipe the disks and Rook's on-node state, reattach
  purge     detach and delete the disks for good, freeing the space

All commands are idempotent.

  --native  run host commands via sudo instead of a helper container
            (rootless Docker, where the container's PID 1 is not the host's)

Environment: CLUSTER=$CLUSTER DISK_COUNT=$DISK_COUNT DISK_SIZE=$DISK_SIZE
             DISK_VOLUME=$DISK_VOLUME NATIVE=$NATIVE
EOF
}

args=()
for a in "$@"; do
    case "$a" in
        --native) NATIVE=1 ;;
        *) args+=("$a") ;;
    esac
done

case "${args[0]:-}" in
    doctor)  doctor ;;
    attach)  attach ;;
    devices) devices ;;
    status)  status ;;
    detach)  detach ;;
    reset)   reset ;;
    purge)   purge ;;
    "")      usage ;;
    *)       printf 'unknown command: %s\n\n' "${args[0]}" >&2; usage >&2; exit 1 ;;
esac
