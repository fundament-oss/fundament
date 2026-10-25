# shellcheck shell=bash
# Provider: a local VM. qemu, pinned by the flake, runs inside the nixos/nix
# container with /dev/kvm passed through, so the host needs only docker and KVM.
# Sourced by host/cli.sh after host/lib.sh.

PROVIDER_CA=host
QEMU_CONTAINER=fundament-test-env-$TEST_ENV_NAME

provider_doctor() {
  [ "$(uname -s)" = Linux ] || { echo "the qemu provider needs Linux with KVM"; return 1; }
  docker info --format '{{.OperatingSystem}}' 2>/dev/null | grep -q 'Docker Desktop' \
    && { echo "Docker Desktop runs containers in its own VM without /dev/kvm; use Docker Engine"; return 1; }
  [ -e /dev/kvm ] || { echo "no /dev/kvm: enable virtualisation (VT-x/AMD-V) in the BIOS and load kvm_intel or kvm_amd"; return 1; }
  local avail
  avail=$(awk '/^MemAvailable:/{print int($2/1048576)}' /proc/meminfo)
  [ "$avail" -ge "$VM_MEM_GIB" ] || echo "warning: ${avail} GiB available, the VM may use up to ${VM_MEM_GIB} GiB (TEST_ENV_MEM_GIB)" >&2
}

provider_create() {
  log "building the NixOS image (cached after the first build)"
  local image run_vm port
  image=$(nix_build vm-image) || die "image build failed"
  image=$(docker run --rm -v "$NIX_VOLUME:/nix:ro" "$NIX_IMAGE" sh -c "ls $image/*.qcow2")
  run_vm=$(nix_build run-vm) || die "run-vm build failed"
  port=$(free_port)
  # The cache disk outlives the instance: one per instance name, kept by `down`.
  local cache_args=()
  if [ "${TEST_ENV_CACHE:-1}" != 0 ]; then
    mkdir -p "$STATE_DIR/cache"
    cache_args=(-v "$STATE_DIR/cache:/cache" -e CACHE_DISK="/cache/$TEST_ENV_NAME.raw")
  fi
  write_ssh_config 127.0.0.1 "$port"
  log "booting the VM ($VM_CPUS vCPU, $VM_MEM_GIB GiB, ssh on 127.0.0.1:$port)"
  # Host networking: slirp keeps the client's source address, which collides with
  # the guest's own docker networks when it comes through a docker port proxy.
  docker run -d --name "$QEMU_CONTAINER" --network host \
    --device /dev/kvm --group-add "$(stat -c %g /dev/kvm)" --user "$(id -u):$(id -g)" \
    -v "$NIX_VOLUME:/nix:ro" -v "$INSTANCE_DIR:/instance" \
    ${cache_args[@]+"${cache_args[@]}"} \
    -e BASE_IMAGE="$image" \
    -e VM_CPUS="$VM_CPUS" -e VM_MEM_GIB="$VM_MEM_GIB" -e VM_ROOT_GB="$VM_ROOT_GB" \
    -e VM_DOCKER_GB="$VM_DOCKER_GB" -e VM_SSH_PORT="$port" \
    "$NIX_IMAGE" "$run_vm/bin/run-vm" >/dev/null
  sleep 3
  [ "$(docker inspect -f '{{.State.Running}}' "$QEMU_CONTAINER" 2>/dev/null)" = true ] \
    || die "qemu exited: $(docker logs "$QEMU_CONTAINER" 2>&1 | tail -20)"
}

provider_status() {
  docker ps -a --filter "name=^${QEMU_CONTAINER}$" --format 'qemu container {{.Names}}: {{.Status}}'
  echo "serial console log: $INSTANCE_DIR/serial.log"
}

provider_logs() { # dir
  cp "$INSTANCE_DIR/serial.log" "$1/serial.log" 2>/dev/null || true
}

provider_destroy() {
  # Shut the guest down first, so it flushes the cache disk the next VM reuses.
  if [ -n "$(docker ps -q --filter "name=^${QEMU_CONTAINER}$")" ]; then
    vm_ssh 'sudo systemctl poweroff' >/dev/null 2>&1 || true
    timeout 60 docker wait "$QEMU_CONTAINER" >/dev/null 2>&1 || true
  fi
  docker rm -f "$QEMU_CONTAINER" >/dev/null 2>&1 || true
}
