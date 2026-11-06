# Boots one VM instance. run-vm.nix wraps this with the flake's qemu, which the qemu
# provider runs inside the nixos/nix container. It needs only qemu and xorriso, so
# it also runs with a host's own qemu, under Apple's hypervisor on macOS.
#
# BASE_IMAGE is the qcow2 the root disk is layered on (read-only). The instance
# directory (/instance in the container) holds seed/ with user-data and meta-data
# for the NoCloud seed, and receives root.qcow2, docker.raw, efi-vars.fd, seed.iso,
# serial.log, serial.sock and qmp.sock. CACHE_DISK, when set, is a disk image that
# outlives the instance. Arguments go to qemu.
set -euo pipefail

: "${BASE_IMAGE:?BASE_IMAGE must name the image to boot}"
dir=${INSTANCE_DIR:-/instance}
cpus=${VM_CPUS:-16}
mem=${VM_MEM_GIB:-40}
root_gb=${VM_ROOT_GB:-80}
docker_gb=${VM_DOCKER_GB:-200}
ssh_port=${VM_SSH_PORT:-2022}

# VM_ARCH boots another architecture, emulated: slow, for checking an image only,
# and with a qemu that has that target (the flake's run-vm has only the host's).
host=$(uname -m)
arch=${VM_ARCH:-$host}
[ "$host" = arm64 ] && host=aarch64
[ "$arch" = arm64 ] && arch=aarch64
cpu=host
if [ "$arch" != "$host" ]; then accel=tcg cpu=max
elif [ "$(uname -s)" = Darwin ]; then accel=hvf
elif [ -e /dev/kvm ]; then accel=kvm
else accel=tcg cpu=max
fi
case "$arch" in
  x86_64)
    qemu=qemu-system-x86_64 machine="q35,accel=$accel"
    code=edk2-x86_64-code.fd vars=edk2-i386-vars.fd ;;
  aarch64)
    qemu=qemu-system-aarch64 machine="virt,accel=$accel"
    [ "$accel" = kvm ] && machine="$machine,gic-version=max"
    code=edk2-aarch64-code.fd vars=edk2-arm-vars.fd ;;
  *) echo "unsupported architecture $arch" >&2; exit 1 ;;
esac
# The UEFI firmware qemu ships next to its binary.
firmware=${QEMU_FIRMWARE_DIR:-$(dirname "$(command -v "$qemu")")/../share/qemu}

cd "$dir"
[ -f root.qcow2 ] || qemu-img create -q -f qcow2 -F qcow2 -b "$BASE_IMAGE" root.qcow2 "${root_gb}G"
[ -f docker.raw ] || qemu-img create -q -f raw docker.raw "${docker_gb}G"
[ -f efi-vars.fd ] || { cp "$firmware/$vars" efi-vars.fd; chmod u+w efi-vars.fd; }
cache=()
if [ -n "${CACHE_DISK:-}" ]; then
  [ -f "$CACHE_DISK" ] || qemu-img create -q -f raw "$CACHE_DISK" "${VM_CACHE_GB:-100}G"
  cache=(-drive "if=none,id=ch,file=$CACHE_DISK,format=raw,discard=unmap" -device "virtio-blk-pci,drive=ch,serial=cache")
fi
xorriso -as genisoimage -quiet -output seed.iso -volid cidata -joliet -rock seed/user-data seed/meta-data

exec "$qemu" \
  -machine "$machine" -cpu "$cpu" -smp "$cpus" -m "${mem}G" \
  -drive "if=pflash,format=raw,readonly=on,file=$firmware/$code" \
  -drive if=pflash,format=raw,file=efi-vars.fd \
  -drive if=none,id=root,file=root.qcow2,format=qcow2,discard=unmap \
  -device virtio-blk-pci,drive=root,bootindex=0 \
  -drive if=none,id=dk,file=docker.raw,format=raw,discard=unmap,cache=unsafe \
  -device virtio-blk-pci,drive=dk,serial=docker \
  ${cache[@]+"${cache[@]}"} \
  -drive if=none,id=seed,file=seed.iso,format=raw,readonly=on \
  -device virtio-blk-pci,drive=seed \
  -netdev user,id=n0,hostfwd=tcp:127.0.0.1:"$ssh_port"-:2022 -device virtio-net-pci,netdev=n0 \
  -device virtio-balloon-pci,free-page-reporting=on \
  -device virtio-rng-pci \
  -chardev socket,id=s0,path=serial.sock,server=on,wait=off,logfile=serial.log \
  -serial chardev:s0 \
  -qmp unix:qmp.sock,server,nowait \
  -display none \
  "$@"
