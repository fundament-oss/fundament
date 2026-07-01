#!/usr/bin/env bash
# Hetzner Cloud one-time-use box — NO local nix required.
#
# Local deps: ssh, curl, and docker (you already run docker for fundament/k3d).
#   - hcloud CLI is fetched as a pinned static binary into cache/ (not brew/nix).
#   - NixOS is installed with nixos-anywhere run inside a throwaway `nixos/nix`
#     container, where root is a TRUSTED nix user by default. That is the missing
#     ingredient on a stock nix install (trusted-users=root only — common on macOS,
#     possible on Linux too), and it lets --build-on-remote work: the Hetzner box
#     builds the closure itself (native x86_64), the container just orchestrates over
#     SSH. Clean disko install, reusing the flake's hosts/hetzner config — no infect.
#
# Runs on macOS or Linux. Usage: ./hetzner.sh {up|down|ssh|status|stack|certs}
set -euo pipefail
cd "$(dirname "$0")"

# --- config (override via env) ---------------------------------------------
HZ_TYPE=${HZ_TYPE:-cx53}              # 16 vCPU / 32GB / 320GB. cx43 (16GB) OOMs the full stack
                                     # (gardener runs several apiservers + fundament). ccx43=64GB if needed.
HZ_IMAGE=${HZ_IMAGE:-ubuntu-24.04}   # base image nixos-anywhere kexecs away from (any works)
HZ_LOCATION=${HZ_LOCATION:-nbg1}     # EU (CX is EU-only)
HZ_NAME=${HZ_NAME:-fundament-test}
HZ_KEYNAME=${HZ_KEYNAME:-fundament-admin}
SSH_PORT=${SSH_PORT:-2022}           # NixOS sshd (baseline.nix); install phase is :22
ADMIN_PUBKEY=${ADMIN_PUBKEY:-$HOME/.ssh/id_rsa.pub}
HCLOUD_VERSION=${HCLOUD_VERSION:-1.51.0}
NIX_IMAGE=${NIX_IMAGE:-nixos/nix:latest}
ENVFILE=secrets/hetzner.env
CACHE=cache/bin

log() { printf '>> %s\n' "$*"; }
die() { printf '!! %s\n' "$*" >&2; exit 1; }

# Cattle boxes reuse IPs across recreations, so never record/verify host keys.
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=8)

# --- token -----------------------------------------------------------------
[ -f "$ENVFILE" ] || die "missing $ENVFILE (api_key=...); cp $ENVFILE.example $ENVFILE"
set -a; . "$ENVFILE"; set +a
export HCLOUD_TOKEN="${api_key:?api_key not set in $ENVFILE}"

# --- pinned static hcloud (no nix/brew) ------------------------------------
ensure_hcloud() {
  HC="$CACHE/hcloud"
  if [ ! -x "$HC" ]; then
    mkdir -p "$CACHE"
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m); case "$arch" in arm64|aarch64) arch=arm64;; x86_64|amd64) arch=amd64;; *) die "unsupported arch $arch";; esac
    url="https://github.com/hetznercloud/cli/releases/download/v${HCLOUD_VERSION}/hcloud-${os}-${arch}.tar.gz"
    log "fetching hcloud ${HCLOUD_VERSION} ($os/$arch)"
    curl -fsSL "$url" | tar -xz -C "$CACHE" hcloud || die "hcloud download failed: $url"
    chmod +x "$HC"
  fi
}
hc() { "$HC" "$@"; }

wait_ssh() { # host user port
  local i
  for i in $(seq 1 "${3:-60}"); do
    ssh -p "${4:-22}" "${SSH_OPTS[@]}" "${2}@${1}" true 2>/dev/null && return 0
    sleep 8
  done
  return 1
}

