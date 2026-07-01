#!/usr/bin/env bash
# Hetzner Cloud one-time-use box — NO local nix required.
#
# Local deps: ssh, curl, docker (you already run docker for fundament/k3d), and
# mkcert (you already use it for local fundament dev).
#   - hcloud CLI is fetched as a pinned static binary into cache/ (not brew/nix).
#   - NixOS is installed with nixos-anywhere run inside a throwaway `nixos/nix`
#     container, where root is a TRUSTED nix user by default. That is the missing
#     ingredient on a stock nix install (trusted-users=root only), and it lets
#     --build-on-remote work: the box builds the closure itself (native x86_64), the
#     container just orchestrates over SSH. Clean disko install — no infect.
#
# Runs on macOS or Linux. Usage: ./hetzner.sh {up|down|ssh|status|stack|certs|tunnel}
set -euo pipefail
cd "$(dirname "$0")"

# --- config (override via env) ---------------------------------------------
HZ_TYPE=${HZ_TYPE:-cx53}              # 16 vCPU / 32GB / 320GB. cx43 (16GB) OOMs the full stack
                                     # (gardener runs several apiservers + fundament). ccx43=64GB if needed.
HZ_IMAGE=${HZ_IMAGE:-ubuntu-24.04}   # base image nixos-anywhere kexecs away from (any works)
HZ_LOCATION=${HZ_LOCATION:-nbg1}     # EU (CX is EU-only)
HZ_NAME=${HZ_NAME:-fundament-test}
HZ_KEYNAME=${HZ_KEYNAME:-fundament-admin}
BOX_USER=${BOX_USER:-thom}           # login user on the box (must match modules/baseline.nix)
SSH_PORT=${SSH_PORT:-2022}           # NixOS sshd (baseline.nix); install phase is :22
HCLOUD_VERSION=${HCLOUD_VERSION:-1.51.0}
NIX_IMAGE=${NIX_IMAGE:-nixos/nix:latest}
ENVFILE=secrets/hetzner.env
CACHE=cache/bin
BOX_CA=cache/box-ca                  # the box's ephemeral CA, fetched here + trusted while the box lives

# print a progress line to stdout
log() { printf '>> %s\n' "$*"; }
# print an error to stderr and abort
die() { printf '!! %s\n' "$*" >&2; exit 1; }

# Admin pubkey registered with hcloud for the install phase; prefer ed25519, else rsa.
if [ -z "${ADMIN_PUBKEY:-}" ]; then
  for k in "$HOME/.ssh/id_ed25519.pub" "$HOME/.ssh/id_rsa.pub"; do
    [ -f "$k" ] && { ADMIN_PUBKEY="$k"; break; }
  done
  ADMIN_PUBKEY=${ADMIN_PUBKEY:-$HOME/.ssh/id_ed25519.pub}   # for the not-found message
fi

# Cattle boxes reuse IPs across recreations, so never record/verify host keys.
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=8)

# --- token -----------------------------------------------------------------
[ -f "$ENVFILE" ] || die "missing $ENVFILE (api_key=...); cp $ENVFILE.example $ENVFILE"
set -a; . "$ENVFILE"; set +a
export HCLOUD_TOKEN="${api_key:?api_key not set in $ENVFILE}"

# --- helpers ---------------------------------------------------------------
# pinned static hcloud (no nix/brew)
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
# run the fetched hcloud binary (token comes from the exported HCLOUD_TOKEN)
hc() { "$HC" "$@"; }

# public IPv4 of the box, or die (hcloud prints empty + exits 0 for a server with no IPv4).
box_ip() {
  local ip; ip=$(hc server ip "$HZ_NAME" 2>/dev/null) || die "no $HZ_NAME server — run ./hetzner.sh up"
  [ -n "$ip" ] || die "$HZ_NAME has no public IPv4"
  printf '%s\n' "$ip"
}

# the operator's mkcert (global, else via the repo's mise); MKCERT is a bash array.
resolve_mkcert() {
  if command -v mkcert >/dev/null 2>&1; then MKCERT=(mkcert)
  elif mise exec -- mkcert --version >/dev/null 2>&1; then MKCERT=(mise exec -- mkcert)
  else die "mkcert not found (needed to trust the box CA) — install it, or run from the fundament repo"; fi
}

