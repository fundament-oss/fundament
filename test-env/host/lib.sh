# shellcheck shell=bash
# Shared host-side helpers for test-env/host/cli.sh and the provider drivers.
# Everything after "an SSH-reachable NixOS host with the test-env modules and a
# docker disk" goes over SSH and does not depend on the provider.

TEST_ENV_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
REPO_DIR=$(cd "$TEST_ENV_DIR/.." && pwd)

# Login user and sshd port of the NixOS image (nixos/modules/base.nix).
BOX_USER=fundament
BOX_SSH_PORT=2022
BOX_REPO=fundament

STATE_DIR=${XDG_CACHE_HOME:-$HOME/.cache}/fundament/test-env
TEST_ENV_NAME=${TEST_ENV_NAME:-default}
INSTANCE_DIR=$STATE_DIR/instances/$TEST_ENV_NAME
BUNDLE_DIR=${TEST_ENV_BUNDLE_DIR:-$TEST_ENV_DIR/bundles}

# Nix runs in this container, so the host needs no Nix.
NIX_IMAGE=${NIX_IMAGE:-nixos/nix:2.31.2}
# Docker seeds a named volume from the image only while it is empty, and the
# image's own binaries live in /nix, so each image version gets its own volume.
NIX_VOLUME=fundament-test-env-nix-${NIX_IMAGE##*:}

# 24 GiB is the smallest size the whole suite passed in, with two shoots; at 20
# GiB the second shoot runs the VM out of memory. vCPUs reserve nothing on the
# host, so the VM gets the host's threads, up to 16.
HOST_CPUS=$(getconf _NPROCESSORS_ONLN)
VM_CPUS=${TEST_ENV_CPUS:-$(( HOST_CPUS < 16 ? HOST_CPUS : 16 ))}
VM_MEM_GIB=${TEST_ENV_MEM_GIB:-24}
VM_ROOT_GB=${TEST_ENV_ROOT_GB:-80}
VM_DOCKER_GB=${TEST_ENV_DOCKER_GB:-200}

# The fundament ingress inside the VM; the tunnel forwards it to the same port on
# the host, because dex, the OIDC callbacks and CORS are bound to this origin.
CONSOLE_PORT=8443
CONSOLE_URL="https://console.fundament.localhost:${CONSOLE_PORT}"

log() { printf '>> %s\n' "$*"; }

# The host as release downloads name it: linux-amd64, darwin-arm64, ...
host_platform() {
  local arch
  case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) arch=$(uname -m) ;; esac
  echo "$(uname -s | tr '[:upper:]' '[:lower:]')-$arch"
}

sha256_of() { # file
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | cut -d' ' -f1
}

# Whether something listens on 127.0.0.1:<port>; bash's /dev/tcp works on Linux and macOS.
port_in_use() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
die() { printf '!! %s\n' "$*" >&2; exit 1; }

# First free TCP port from 2022 up, for the VM's sshd on 127.0.0.1.
free_port() {
  local p=2022
  while port_in_use "$p"; do p=$((p + 1)); done
  echo "$p"
}

# Run nix inside the container with the flake mounted read-only. The flake is
# copied first, because nix refuses a flake on a read-only path without git.
nix_run() {
  # shellcheck disable=SC2086 # NIX_DOCKER_ARGS holds several arguments
  docker run --rm -i \
    ${NIX_DOCKER_ARGS:-} \
    -v "$NIX_VOLUME:/nix" \
    -v "$TEST_ENV_DIR/nixos:/flake:ro" \
    -e NIX_CONFIG="$(printf 'experimental-features = nix-command flakes\nsystem-features = kvm nixos-test benchmark big-parallel\nmax-jobs = auto')" \
    "$NIX_IMAGE" sh -c 'cp -r /flake /src && cd /src && '"$1"
}

# Build a flake output for the host's architecture and print its store path. Nix
# skips the build when nothing in test-env/nixos changed. Without /dev/kvm (macOS)
# the image build's helper VM runs emulated, slower but the same.
nix_build() { # attr
  NIX_DOCKER_ARGS=$([ -e /dev/kvm ] && echo "--device /dev/kvm") nix_run "nix build --no-link --print-out-paths .#$1 2>/tmp/build.log || { tail -40 /tmp/build.log >&2; exit 1; }" \
    | tail -1
}

ssh_config() { printf '%s\n' "$INSTANCE_DIR/ssh_config"; }