# Install NixOS onto an already-created box. Returns 0 only when the installed
# NixOS answers SSH on :2022 (the real success oracle — nixos-anywhere's exit code
# isn't reliable: it often drops SSH on the post-install reboot). Recovers the
# common "installed but not cleanly rebooted" case with one hard reset.
deploy_once() { # ip priv
  local ip=$1 priv=$2
  log "$HZ_NAME @ $ip — waiting for SSH on :22"
  wait_ssh "$ip" root 60 22 || { log "box never reachable on :22"; return 1; }

  log "installing NixOS via nixos-anywhere (throwaway nixos/nix container; build-on-remote)"
  docker run --rm \
    -v "$PWD:/work" -w /work \
    -v "$priv:/root/.ssh/id_rsa:ro" \
    -e NIX_CONFIG="experimental-features = nix-command flakes" \
    "$NIX_IMAGE" \
    nix run github:nix-community/nixos-anywhere -- \
      --flake /work#hetzner --build-on-remote \
      -i /root/.ssh/id_rsa \
      --ssh-option StrictHostKeyChecking=no --ssh-option UserKnownHostsFile=/dev/null \
      "root@$ip" \
    || log "nixos-anywhere exited nonzero (usually just the post-install reboot dropping SSH) — verifying over SSH"

  log "waiting for the installed NixOS on :$SSH_PORT"
  wait_ssh "$ip" thom 40 "$SSH_PORT" && return 0
  # nixos-anywhere sometimes dies before it cleanly reboots; power-cycle to boot the disk.
  log "no SSH on :$SSH_PORT yet — hard-resetting to boot from the installed disk"
  hc server reset "$HZ_NAME" >/dev/null 2>&1 || true
  wait_ssh "$ip" thom 45 "$SSH_PORT" && return 0
  return 1
}

cmd_up() {
  ensure_hcloud
  command -v docker >/dev/null 2>&1 || die "docker is required (used to run nixos-anywhere without local nix)"
  docker info >/dev/null 2>&1 || die "docker daemon is not running"
  [ -f "$ADMIN_PUBKEY" ] || die "admin pubkey not found: $ADMIN_PUBKEY (set ADMIN_PUBKEY=...)"
  local priv="${ADMIN_PUBKEY%.pub}"
  [ -f "$priv" ] || die "private key not found: $priv (matching $ADMIN_PUBKEY)"
  hc ssh-key describe "$HZ_KEYNAME" >/dev/null 2>&1 \
    || hc ssh-key create --name "$HZ_KEYNAME" --public-key-from-file "$ADMIN_PUBKEY"

  # Cattle: recover cheaply if we can, else start fresh (destroy + recreate). HZ_RETRIES total attempts.
  local tries=${HZ_RETRIES:-2} n=0 ip
  while :; do
    n=$((n + 1))
    if ! hc server describe "$HZ_NAME" >/dev/null 2>&1; then
      log "creating $HZ_NAME ($HZ_TYPE @ $HZ_LOCATION) — BILLING STARTS"
      hc server create --name "$HZ_NAME" --type "$HZ_TYPE" --image "$HZ_IMAGE" \
        --location "$HZ_LOCATION" --ssh-key "$HZ_KEYNAME" \
        || die "server create failed (on resource_unavailable try another HZ_LOCATION, e.g. hel1/fsn1)"
    else
      log "$HZ_NAME already exists; reusing"
    fi
    ip=$(hc server ip "$HZ_NAME")
    if deploy_once "$ip" "$priv"; then
      log "READY:  ssh -p $SSH_PORT thom@$ip   (or: ./hetzner.sh ssh)"
      log "BILLING IS RUNNING — tear down with: ./hetzner.sh down"
      return 0
    fi
    [ "$n" -ge "$tries" ] && die "deploy failed after $n attempt(s). Inspect with ./hetzner.sh ssh, or ./hetzner.sh down."
    log "attempt $n failed — starting fresh (destroying $HZ_NAME and recreating)"
    hc server delete "$HZ_NAME" >/dev/null 2>&1 || true
    sleep 5
  done
}

