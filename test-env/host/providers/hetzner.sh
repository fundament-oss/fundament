# shellcheck shell=bash
# Provider: a throwaway Hetzner Cloud server. It boots Hetzner's stock image and
# nixos-anywhere installs the flake's `hetzner` host onto it; the instance data
# goes in with --extra-files. Sourced by host/cli.sh after host/lib.sh.
#
# Not run yet: there is no Hetzner account to test it with.

PROVIDER_CA=fresh
HZ_TYPE=${HZ_TYPE:-cx53}              # 16 vCPU / 32 GB; 16 GB runs out of memory
HZ_IMAGE=${HZ_IMAGE:-ubuntu-24.04}    # replaced by NixOS during the install
HZ_LOCATION=${HZ_LOCATION:-nbg1}
HZ_NAME=${HZ_NAME:-fundament-test-$TEST_ENV_NAME}
HCLOUD_VERSION=${HCLOUD_VERSION:-1.51.0}
HZ_ENV=$TEST_ENV_DIR/host/providers/hetzner.env

hz_init() {
  [ -f "$HZ_ENV" ] || die "missing $HZ_ENV (api_key=...); cp $HZ_ENV.example $HZ_ENV"
  set -a
  # shellcheck disable=SC1090
  . "$HZ_ENV"
  set +a
  export HCLOUD_TOKEN="${api_key:?api_key not set in $HZ_ENV}"
  HC=$STATE_DIR/bin/hcloud
  if [ ! -x "$HC" ]; then
    mkdir -p "$STATE_DIR/bin"
    curl -fsSL "https://github.com/hetznercloud/cli/releases/download/v${HCLOUD_VERSION}/hcloud-$(host_platform).tar.gz" \
      | tar -xz -C "$STATE_DIR/bin" hcloud || die "hcloud download failed"
  fi
}
hc() { "$HC" "$@"; }

provider_doctor() { hz_init; hc server list >/dev/null || { echo "the Hetzner token in $HZ_ENV does not work"; return 1; }; }

provider_create() {
  hz_init
  local key=$INSTANCE_DIR/install-key
  ssh-keygen -q -t ed25519 -N '' -C fundament-install -f "$key"
  hc ssh-key delete "$HZ_NAME" >/dev/null 2>&1 || true
  hc ssh-key create --name "$HZ_NAME" --public-key-from-file "$key.pub" >/dev/null
  log "creating $HZ_NAME ($HZ_TYPE in $HZ_LOCATION); billing starts"
  hc server create --name "$HZ_NAME" --type "$HZ_TYPE" --image "$HZ_IMAGE" \
    --location "$HZ_LOCATION" --ssh-key "$HZ_NAME" >/dev/null || die "server create failed"
  local ip; ip=$(hc server ip "$HZ_NAME")
  write_ssh_config "$ip" "$BOX_SSH_PORT"

  # The instance data the NoCloud reader would otherwise find on a cidata volume.
  local extra=$INSTANCE_DIR/extra-files
  mkdir -p "$extra/var/lib/test-env/instance/authorized_keys.d"
  sed -n 's/^  - //p' "$INSTANCE_DIR/seed/user-data" > "$extra/var/lib/test-env/instance/authorized_keys.d/fundament"

  log "installing NixOS with nixos-anywhere (the server builds its own system)"
  local i
  for i in $(seq 1 60); do
    ssh -i "$key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o LogLevel=ERROR -o ConnectTimeout=8 "root@$ip" true 2>/dev/null && break
    sleep 5
  done
  docker run --rm \
    -v "$NIX_VOLUME:/nix" -v "$TEST_ENV_DIR/nixos:/flake:ro" \
    -v "$extra:/extra:ro" -v "$key:/root/.ssh/id_ed25519:ro" \
    -e NIX_CONFIG="experimental-features = nix-command flakes" \
    "$NIX_IMAGE" sh -c "cp -r /flake /src && nix run github:nix-community/nixos-anywhere -- \
      --flake /src#hetzner --build-on-remote --extra-files /extra \
      -i /root/.ssh/id_ed25519 --ssh-option StrictHostKeyChecking=no --ssh-option UserKnownHostsFile=/dev/null \
      root@$ip" \
    || log "nixos-anywhere exited non-zero (usually the reboot dropping SSH); checking over SSH"
  wait_ssh 40 && return 0
  log "no SSH yet; hard-resetting to boot the installed disk"
  hc server reset "$HZ_NAME" >/dev/null 2>&1 || true
}

provider_status() { hz_init; hc server describe "$HZ_NAME" -o format='{{.Name}} {{.Status}} {{.PublicNet.IPv4.IP}}'; }

provider_destroy() {
  hz_init
  # A failed delete stops `down` before it forgets the instance: the server
  # would keep running, and billing, with nothing pointing at it.
  local servers
  servers=$(hc server list -o noheader -o columns=name) || die "could not list servers; $HZ_NAME may still run"
  if grep -qxF "$HZ_NAME" <<<"$servers"; then
    hc server delete "$HZ_NAME" >/dev/null || die "could not delete server $HZ_NAME; it still runs"
    log "deleted $HZ_NAME; billing stopped"
  fi
  hc ssh-key delete "$HZ_NAME" >/dev/null 2>&1 || true
}
