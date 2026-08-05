---
title: Block devices for k3d
sidebar:
  order: 11
---

A k3d node has no raw block devices, so a storage plugin that needs one cannot run locally. `deploy/k3d/storage-disks.sh` provides them as loop-backed partitions. Rook Ceph is the first consumer and shapes some of the requirements below, but the disks themselves are generic.

![How the loop-backed disks are layered on macOS and Linux](assets/k3d-block-devices.svg)

## Quick start

```bash
cd plugins
just storage-disks doctor      # check the kernel Docker runs on
just storage-disks attach      # creates /dev/loop0p1 .. /dev/loop2p1
```

Then verify the disks really work, independently of any plugin:

```bash
../deploy/k3d/rook-smoke.sh up     # upstream Rook chart + a minimal CephCluster
../deploy/k3d/rook-smoke.sh status # expect HEALTH_OK
../deploy/k3d/rook-smoke.sh test   # 1Gi PVC + a pod that writes to it -> SMOKE-OK
../deploy/k3d/rook-smoke.sh down   # frees the rook-ceph namespace
```

`rook-smoke.sh` uses no fundament code, so it separates a broken environment from a broken plugin.

## Requirements

The cluster must bind `/dev` and `/run/udev` into its node, which `plugins/sandbox/k3d-config.yaml` does. A cluster created before that change needs `just cluster-delete && just cluster-create`.

Defaults are 3 disks of 20 GiB, sparse, overridable with `DISK_COUNT` and `DISK_SIZE`. Three is the useful floor: it is what lets a pool hold 3 replicas across OSDs. BlueStore reserves several GiB of overhead, so do not go below 10 GiB per disk.

Ceph needs roughly 2 GiB per OSD plus its own daemons, so give the VM at least 8 GiB — on colima, `colima start --cpus 4 --memory 8`. Memory cannot be changed on a running VM.

## After a reboot

The backing files are ordinary files on the Docker data disk, so they survive a Docker daemon restart, a VM restart and a laptop reboot. The loop devices do not — those are kernel state. Recovery is `just storage-disks attach`, and nothing else: `/dev` is bound into the node as a live devtmpfs, so the devices reappear there immediately.

## Cleaning up

Nothing removes the images on its own — `k3d cluster delete` knows nothing about the volume, and `reset` deletes them only to recreate them empty. `just storage-disks purge` detaches and deletes the volume, which is the only way to get the space back. This matters more on Linux than on macOS, where deleting the VM takes the disks with it.

Deleting an image while its loop device is still attached frees nothing, because the kernel keeps the deleted inode alive until the device goes away. `purge` and `reset` therefore refuse while anything still holds a device; stop the consumer first.

Use `just storage-disks reset` to start over from clean disks, which is needed after deleting the cluster: the images keep their on-disk state while the node that referenced them is gone.

The volume is never attached to a container, so Docker counts it as unused: `docker volume prune -a` and `docker system prune --volumes` will delete the images. Plain `docker volume prune` will not, as it only removes anonymous volumes.

## Only ever select /dev/loopNpN

A k3d node container is privileged, and Docker populates its `/dev` with **every** device on the Docker host, so the node can open the host's real disks with or without the `/dev` bind. On macOS those are the VM's virtual disks; on a Linux desktop they are the machine's own drives. They also reach Rook's discover ConfigMap, so nothing at the Rook layer filters them out.

Whatever consumes these disks must therefore restrict itself to the loop partitions rather than trusting discovery. One consequence on Linux: any loop partition qualifies, including one backing an unrelated disk image you attached yourself with `losetup -P`.

## Docker backends

Loop devices and kernel modules live in the kernel the Docker daemon runs on: the VM on macOS, the machine itself on Linux. The helper reaches it the same way everywhere, with `docker run --privileged --pid=host … nsenter -t 1`, so no backend-specific shell is needed. Use `--native` for rootless Docker, where the container's PID 1 is not the host's.

What varies between backends is kernel module availability, which is what `doctor` reports.

| Backend | `rbd` / `ceph` modules |
|---|---|
| colima | verified present (Ubuntu 24.04 base `linux-modules`) |
| Rancher Desktop | Alpine `linux-virt` ships them, but the ISO is minimized — run `doctor` |
| Docker Desktop | its kernel is built by Docker — run `doctor`. Enhanced Container Isolation blocks `--privileged`, and then this does not work at all |
| OrbStack | custom kernel, undocumented — run `doctor` |
| Linux desktop | distro kernels ship `rbd` in the standard modules package |

A missing `rbd` module does not stop the disks working. Ceph OSDs write to the block device directly and never use it; only `csi-rbdplugin` crash-loops, so RBD PVCs fail to mount. It fatals at startup before reading a StorageClass, so `mounter: rbd-nbd` does not help. If you need working PVCs on such a backend, use colima.

If you run an unlisted backend, run `doctor` and report the result.

Ceph's reef (v18.x) arm64 images segfault on startup — `ceph-osd --version` exits 139 in a plain `docker run` — so anything running Ceph here needs v19 or newer.

## Why partitions and not bare loop devices

`rook-discover` does not report bare loop devices, so a `/dev/loopN` never reaches the ConfigMap Rook consumers read. Partitions are reported as `type=part`, which it accepts, so each image is attached with partition scanning and given a single GPT partition. This is about discoverability only: OSD provisioning happily accepts a bare loop device once the operator has `allowLoopDevices=true`, and Ceph runs fine on one.

The cause is an upstream defect rather than a design decision. Rook gates the device-type allowlist on an environment variable — `pkg/clusterd/disk.go:50` calls `getAllowLoopDevices()`, which reads `CEPH_VOLUME_ALLOW_LOOP_DEVICES` (`disk.go:256-258`) — and the operator injects that variable into the discover DaemonSet when `controller.LoopDevicesAllowed()` is true (`pkg/operator/discover/discover.go:236-238`). Here the operator ConfigMap contains `ROOK_CEPH_ALLOW_LOOP_DEVICES: true` while the DaemonSet has no such variable, so the allowance never reaches the daemon that needs it. Setting it by hand with `kubectl -n rook-ceph set env ds/rook-discover CEPH_VOLUME_ALLOW_LOOP_DEVICES=true` makes an unpartitioned device appear as `type=loop empty=True`.

Partitioning is kept anyway because it is self-contained and deterministic, where the alternative depends on patching a resource the operator owns, after it creates it. Two things were not chased further: whether the missing variable is deterministic or a startup race — a race would make bare loop devices work intermittently, which is worse than not at all — and which code path calls `SetAllowLoopDevices` (`pkg/operator/ceph/controller/controller_utils.go:110-118`). The code is identical in v1.16.0 and v1.20.3 (current release), so upgrading does not help, and no upstream issue reports it.