cmd_down()   { ensure_hcloud; hc server delete "$HZ_NAME" && log "deleted $HZ_NAME — billing stopped."; }
cmd_status() { ensure_hcloud; hc server list; }
cmd_ssh()    { ensure_hcloud; exec ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" "thom@$(hc server ip "$HZ_NAME")"; }

# Copy YOUR machine's mkcert CA onto the box (into mkcert's CAROOT) so the box's
# setup-certs signs *.fundament.localhost with a CA your OS/browser ALREADY trusts
# (from local `mkcert -install`). Then browser, functl and kubectl all trust the box
# — no cert install, no --insecure. Works the same on macOS and Linux: `mkcert -CAROOT`
# resolves the right per-OS path. Must land BEFORE setup-certs runs (stack does this
# automatically). Returns non-zero (no die) if there's no local mkcert CA to copy.
# Note: this copies the CA *private key* to a throwaway cloud box — fine for a test box.
copy_local_mkcert_ca() { # ip
  local ip=$1 caroot
  caroot=$(mkcert -CAROOT 2>/dev/null) || caroot=$( (cd .. && mise exec -- mkcert -CAROOT) 2>/dev/null) || caroot=""
  [ -n "$caroot" ] && [ -f "$caroot/rootCA.pem" ] && [ -f "$caroot/rootCA-key.pem" ] || return 1
  log "copying local mkcert CA ($caroot) -> box (its certs become trusted on your machine)"
  ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" "thom@$ip" 'mkdir -p ~/.local/share/mkcert' || return 1
  scp -P "$SSH_PORT" "${SSH_OPTS[@]}" "$caroot/rootCA.pem" "$caroot/rootCA-key.pem" "thom@$ip:.local/share/mkcert/" >/dev/null
}

# Standalone: copy the CA now. Run BEFORE `stack` (setup-certs picks it up); if the
# stack already ran with the box's own CA, re-run `stack` afterwards to re-issue.
cmd_certs() {
  ensure_hcloud
  local ip; ip=$(hc server ip "$HZ_NAME") || die "no $HZ_NAME server — run ./hetzner.sh up"
  copy_local_mkcert_ca "$ip" || die "no local mkcert CA found — run 'mkcert -install' first"
  log "done. Run BEFORE 'hetzner.sh stack' (or re-run stack to re-issue certs with it)."
}

# Push the on-box scripts + patches and run the full fundament + Gardener stack
# (clone -> mise -> cluster-create -> gardener-up -> skaffold -> shoot). Run after `up`.
cmd_stack() {
  ensure_hcloud
  local ip; ip=$(hc server ip "$HZ_NAME") || die "no $HZ_NAME server — run ./hetzner.sh up first"
  # Seed your machine's trusted mkcert CA first so setup-certs signs ingress certs with it.
  copy_local_mkcert_ca "$ip" || log "no local mkcert CA — box will self-sign (certs won't be trusted on your machine; see TODO.md)"
  log "staging box scripts + patches onto $HZ_NAME @ $ip"
  ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" "thom@$ip" 'mkdir -p ~/patches ~/box'
  scp -P "$SSH_PORT" "${SSH_OPTS[@]}" patches/*.patch "thom@$ip:patches/" >/dev/null
  scp -P "$SSH_PORT" "${SSH_OPTS[@]}" box/bootstrap.sh box/run-stack.sh "thom@$ip:box/" >/dev/null
  log "running bootstrap + stack on the box (gardener-up takes ~10-15 min)"
  exec ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" -o ServerAliveInterval=30 "thom@$ip" \
    'chmod +x ~/box/*.sh && ~/box/bootstrap.sh && ~/box/run-stack.sh'
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  ssh) cmd_ssh ;;
  status) cmd_status ;;
  stack) cmd_stack ;;
  certs) cmd_certs ;;
  *) die "usage: $0 {up|down|ssh|status|stack|certs}" ;;
esac
