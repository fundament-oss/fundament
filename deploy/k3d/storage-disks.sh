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
# Run without arguments for the commands. See docs/developer/fundament/k3d-block-devices.md for the
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
ALLOW_BARE_METAL="${ALLOW_BARE_METAL:-0}"
ROOK_NS="${ROOK_NS:-rook-ceph}"

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

# True when this machine has ever had loop-backed disks attached. The backing
# volume is created by `attach` and survives everything short of `purge`, so its
# absence means no cluster here has ever run Ceph.
#
# The lifecycle hooks (drain, uncordon) run on every `just cluster-start` and
# `just cluster-stop`, including for people who never touch storage. This is a
# cheap local query; without it those hooks would spin up a privileged helper
# container to inspect the host kernel on every start and stop.
disks_present() {
    docker volume inspect "$DISK_VOLUME" >/dev/null 2>&1
}

# True when the Docker endpoint is a local socket rather than a remote host.
# DOCKER_HOST wins because it overrides the active context.
docker_daemon_is_local() {
    local ep="${DOCKER_HOST:-}"
    [ -n "$ep" ] || ep="$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || true)"
    case "$ep" in
        ""|unix://*|npipe://*) return 0 ;;
        *) return 1 ;;
    esac
}

# The virtualisation technology the Docker host runs on, "none" for bare metal,
# or "unknown" when nothing answers.
#
# Probed inside the Docker host rather than on this machine, because DOCKER_HOST
# can point somewhere else entirely -- a Mac driving a remote bare-metal Linux
# box must still refuse.
host_virt() {
    local virt
    virt="$(printf '%s\n' '
        if command -v systemd-detect-virt >/dev/null 2>&1; then
            systemd-detect-virt 2>/dev/null || echo none
        elif [ -r /sys/class/dmi/id/product_name ]; then
            case "$(cat /sys/class/dmi/id/product_name)" in
                *Virtual*|*VMware*|*KVM*|*QEMU*|*Hyper-V*) echo vm ;;
                *) echo none ;;
            esac
        # Neither probe fires on an aarch64 guest: there is no SMBIOS/DMI to read,
        # and the minimal guests these backends ship (OrbStack, colima) have no
        # systemd. That is every Docker backend on Apple Silicon, so without the
        # two below the normal macOS setup reports "unknown" and gets refused.
        # The device tree names the hypervisor; a virtio bus only exists under
        # paravirtualisation.
        elif grep -qaiE "qemu|kvm|virt|apple|orbstack" /proc/device-tree/compatible 2>/dev/null; then
            echo device-tree
        elif [ -n "$(ls -A /sys/bus/virtio/devices 2>/dev/null)" ]; then
            echo virtio
        else
            echo unknown
        fi
    ' | host_exec | tr -d '[:space:]')"

    # Last resort: macOS cannot run Linux containers without a VM, whatever the
    # backend. Only trusted for a local daemon, and only once the in-guest
    # probes have come back inconclusive.
    if [ "$virt" = unknown ] && [ "$(uname -s)" = Darwin ] && docker_daemon_is_local; then
        virt=macos
    fi
    printf '%s\n' "$virt"
}

# Ceph in k3d puts block devices, filesystems and their kernel threads in the
# Docker host's kernel. On a VM a mistake costs a VM restart; on a workstation an
# RBD mapping can wedge unkillably (only a reboot clears it) and the privileged
# node enumerates the real drives. Rook says the same: "Always use a virtual
# machine when testing Rook. Never use your host system where local devices may
# mistakenly be consumed."
require_vm() {
    local virt; virt="$(host_virt)"
    case "$virt" in
        none|unknown) ;;
        *) log "Docker host is a VM ($virt) -- mistakes are contained to it"; return 0 ;;
    esac
    [ "$ALLOW_BARE_METAL" = 1 ] && {
        warn "bare-metal Docker host, continuing because ALLOW_BARE_METAL is set"
        return 0
    }
    cat >&2 <<EOF

  ####################################################################
  #  REFUSING: the Docker host is not a virtual machine              #
  ####################################################################

  Detected: $virt. Every device, filesystem and kernel thread Ceph
  creates lands in THIS machine's kernel.

    * a stuck RBD mapping is an unkillable kernel thread, and unless
      the StorageClass bounds its I/O the only recovery is a reboot
    * the k3d node is privileged and sees your real disks

  Rook's own guidance: "Always use a virtual machine when testing
  Rook. Never use your host system where local devices may mistakenly
  be consumed."

  Run Docker inside a VM, which is what macOS already does. If you
  accept the risk on this machine:

      ALLOW_BARE_METAL=1 $0 ${args[*]}
      just storage-disks --allow-bare-metal <command>