# poll SSH on host:port as the given user until it answers (tries x 8s); 0 if reachable
wait_ssh() { # host user tries port
  local i
  for i in $(seq 1 "${3:-60}"); do
    ssh -p "${4:-22}" "${SSH_OPTS[@]}" "${2}@${1}" true 2>/dev/null && return 0
    sleep 8
  done
  return 1
}

# --- cert trust (ephemeral per-box CA) -------------------------------------
# The box generates its OWN mkcert CA per deploy (setup-certs runs mkcert on the box).
# We fetch that CA here and `mkcert -install` it into the local trust stores (system +
# browser NSS, cross-platform) so browser/functl/kubectl trust the box with no --insecure.
# YOUR real mkcert CA never touches the box. `down` runs -uninstall + deletes the copy.
trust_box_ca() { # ip
  local ip=$1 boxroot
  resolve_mkcert
  boxroot=$(ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" "$BOX_USER@$ip" \
    'export PATH=$HOME/.nix-profile/bin:$PATH; cd ~/fundament 2>/dev/null && mise exec -- mkcert -CAROOT 2>/dev/null' </dev/null) \
    || die "couldn't reach the box to read its mkcert CA"
  [ -n "$boxroot" ] || die "box has no mkcert CA yet (run ./hetzner.sh stack first)"
  mkdir -p "$BOX_CA"
  scp -P "$SSH_PORT" "${SSH_OPTS[@]}" \
    "$BOX_USER@$ip:$boxroot/rootCA.pem" "$BOX_USER@$ip:$boxroot/rootCA-key.pem" "$BOX_CA/" \
    || die "failed to fetch the box CA from $boxroot"
  log "trusting the box's ephemeral CA locally (mkcert may prompt for sudo)"
  CAROOT="$PWD/$BOX_CA" "${MKCERT[@]}" -install
}

# remove the box's ephemeral CA from local trust and delete the local copy (run on down)
untrust_box_ca() {
  [ -f "$BOX_CA/rootCA.pem" ] || return 0
  resolve_mkcert
  log "removing the box CA from local trust (mkcert -uninstall)"
  CAROOT="$PWD/$BOX_CA" "${MKCERT[@]}" -uninstall || true
  rm -rf "$BOX_CA"
}

# tell the operator how to reach the box UIs from their browser (tunnel + URL)
print_access() {
  log ""
  log "reach the box UIs from your normal browser:"
  log "  1) run:   just hetzner-tunnel      # SSH tunnel :8443 (stop a local k3d first if it owns 8443)"
  log "  2) open:  $CONSOLE_URL"
  log "     (also docs./dcim./dex.fundament.localhost:8443 — the box CA is trusted locally, no cert warning)"
}

# --- install NixOS onto an already-created box -----------------------------
# Returns 0 only when the installed NixOS answers SSH on :$SSH_PORT (nixos-anywhere's
# exit code isn't reliable — it often drops SSH on the post-install reboot). Recovers
# the "installed but not cleanly rebooted" case with one hard reset.
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
  wait_ssh "$ip" "$BOX_USER" 40 "$SSH_PORT" && return 0
  log "no SSH on :$SSH_PORT yet — hard-resetting to boot from the installed disk"
  hc server reset "$HZ_NAME" >/dev/null 2>&1 || true
  wait_ssh "$ip" "$BOX_USER" 45 "$SSH_PORT" && return 0
  return 1
}

# create the box (if absent) and install NixOS onto it; on failure, destroy + recreate (HZ_RETRIES)
cmd_up() {
  ensure_hcloud
  command -v docker >/dev/null 2>&1 || die "docker is required (used to run nixos-anywhere without local nix)"
  docker info >/dev/null 2>&1 || die "docker daemon is not running"
  [ -f "$ADMIN_PUBKEY" ] || die "admin pubkey not found: $ADMIN_PUBKEY (set ADMIN_PUBKEY=..., or create an ssh key)"
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
    ip=$(box_ip)
    if deploy_once "$ip" "$priv"; then
      log "READY:  ssh -p $SSH_PORT $BOX_USER@$ip   (or: ./hetzner.sh ssh)"
      log "next:   just hetzner-stack   # deploy fundament + Gardener and run a shoot"
      log "BILLING IS RUNNING — tear down with: ./hetzner.sh down"
      return 0
    fi
    [ "$n" -ge "$tries" ] && die "deploy failed after $n attempt(s). Inspect with ./hetzner.sh ssh, or ./hetzner.sh down."
    log "attempt $n failed — starting fresh (destroying $HZ_NAME and recreating)"
    hc server delete "$HZ_NAME" >/dev/null 2>&1 || true
    sleep 5
  done
}