# Write the ssh_config every verb uses; ControlMaster keeps repeated calls fast.
write_ssh_config() { # host port
  cat > "$INSTANCE_DIR/ssh_config" <<EOF
Host vm
  HostName $1
  Port $2
  User $BOX_USER
  IdentityFile $INSTANCE_DIR/id_ed25519
  IdentitiesOnly yes
  StrictHostKeyChecking accept-new
  UserKnownHostsFile $INSTANCE_DIR/known_hosts
  LogLevel ERROR
  ConnectTimeout 8
  ServerAliveInterval 30
  ControlMaster auto
  ControlPath $INSTANCE_DIR/cm
  ControlPersist 10m
EOF
}

vm_ssh() { ssh -F "$(ssh_config)" vm "$@"; }

# Run a recipe of the in-VM side in the VM's checkout. Without SSH_CLIENT,
# just's `bash -u` does not count as started by sshd, so it does not source
# /etc/bashrc, which fails under -u.
vm_just() { vm_ssh -t "cd ~/$BOX_REPO && env -u SSH_CLIENT -u SSH2_CLIENT just test-env $*"; }

# Long recipes (up, test) run in a tmux session in the VM when there is a
# terminal, so a dropped connection or a sleeping laptop does not end them:
# just test-env attach picks one up again. Without a terminal they run as any
# other recipe. Returns the recipe's exit code.
vm_just_session() { # session recipe [args]
  local session=$1
  shift
  [ -t 0 ] && [ -t 1 ] || { vm_just "$@"; return; }
  vm_ssh -t "cd ~/$BOX_REPO && $VM_TERM && tmux new-session -A -s $session test-env/box/session.sh $session $*"
  session_result "$session"
}

# tmux refuses a TERM the VM has no terminfo for (Ghostty, kitty).
VM_TERM='{ infocmp >/dev/null 2>&1 || export TERM=xterm-256color; }'

# After the tmux client returns: the recipe's exit code, or a stop when it
# still runs (a detach or a dropped connection).
session_result() { # session
  local logs=/var/lib/test-env/logs rc
  vm_ssh true 2>/dev/null || die "lost the connection to the VM; $1 goes on there: just test-env attach"
  if vm_ssh "tmux has-session -t $1" 2>/dev/null; then
    log "$1 goes on in the VM; pick it up again with: just test-env attach"
    exit 1
  fi
  # tmux restored the screen; the end of the output is in the log.
  vm_ssh "tail -n 15 $logs/$1.out" 2>/dev/null || true
  rc=$(vm_ssh "cat $logs/$1.rc" 2>/dev/null) || rc=1
  return "${rc:-1}"
}

wait_ssh() { # tries
  local i
  for i in $(seq 1 "${1:-100}"); do
    vm_ssh true 2>/dev/null && return 0
    sleep 3
  done
  return 1
}

require_instance() {
  [ -f "$INSTANCE_DIR/provider" ] || die "no test-env instance '$TEST_ENV_NAME'; start one with: just test-env up"
  # shellcheck disable=SC1090
  . "$TEST_ENV_DIR/host/providers/$(cat "$INSTANCE_DIR/provider").sh"
}

# The instance key (passphrase-less, so automation works without an agent) and
# the operator's own key, for manual logins.
make_seed() {
  mkdir -p "$INSTANCE_DIR/seed"
  ssh-keygen -q -t ed25519 -N '' -C "test-env-$TEST_ENV_NAME" -f "$INSTANCE_DIR/id_ed25519"
  {
    echo '#cloud-config'
    echo 'ssh_authorized_keys:'
    echo "  - $(cat "$INSTANCE_DIR/id_ed25519.pub")"
    for k in "$HOME/.ssh/id_ed25519.pub" "$HOME/.ssh/id_rsa.pub"; do
      [ -f "$k" ] && echo "  - $(cat "$k")"
    done
  } > "$INSTANCE_DIR/seed/user-data"
  printf 'instance-id: %s-%s\nlocal-hostname: fundament-test\n' "$TEST_ENV_NAME" "$(date +%s)" > "$INSTANCE_DIR/seed/meta-data"
}

host_mkcert() {
  if command -v mkcert >/dev/null 2>&1; then mkcert "$@"
  else (cd "$REPO_DIR" && mise exec -- mkcert "$@"); fi
}

