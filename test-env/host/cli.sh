#!/usr/bin/env bash
# Host side of the test environment: starts a VM at a provider and drives the
# environment inside it over SSH. Run through `just test-env <verb>`.
set -euo pipefail
. "$(dirname "$0")/lib.sh"

usage() {
  cat <<EOF
usage: just test-env <verb>
  up            fresh VM, environment up, tunnel to the console
  sync          copy the working tree into the VM
  redeploy      sync, then redeploy fundament; databases and shoots stay
  test [tags]   run the e2e scenarios in the VM; the bundle lands in test-env/bundles/
  run <sha>     fresh VM, full run of one commit; the VM stays when red
  collect       fetch a bundle of reports and logs from the VM
  ssh [cmd]     shell in the VM's checkout
  tunnel        forward the console to https://console.fundament.localhost:8443
  trust         trust the VM's CA on this host (only when it is not your own mkcert CA)
  status        provider state of the instance
  down          destroy the VM and its disks
  doctor        check the host requirements
Provider: TEST_ENV_PROVIDER (default qemu, macos on macOS). Instance: TEST_ENV_NAME (default "default").
EOF
}

load_provider() {
  local p=${TEST_ENV_PROVIDER:-$([ "$(uname -s)" = Darwin ] && echo macos || echo qemu)}
  [ -f "$TEST_ENV_DIR/host/providers/$p.sh" ] || die "unknown provider '$p' (see test-env/host/providers/)"
  # shellcheck disable=SC1090
  . "$TEST_ENV_DIR/host/providers/$p.sh"
  PROVIDER=$p
}

cmd_doctor() {
  local ok=true
  for c in ssh ssh-keygen git rsync; do
    command -v "$c" >/dev/null 2>&1 || { echo "missing: $c"; ok=false; }
  done
  # A provider sets PROVIDER_DOCKER=no when it can boot without Docker.
  if [ "${PROVIDER_DOCKER:-yes}" != no ]; then
    command -v docker >/dev/null 2>&1 || { echo "missing: docker"; ok=false; }
    docker info >/dev/null 2>&1 || { echo "docker daemon not reachable"; ok=false; }
  fi
  [ "$(type -t provider_doctor)" = function ] && { provider_doctor || ok=false; }
  $ok && echo "host ok for provider $PROVIDER"
  $ok
}

cmd_up() { # [sha]
  [ -f "$INSTANCE_DIR/provider" ] && die "instance '$TEST_ENV_NAME' exists; run: just test-env down (or set TEST_ENV_NAME)"
  cmd_doctor >/dev/null || { cmd_doctor; exit 1; }
  local start=$SECONDS
  # Private: the VM disk holds the CA key when PROVIDER_CA=host.
  mkdir -p "$INSTANCE_DIR"
  chmod 0700 "$INSTANCE_DIR"
  echo "$PROVIDER" > "$INSTANCE_DIR/provider"
  make_seed
  provider_create
  log "waiting for SSH"
  wait_ssh 100 || die "the VM does not answer SSH; see: just test-env status"
  vm_ssh 'systemctl is-system-running --wait >/dev/null || true'
  log "VM up after $((SECONDS - start))s"
  push_ca
  push_source "${1:-}"
  vm_just up || die "environment did not come up; inspect with: just test-env ssh (the VM stays)"
  start_tunnel
  log "environment up after $((SECONDS - start))s"
}

cmd_test() {
  require_instance
  local rc=0
  vm_just test "$(printf '%q' "${1:-}")" || rc=$?
  fetch_bundles
  return $rc
}

cmd_run() { # sha
  local sha=${1:?usage: just test-env run <sha>}
  sha=$(git -C "$REPO_DIR" rev-parse --verify "$sha^{commit}")
  [ -f "$INSTANCE_DIR/provider" ] && die "instance '$TEST_ENV_NAME' exists; run: just test-env down (or set TEST_ENV_NAME)"
  cmd_up "$sha"
  if cmd_test ""; then
    log "green: $sha"
    [ "${TEST_ENV_KEEP:-}" = 1 ] || cmd_down
  else
    log "red: $sha; the VM stays for inspection: just test-env ssh"
    exit 1
  fi
}

cmd_down() {
  [ -f "$INSTANCE_DIR/provider" ] || { log "no instance '$TEST_ENV_NAME'"; return 0; }
  require_instance
  stop_tunnel
  ssh -F "$(ssh_config)" -O exit vm 2>/dev/null || true
  provider_destroy
  rm -rf "$INSTANCE_DIR"
  log "instance '$TEST_ENV_NAME' destroyed"
}

cmd_trust() {
  require_instance
  mkdir -p "$INSTANCE_DIR/ca"
  scp -q -F "$(ssh_config)" vm:/var/lib/test-env/ca/rootCA.pem "$INSTANCE_DIR/ca/"
  CAROOT="$INSTANCE_DIR/ca" host_mkcert -install
}

verb=${1:-}
[ $# -gt 0 ] && shift
case "$verb" in
  up) load_provider; cmd_up ;;
  run) load_provider; cmd_run "$@" ;;
  doctor) load_provider; cmd_doctor ;;
  sync) require_instance; push_source; log "synced; redeploy with: just test-env redeploy" ;;
  redeploy) require_instance; push_source; vm_just redeploy ;;
  test) cmd_test "$@" ;;
  collect) require_instance; vm_just collect; fetch_bundles ;;
  ssh) require_instance; if [ $# -gt 0 ]; then vm_ssh -t "cd ~/$BOX_REPO && env -u SSH_CLIENT -u SSH2_CLIENT bash -lic $(printf '%q' "$*")"; else vm_ssh -t "cd ~/$BOX_REPO && exec bash -l"; fi ;;
  tunnel) require_instance; start_tunnel ;;
  trust) cmd_trust ;;
  status) require_instance; provider_status ;;
  down) cmd_down ;;
  ""|help|-h|--help) usage ;;
  *) usage; die "unknown verb '$verb' (in-VM recipes such as gardener-status run inside: just test-env ssh)" ;;
esac