# untrust the box's CA locally, then destroy the box (stops billing)
cmd_down() {
  ensure_hcloud
  untrust_box_ca
  hc server delete "$HZ_NAME" && log "deleted $HZ_NAME — billing stopped."
}
# list the project's servers (so a billing box is never forgotten)
cmd_status() { ensure_hcloud; hc server list; }
# open an interactive shell on the box (NixOS, port 2022)
cmd_ssh()    { ensure_hcloud; exec ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" "$BOX_USER@$(box_ip)"; }

# --- reach the box ingress -------------------------------------------------
# The box's fundament is hardwired to the https://*.fundament.localhost:8443 origin
# (dex issuer, OIDC callbacks, API CORS allowlists), so the UIs only work when reached
# at EXACTLY that origin — forward local 8443, which a LOCAL k3d fundament usually owns
# (the guard catches that). One forwarded port serves every *.fundament.localhost host
# (host-routed nginx). *.localhost -> 127.0.0.1.
LCONSOLE_PORT=8443
CONSOLE_URL="https://console.fundament.localhost:${LCONSOLE_PORT}"

# refuse the tunnel if local :8443 is held by something other than our own ssh tunnel
require_console_port() {
  local who
  # lsof exits non-zero when nothing is listening (port free) — don't let that trip
  # `set -e`/pipefail; an empty `who` means free.
  who=$( { lsof -nP -iTCP:"${LCONSOLE_PORT}" -sTCP:LISTEN -F c 2>/dev/null || true; } | sed -n 's/^c//p' | head -1)
  [ -z "$who" ] && return 0          # free
  [ "$who" = "ssh" ] && return 0     # our own tunnel — reuse
  die "local :${LCONSOLE_PORT} is held by '$who' (your LOCAL k3d fundament?). The box's app only
    works at the :${LCONSOLE_PORT} origin, so free it first:  k3d cluster stop fundament"
}

# Foreground SSH tunnel: localhost:8443 -> box ingress (all UIs). Ctrl-C to close.
cmd_tunnel() {
  ensure_hcloud
  require_console_port
  local ip; ip=$(box_ip)
  log "tunnel open: $CONSOLE_URL  (also docs./dcim./dex.fundament.localhost:${LCONSOLE_PORT})"
  log "open the URL in your browser; Ctrl-C to close the tunnel."
  exec ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" -N -L "${LCONSOLE_PORT}:127.0.0.1:8443" "$BOX_USER@$ip"
}

# Re-trust the box CA locally (stack does this automatically; use if you switched machines).
cmd_certs() { ensure_hcloud; trust_box_ca "$(box_ip)"; log "box CA trusted locally."; }

# --- run the full stack on the box -----------------------------------------
# Push the on-box scripts + patches, run bootstrap + the stack, then trust the box's
# freshly-generated CA locally and print how to reach the UIs.
cmd_stack() {
  ensure_hcloud
  local ip; ip=$(box_ip)
  log "staging box scripts + patches onto $HZ_NAME @ $ip"
  ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" "$BOX_USER@$ip" 'mkdir -p ~/patches ~/box'
  scp -P "$SSH_PORT" "${SSH_OPTS[@]}" patches/*.patch "$BOX_USER@$ip:patches/" >/dev/null
  scp -P "$SSH_PORT" "${SSH_OPTS[@]}" box/bootstrap.sh box/run-stack.sh "$BOX_USER@$ip:box/" >/dev/null
  log "running bootstrap + stack on the box (gardener-up takes ~10-15 min)"
  ssh -p "$SSH_PORT" "${SSH_OPTS[@]}" -o ServerAliveInterval=30 "$BOX_USER@$ip" \
    'chmod +x ~/box/*.sh && ~/box/bootstrap.sh && ~/box/run-stack.sh' \
    || die "stack failed on the box — inspect with ./hetzner.sh ssh"
  trust_box_ca "$ip"
  print_access
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  ssh) cmd_ssh ;;
  status) cmd_status ;;
  stack) cmd_stack ;;
  certs) cmd_certs ;;
  tunnel) cmd_tunnel ;;
  *) die "usage: $0 {up|down|ssh|status|stack|certs|tunnel}" ;;
esac