# PROVIDER_CA=host copies the CA the host browser already trusts; otherwise the VM
# generates its own and the operator decides whether to trust it.
push_ca() {
  vm_ssh 'sudo install -d -o fundament -m 0700 /var/lib/test-env/ca'
  [ "${PROVIDER_CA:-fresh}" = host ] || return 0
  local caroot
  caroot=$(host_mkcert -CAROOT 2>/dev/null) || caroot=""
  if [ -n "$caroot" ] && [ -f "$caroot/rootCA.pem" ] && [ -f "$caroot/rootCA-key.pem" ]; then
    scp -q -F "$(ssh_config)" "$caroot/rootCA.pem" "$caroot/rootCA-key.pem" vm:/var/lib/test-env/ca/
    log "the VM signs with your mkcert CA ($caroot)"
  else
    log "no mkcert CA on this host; the VM generates one (trust it with: just test-env trust)"
  fi
}

# Push the commit with git (real shas for skaffold's gitCommit tagger, unpushed
# commits work), then lay uncommitted changes on top unless a sha was asked for.
push_source() { # [sha]
  local rev=${1:-HEAD}
  vm_ssh "[ -d ~/$BOX_REPO/.git ] || { git init -q ~/$BOX_REPO && git -C ~/$BOX_REPO config receive.denyCurrentBranch ignore; }"
  GIT_SSH_COMMAND="ssh -F $(ssh_config)" git -C "$REPO_DIR" push -q --force "vm:$BOX_REPO" "$rev:refs/test-env/head"
  vm_ssh "cd ~/$BOX_REPO && git checkout -q -f --detach refs/test-env/head && git clean -q -fd"
  [ -n "${1:-}" ] && return 0
  # --no-renames: a staged rename is a delete of the old path plus an add.
  (cd "$REPO_DIR" && { git diff --no-renames --name-only -z HEAD --diff-filter=d; git ls-files -oz --exclude-standard; }) \
    | rsync -a --from0 --files-from=- -e "ssh -F $(ssh_config)" "$REPO_DIR/" "vm:$BOX_REPO/"
  local deleted
  deleted=$(cd "$REPO_DIR" && git diff --no-renames --name-only HEAD --diff-filter=D)
  [ -z "$deleted" ] || printf '%s\n' "$deleted" | vm_ssh "cd ~/$BOX_REPO && xargs -r -d '\n' rm -f --"
}

# Copy bundles the VM wrote and has not handed over yet.
fetch_bundles() {
  mkdir -p "$BUNDLE_DIR"
  rsync -a --remove-source-files -e "ssh -F $(ssh_config)" "vm:/var/lib/test-env/bundles/" "$BUNDLE_DIR/" \
    || log "could not fetch bundles from the VM; they stay in /var/lib/test-env/bundles there"
  local b
  for b in "$BUNDLE_DIR"/*.tar.gz; do
    [ -e "$b" ] || continue
    local d=${b%.tar.gz}
    tar -xzf "$b" -C "$BUNDLE_DIR" && rm -f "$b"
    [ -n "$(type -t provider_logs)" ] && provider_logs "$d"
    log "bundle: $d"
  done
}

tunnel_running() { [ -f "$INSTANCE_DIR/tunnel.pid" ] && kill -0 "$(cat "$INSTANCE_DIR/tunnel.pid")" 2>/dev/null; }

start_tunnel() {
  tunnel_running && return 0
  if port_in_use "$CONSOLE_PORT"; then
    log "port $CONSOLE_PORT is taken on this host (a local k3d fundament?); no tunnel. Free it and run: just test-env tunnel"
    return 0
  fi
  ssh -F "$(ssh_config)" -o ControlMaster=no -o ControlPath=none -o ExitOnForwardFailure=yes \
    -f -N -L "127.0.0.1:${CONSOLE_PORT}:127.0.0.1:${CONSOLE_PORT}" vm
  pgrep -n -f "ssh -F $(ssh_config) .*-L 127.0.0.1:${CONSOLE_PORT}" > "$INSTANCE_DIR/tunnel.pid"
  log "console: $CONSOLE_URL (alice@acme-corp.com / password)"
}

stop_tunnel() {
  tunnel_running && kill "$(cat "$INSTANCE_DIR/tunnel.pid")" 2>/dev/null
  rm -f "$INSTANCE_DIR/tunnel.pid"
}
