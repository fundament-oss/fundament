# shellcheck shell=bash
# Provider: a local VM on Apple silicon. Docker builds the aarch64 image in an arm64
# nixos/nix container; Homebrew's qemu boots it under Apple's hypervisor.
# Sourced by host/cli.sh after host/lib.sh.

PROVIDER_CA=host
QEMU_PID=$INSTANCE_DIR/qemu.pid

# The image is copied out of the nix volume under a hash of nixos/, so a cached
# image boots without Docker running.
MAC_IMAGE=$STATE_DIR/images/$(cd "$TEST_ENV_DIR/nixos" && find . -type f -print0 | LC_ALL=C sort -z \
  | xargs -0 shasum -a 256 | shasum -a 256 | cut -c1-16).qcow2
[ -f "$MAC_IMAGE" ] && PROVIDER_DOCKER=no

provider_doctor() {
  [ "$(uname -s)-$(uname -m)" = Darwin-arm64 ] || { echo "the macos provider needs macOS on Apple silicon"; return 1; }
  local ok=0 c total docker_gib
  for c in qemu-system-aarch64 qemu-img xorriso; do
    command -v "$c" >/dev/null 2>&1 || { echo "missing: $c (brew install qemu xorriso)"; ok=1; }
  done
  # Docker's VM holds its memory while it runs, next to this VM.
  total=$(( $(sysctl -n hw.memsize) >> 30 ))
  docker_gib=$(docker info --format '{{.MemTotal}}' 2>/dev/null) || docker_gib=0
  docker_gib=$(( docker_gib >> 30 ))
  [ $(( total - docker_gib - VM_MEM_GIB )) -ge 6 ] \
    || echo "warning: ${total} GiB memory, Docker holds ${docker_gib} GiB, the VM takes ${VM_MEM_GIB} GiB (TEST_ENV_MEM_GIB); stop or shrink Docker's VM once the image is built" >&2
  return $ok
}

provider_create() {
  if [ ! -f "$MAC_IMAGE" ]; then
    log "building the NixOS image (cached after the first build)"
    local store
    store=$(nix_build vm-image) || die "image build failed"
    mkdir -p "$STATE_DIR/images"
    docker run --rm -v "$NIX_VOLUME:/nix:ro" -v "$STATE_DIR/images:/out" "$NIX_IMAGE" \
      sh -c "cp $store/*.qcow2 /out/partial.qcow2" || die "copying the image out of the nix volume failed"
    mv "$STATE_DIR/images/partial.qcow2" "$MAC_IMAGE"
    # An older image stays only while an instance's root disk is layered on it.
    local old
    for old in "$STATE_DIR"/images/*.qcow2; do
      [ "$old" = "$MAC_IMAGE" ] || grep -qsxF "$old" "$STATE_DIR"/instances/*/base-image || rm -f "$old"
    done
  fi
  echo "$MAC_IMAGE" > "$INSTANCE_DIR/base-image"
  local port cache=
  port=$(free_port)
  # The cache disk outlives the instance: one per instance name, kept by `down`.
  if [ "${TEST_ENV_CACHE:-1}" != 0 ]; then
    mkdir -p "$STATE_DIR/cache"
    cache=$STATE_DIR/cache/$TEST_ENV_NAME.raw
  fi
  write_ssh_config 127.0.0.1 "$port"
  log "booting the VM ($VM_CPUS vCPU, $VM_MEM_GIB GiB, ssh on 127.0.0.1:$port)"
  # -daemonize detaches qemu from the terminal, so Ctrl-C on a verb leaves the VM running.
  INSTANCE_DIR=$INSTANCE_DIR BASE_IMAGE=$MAC_IMAGE CACHE_DISK=$cache \
    VM_CPUS=$VM_CPUS VM_MEM_GIB=$VM_MEM_GIB VM_ROOT_GB=$VM_ROOT_GB VM_DOCKER_GB=$VM_DOCKER_GB VM_SSH_PORT=$port \
    bash "$TEST_ENV_DIR/nixos/run-vm.sh" -daemonize -pidfile "$QEMU_PID" >"$INSTANCE_DIR/qemu.log" 2>&1 \
    || die "qemu did not start: $(cat "$INSTANCE_DIR/qemu.log")"
}

qemu_running() { [ -f "$QEMU_PID" ] && kill -0 "$(cat "$QEMU_PID")" 2>/dev/null; }

provider_status() {
  if qemu_running; then echo "qemu pid $(cat "$QEMU_PID"): running"; else echo "qemu: not running"; fi
  echo "serial console log: $INSTANCE_DIR/serial.log"
}

provider_logs() { # dir
  cp "$INSTANCE_DIR/serial.log" "$1/serial.log" 2>/dev/null || true
}

provider_destroy() {
  qemu_running || return 0
  local pid
  pid=$(cat "$QEMU_PID")
  # Shut the guest down first, so it flushes the cache disk the next VM reuses.
  vm_ssh 'sudo systemctl poweroff' >/dev/null 2>&1 || true
  for _ in $(seq 1 60); do kill -0 "$pid" 2>/dev/null || return 0; sleep 1; done
  kill "$pid" 2>/dev/null || true
}