EOF
    exit 1
}

# Whether the Docker host's kernel has a module, loading it if it is not already
# in. Exposed as a command so rook-smoke.sh can decide what it is able to test:
# 'rbd' maps block devices and 'ceph' mounts CephFS, and a kernel can ship one
# without the other (OrbStack has rbd but not ceph).
has_module() {
    [ -n "${1:-}" ] || die "usage: has-module <name>"
    printf '%s\n' "[ -d /sys/module/$1 ] || modprobe $1 2>/dev/null" | host_exec >/dev/null 2>&1
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

    local virt; virt="$(host_virt)"
    case "$virt" in
        none)    echo "  host:    BARE METAL -- attach refuses; see docs/developer/fundament/k3d-block-devices.md" ;;
        unknown) echo "  host:    could not determine whether this is a VM" ;;
        *)       echo "  host:    virtual machine ($virt)" ;;
    esac

    log "cluster $CLUSTER"
    if docker inspect "$NODE" >/dev/null 2>&1; then
        if docker exec "$NODE" test -d /run/udev; then
            echo "  /run/udev bound into the node: yes"
        else
            echo "  /run/udev bound into the node: NO"
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
# the plugin's disk inventory reads. See docs/developer/fundament/k3d-block-devices.md for why the
# upstream allowance does not fix this.
attach() {
    require_docker
    require_vm
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
        # The Docker host may ship BusyBox losetup rather than util-linux --
        # OrbStack and colima both do -- and BusyBox has neither -j nor --show.
        # What both implementations agree on: -a prints one \"/dev/loopN: ...\"
        # line per association ending in the backing file (util-linux wraps it
        # in parens), -f prints the next free device, and -P takes -f. So the
        # file is looked up by parsing -a, and the device the kernel picked is
        # read back the same way after associating.
        # 'if', not '[ ] && ...': a failing test as the loop body's last command
        # makes the loop exit non-zero, which under set -e kills the script the
        # moment a backing file is not in the list -- i.e. every new disk.
        loop_for() {
            losetup -a 2>/dev/null | while IFS= read -r line; do
                d=\${line%%:*}
                f=\${line##* }
                f=\${f%\\)}
                f=\${f#(}
                if [ \"\$f\" = \"\$1\" ]; then echo \"\$d\"; break; fi
            done
        }
        i=0
        while [ \$i -lt '$DISK_COUNT' ]; do
            img='$dir'/osd\$i.img
            [ -f \"\$img\" ] || truncate -s '$DISK_SIZE' \"\$img\"
            dev=\$(loop_for \"\$img\")
            # An association made without -P never surfaces partitions; redo it.
            if [ -n \"\$dev\" ] && [ ! -b \"\${dev}p1\" ]; then
                losetup -d \"\$dev\" 2>/dev/null && dev=''
            fi
            if [ -z \"\$dev\" ]; then
                losetup -P -f \"\$img\"
                dev=\$(loop_for \"\$img\")
                [ -n \"\$dev\" ] || { echo \"could not associate \$img\" >&2; exit 1; }
            fi
            # The kernel creates partition devices asynchronously after losetup
            # -P. Deciding too early reports an already-partitioned image as raw
            # and rewrites its GPT over live OSD data.
            n=0
            while [ ! -b \"\${dev}p1\" ] && [ \$n -lt 5 ]; do sleep 1; n=\$((n+1)); done
            if [ -b \"\${dev}p1\" ]; then echo \"\$dev ready\"; else echo \"\$dev raw\"; fi
            i=\$((i+1))
        done
    " | host_exec)"
    [ -n "$loops" ] || die "no loop devices were attached"

    # Herestring, not a pipe: this loop must run in the main shell so die works.
    # 'if', not '[ ] && ...': the latter leaves the loop's exit status at 1 when
    # the last device is already partitioned, and set -e then aborts a second
    # (idempotent, no-op) attach with no message at all.
    local dev state
    while read -r dev state; do
        if [ "$state" = raw ]; then partition "$dev"; fi
    done <<<"$loops"

    while read -r dev state; do
        echo "  ${dev}p1"
    done <<<"$loops"

    require_node_bind
}

# Without the bind a node's /dev is a snapshot taken at container start. Testing
# for a device inside the node is not enough: the snapshot may already hold one
# from before, and Ceph CSI would still fail later because it creates /dev/rbdN
# only when a volume is mounted.
#
# Nor is the mount type a usable signal. It looks like it should be -- devtmpfs
# for the real thing, tmpfs for a snapshot -- but a backend whose own /dev is a
# tmpfs (OrbStack) reports tmpfs inside the node either way. The bind list on
# the container definition is what actually distinguishes them.
require_node_bind() {
    docker inspect "$NODE" >/dev/null 2>&1 || return 0
    # Read the bind from the container definition rather than from inside it:
    # a stopped node cannot be exec'd into, and that is not the same as a node
    # without the bind.
    docker inspect "$NODE" --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' 2>/dev/null \
        | grep -qx '/dev' && return 0
    die "$NODE has no /dev bind, so the cluster cannot use these disks.
      Recreate it: cd plugins && just cluster-delete && just cluster-create-storage"
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

# Ceph consumers must let go of their volumes while Ceph is still running to
# service the unmounts -- Rook's documented order. kubectl drain waits for the
# pods to actually be gone, which is what forces the teardown, and cordons so
# nothing reschedules onto a cluster that is about to stop. The selector keeps
# the Ceph daemons; the CSI pods carry no rook_cluster label, so what keeps
# csi-rbdplugin -- which has to be there to do NodeUnstageVolume -- is
# --ignore-daemonsets.
# Safe to call on any cluster: a mapped device is the only thing that makes
# stopping dangerous, so with none the drain is skipped entirely rather than
# evicting the workloads of a cluster that never used storage.
drain() {
    command -v docker >/dev/null && docker info >/dev/null 2>&1 || return 0
    disks_present || return 0
    local ctx="k3d-$CLUSTER" mapped
    mapped="$(printf '%s\n' "ls /dev/rbd[0-9]* 2>/dev/null | tr '\\n' ' '" | host_exec 2>/dev/null)" || return 0
    [ -n "$mapped" ] || return 0

    kubectl --context "$ctx" get node "$NODE" >/dev/null 2>&1 || {
        warn "$mapped still mapped but $CLUSTER is unreachable, so nothing can unmount it.
      The node will not stop until you run: just storage-disks unmap-stale"
        return 0
    }
    log "draining Ceph consumers from $NODE"
    kubectl --context "$ctx" drain "$NODE" \
        --pod-selector="rook_cluster!=$ROOK_NS" \
        --ignore-daemonsets --delete-emptydir-data --force --timeout=2m \
        || warn "drain reported errors; the device check below is what decides"

    local left
    left="$(printf '%s\n' "ls /dev/rbd[0-9]* 2>/dev/null | tr '\\n' ' '" | host_exec)"
    [ -z "$left" ] || die "still mapped after draining: $left
      Something outside this cluster holds a Ceph volume. Stopping now would
      wedge the Docker host (see docs/developer/fundament/k3d-block-devices.md)."
    log "no Ceph volumes are mapped; safe to stop"
}

# After a stop that bypassed the drain, the mapping is what keeps the node
# container from exiting: rbd retries re-registering its watch, a wait with no
# timeout of its own. Removing the device ends it, and the container then stops
# at once. The write blocks until teardown finishes, so it runs detached and the
# device list is what we poll.
# Requires the workload's I/O to have already been aborted -- see the StorageClass
# mapOptions in rook-smoke.sh; without that the freeze never completes.
unmap_stale() {
    require_docker
    # A wedged node stays Status=running and refuses to stop, so that flag cannot
    # tell "live" from "stuck". Entering it can: the stuck mapping pins the dead
    # container's namespaces, so exec fails where a healthy node answers.
    if docker inspect -f '{{.State.Running}}' "$NODE" 2>/dev/null | grep -qx true &&
       docker exec "$NODE" true >/dev/null 2>&1; then
        die "$NODE is live -- unmapping now would pull the device out from
      under a running workload. Use: just cluster-stop"
    fi
    local left
    left="$(printf '%s\n' "ls /sys/bus/rbd/devices/ 2>/dev/null | tr '\\n' ' '" | host_exec)"
    [ -n "$left" ] || { log "no stale rbd mappings"; return 0; }
    log "removing stale rbd mappings: $left"
    printf '%s\n' '
        for d in /sys/bus/rbd/devices/*/; do
            [ -e "$d" ] || continue
            (echo "${d%/}" | sed "s#.*/##" > /sys/bus/rbd/remove_single_major) &
        done
        i=0
        while [ -n "$(ls /sys/bus/rbd/devices/ 2>/dev/null)" ] && [ $i -lt 60 ]; do
            sleep 1; i=$((i+1))
        done
        echo "  remaining after ${i}s: [$(ls /sys/bus/rbd/devices/ 2>/dev/null | tr "\n" " ")]"
    ' | host_exec
    left="$(printf '%s\n' "ls /sys/bus/rbd/devices/ 2>/dev/null | tr '\\n' ' '" | host_exec)"
    [ -z "$left" ] || die "still mapped: $left
      Restart the Docker host to clear it (on macOS: colima restart)."
}

# A k3d restart clears the cordon on its own, so this covers a drain that was
# not followed by one: an aborted stop, a failed delete, or a manual drain.
uncordon() {
    command -v docker >/dev/null && docker info >/dev/null 2>&1 || return 0
    disks_present || return 0
    local ctx="k3d-$CLUSTER"
    # Only this path waits for the node: nothing can be uncordoned before the
    # API server answers, and a cluster that never had disks should not pay for
    # the wait at all.
    kubectl --context "$ctx" wait --for=condition=Ready node --all --timeout=300s >/dev/null 2>&1 || return 0
    kubectl --context "$ctx" get node "$NODE" >/dev/null 2>&1 || return 0
    kubectl --context "$ctx" uncordon "$NODE"
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
  has-module NAME
            exit 0 if the Docker host's kernel has (or can load) a module;
            'rbd' maps block devices, 'ceph' mounts CephFS
  attach    create + attach the disks (also the post-reboot fix)
  devices   short kernel names, one per line, for scripts
  status    what is attached, and what the node can see
  detach    detach, keep the backing files
  drain     evict Ceph consumers so the cluster can be stopped safely;
            does nothing unless a volume is mapped
  uncordon  undo the cordon that drain leaves
  unmap-stale
            drop rbd mappings left by a stop that bypassed the drain
  reset     wipe the disks and Rook's on-node state, reattach
  purge     detach and delete the disks for good, freeing the space

All commands are idempotent.

  --native  run host commands via sudo instead of a helper container
            (rootless Docker, where the container's PID 1 is not the host's)
  --allow-bare-metal
            proceed when the Docker host is not a VM. Ceph then runs against
            this machine's kernel: a stuck RBD mapping needs a reboot, and the
            privileged node sees your real disks.

Environment: CLUSTER=$CLUSTER DISK_COUNT=$DISK_COUNT DISK_SIZE=$DISK_SIZE
             DISK_VOLUME=$DISK_VOLUME NATIVE=$NATIVE
EOF
}

args=()
for a in "$@"; do
    case "$a" in
        --native) NATIVE=1 ;;
        --allow-bare-metal) ALLOW_BARE_METAL=1 ;;
        *) args+=("$a") ;;
    esac
done

case "${args[0]:-}" in
    doctor)  doctor ;;
    has-module) has_module "${args[1]:-}" ;;
    attach)  attach ;;
    devices) devices ;;
    status)  status ;;
    detach)  detach ;;
    drain)     drain ;;
    uncordon) uncordon ;;
    unmap-stale) unmap_stale ;;
    reset)   reset ;;
    purge)   purge ;;
    "")      usage ;;
    *)       printf 'unknown command: %s\n\n' "${args[0]}" >&2; usage >&2; exit 1 ;;
esac
