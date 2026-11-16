# shellcheck shell=bash
# Helpers for the scripts that run inside the test-env VM.

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
STATE=/var/lib/test-env
ENV_FILE=$STATE/env
E2E_ENV_FILE=$STATE/e2e.env
CAROOT_DIR=$STATE/ca
GARDEN_CONTEXT=kind-gardener-local-garden
# mise's prebuilt Node runs through nix-ld; building it from source takes an hour.
export MISE_NODE_COMPILE=0
# Caches on the disk that outlives the VM (nixos/modules/cache-disk.nix).
CACHE=/var/lib/test-env-cache
export MISE_DATA_DIR=$CACHE/mise GOCACHE=$CACHE/go/build GOMODCACHE=$CACHE/go/mod \
  SKAFFOLD_CACHE_FILE=$CACHE/skaffold/cache BUN_INSTALL_CACHE_DIR=$CACHE/bun \
  GARDENER_REGISTRY_CACHE=$CACHE/gardener-registry
mkdir -p "$CACHE/skaffold"
SEED_CONTEXT=kind-gardener-local

log() { printf '>> %s\n' "$*"; }
die() { printf '!! %s\n' "$*" >&2; exit 1; }

STAGE_LOG=$STATE/logs/stages
UP_LOG=$STATE/logs/up.log

# Run a command with a timestamp on every output line, also into up.log, so a
# bundle shows where the time went.
logged() {
  "$@" 2>&1 | while IFS= read -r line; do printf '%(%T)T %s\n' -1 "$line"; done | tee -a "$UP_LOG"
  return "${PIPESTATUS[0]}"
}
_stage_name=""
_stage_start=0
# Time each stage, and name the one that failed.
stage() {
  _stage_end
  _stage_name=$1
  _stage_start=$SECONDS
  log "== $1"
  printf '%(%T)T == %s\n' -1 "$1" >> "$UP_LOG"
}
_stage_end() {
  [ -n "$_stage_name" ] && echo "$_stage_name $((SECONDS - _stage_start))s" >> "$STAGE_LOG"
  _stage_name=""
}
stage_summary() { _stage_end; paste -sd, "$STAGE_LOG"; }
trap '[ -n "$_stage_name" ] && printf "!! failed in stage: %s\n" "$_stage_name" >&2' ERR

# CA trust inside the VM: setup-certs signs with $CAROOT, Chromium reads the NSS
# database, and Go, curl and kubectl read SSL_CERT_FILE. NixOS keeps /etc/ssl in
# the store, so mkcert cannot install into the system store.
write_env() {
  if [ ! -f "$CAROOT_DIR/rootCA.pem" ]; then
    log "no CA from the host; generating one"
    # mkcert is the repository's (mise.toml), not yet on PATH here.
    (cd "$REPO" && CAROOT=$CAROOT_DIR mise exec -- mkcert -install) || die "generating a CA failed"
  fi
  [ -f "$CAROOT_DIR/rootCA.pem" ] || die "no CA in $CAROOT_DIR"
  mkdir -p "$HOME/.pki/nssdb"
  [ -f "$HOME/.pki/nssdb/cert9.db" ] || certutil -d "sql:$HOME/.pki/nssdb" -N --empty-password
  cat /etc/ssl/certs/ca-certificates.crt "$CAROOT_DIR/rootCA.pem" > "$STATE/ca-bundle.pem"
  cat > "$ENV_FILE" <<EOF
export CAROOT=$CAROOT_DIR
export SSL_CERT_FILE=$STATE/ca-bundle.pem
export NODE_EXTRA_CA_CERTS=$CAROOT_DIR/rootCA.pem
EOF
}

# The repository's tools (go, bun, skaffold, ...) and its env (FUNCTL_*), as
# mise.toml pins them, plus the CA settings write_env wrote.
use_toolchain() {
  cd "$REPO" || return
  local env
  env=$(mise env -s bash) || die "mise env failed"
  eval "$env"
  # shellcheck disable=SC1090
  [ -r "$ENV_FILE" ] && . "$ENV_FILE"
  return 0
}

SKAFFOLD_PROFILES=(--kube-context k3d-fundament --profile env-local --profile local-gardener --default-repo=localhost:5111)

# Build and deploy fundament against the local Gardener. Extra skaffold flags (a
# profile) can be passed.
deploy() { build_images && shoot_plugin_values && deploy_built "$@"; }

# Build fundament's images into the k3d registry, recording them for deploy_built.
build_images() { skaffold build "${SKAFFOLD_PROFILES[@]}" --file-output "$STATE/build.json"; }
deploy_built() { skaffold deploy "${SKAFFOLD_PROFILES[@]}" --build-artifacts "$STATE/build.json" "$@"; }

# Shoot nodes can neither pull from the k3d registry nor resolve
# *.fundament.localhost. So the plugin-controller that cluster-worker puts on
# every shoot comes from Gardener's registry, and reaches marketplace-catalog-api
# and authn-api as NodePorts on the k3d node: the services the plugin sandbox
# uses (shoot_plugin_services). The values go into the file gardener-connect
# generates for the local-gardener profile.
GARDENER_REGISTRY=registry.local.gardener.cloud:5001
shoot_plugin_values() {
  local src ref dst digest node
  src=$(jq -r '.builds[] | select(.imageName == "plugin-controller") | .tag' "$STATE/build.json")
  ref=${src%@*}
  dst=$GARDENER_REGISTRY/plugin-controller:${ref##*:}
  docker pull -q "$src" >/dev/null && docker tag "$src" "$dst" || return
  digest=$(docker push "$dst" | sed -n 's/.*digest: \(sha256:[0-9a-f]*\).*/\1/p')
  [ -n "$digest" ] || die "pushing $dst to Gardener's registry failed"
  node=$(docker inspect k3d-fundament-server-0 --format '{{(index .NetworkSettings.Networks "k3d-fundament").IPAddress}}')
  [ -n "$node" ] || die "no address for k3d-fundament-server-0 on the k3d-fundament network"
  IMAGE=$dst@$digest NODE=$node yq -i '
    .pluginController.provisionShoots = true |
    .pluginController.shoot.image = strenv(IMAGE) |
    .pluginController.shoot.catalogApiUrl = "http://" + strenv(NODE) + ":30080" |
    .pluginController.shoot.authnApiUrl = "http://" + strenv(NODE) + ":30081"' \
    "$REPO/charts/fundament/values-gardener-local.yaml"
}
shoot_plugin_services() {
  kubectl --context k3d-fundament apply \
    -f "$REPO/plugins/sandbox/marketplace-catalog-api-nodeport.yaml" \
    -f "$REPO/plugins/sandbox/authn-api-nodeport.yaml"
}

# gardener-start returns when the seed is registered, but the garden reconciles
# once more; wait until it reports success twice, 30 seconds apart.
wait_gardener_settled() {
  local ok=0 i
  for i in $(seq 1 60); do
    if kubectl --context "$SEED_CONTEXT" get garden -o jsonpath='{.items[0].status.lastOperation.state}' 2>/dev/null | grep -qx Succeeded \
      && kubectl --context "$GARDEN_CONTEXT" get seed local \
        -o jsonpath='{.status.conditions[?(@.type=="GardenletReady")].status}' 2>/dev/null | grep -qx True; then
      ok=$((ok + 1))
      [ "$ok" -ge 2 ] && return 0
    else
      ok=0
    fi
    sleep 30
  done
  die "Gardener did not settle within 30 minutes"
}

write_e2e_env() {
  cat > "$E2E_ENV_FILE" <<EOF
BASE_URL=https://console.fundament.localhost:8443
AUTHN_API_URL=https://authn.fundament.localhost:8443
ORGANIZATION_API_URL=https://organization.fundament.localhost:8443
PLUGIN_PROXY_URL=https://plugin-proxy.fundament.localhost:8443
MARKETPLACE_CATALOG_API_URL=https://marketplace-catalog-api.fundament.localhost:8443
NODE_EXTRA_CA_CERTS=$CAROOT_DIR/rootCA.pem
BROWSER_PATH=/run/current-system/sw/bin/chromium
E2E_CLUSTER_NAME=${TEST_ENV_CLUSTER:-e2e-base}
E2E_GARDEN_CONTEXT=$GARDEN_CONTEXT
E2E_SEED_CONTEXT=$SEED_CONTEXT
E2E_FUNCTL=$STATE/bin/functl
E2E_GARDENER=real
E2E_TRACE=on
EOF
}

# Deletes every shoot and waits until they are gone. A database reset leaves
# the shoots of its clusters behind, since cluster-worker deletes no shoot
# without a cluster row, and this Gardener serves only this environment.
delete_shoots() {
  local shoots ns name
  shoots=$(kubectl --context "$GARDEN_CONTEXT" get shoots -A \
    -o jsonpath='{range .items[*]}{.metadata.namespace} {.metadata.name}{"\n"}{end}' 2>/dev/null) \
    || { log "no garden to list shoots in; none deleted"; return 0; }
  while read -r ns name; do
    [ -n "$name" ] || continue
    kubectl --context "$GARDEN_CONTEXT" -n "$ns" annotate shoot "$name" confirmation.gardener.cloud/deletion=true --overwrite
    kubectl --context "$GARDEN_CONTEXT" -n "$ns" delete shoot "$name" --wait=false
  done <<< "$shoots"
  [ -z "$shoots" ] || kubectl --context "$GARDEN_CONTEXT" wait --for=delete shoots --all -A --timeout=20m
}

# Publishes and approves a first-party plugin (see plugin-publish.sh). A
# subshell, because it exports a functl session of its own.
publish_plugin() ( # path [flags]
  path=$1
  shift
  SYSTEM_ORG=019b4000-0000-7000-8000-000000000000
  login='{"email":"platform-admin@fundament.io","password":"password"}'
  json='Content-Type: application/json'

  token=$(curl -sf https://authn.fundament.localhost:8443/login/password -H "$json" -d "$login" | jq -r .access_token)
  key=$(curl -sf https://organization.fundament.localhost:8443/organization.v1.APIKeyService/CreateAPIKey \
    -H "Authorization: Bearer $token" -H "Fun-Organization: $SYSTEM_ORG" -H "$json" \
    -H "Idempotency-Key: $(cat /proc/sys/kernel/random/uuid)" \
    -d "{\"name\":\"plugin-publish-$(date +%s)\",\"expiresIn\":\"1h\"}" | jq -r .token)
  [ -n "$key" ] && [ "$key" != null ] || die "could not create an API key for platform-admin"

  # A functl session of its own, so the developer's session stays as it is.
  export FUNCTL_CONFIG_DIR=$STATE/functl-publisher
  just functl auth login <<<"$key" >/dev/null
  just functl org set "$SYSTEM_ORG" >/dev/null

  # Shoot nodes pull the plugin image from Gardener's registry (see shoot_plugin_values).
  out=$(PLUGIN_REGISTRY=$GARDENER_REGISTRY just plugins publish "$path" --submit "$@" 2>&1) || { echo "$out"; die "publish failed"; }
  published=$(grep '^published ' <<<"$out" || true)
  plugin_id=$(grep -o ' id=[^ ]*' <<<"$published" | cut -d= -f2)
  version=$(grep -o ' version=[^ ]*' <<<"$published" | cut -d= -f2)
  [ -n "$plugin_id" ] && [ -n "$version" ] || { echo "$out"; die "publish printed no plugin id and version"; }
  # The reviewer must be a DCIM user; alice is one in the seed (db/testdata/030_0101-content.sql).
  just funops plugin approve "$plugin_id" "$version" --reviewer alice@acme-corp.com

  log "$published"
  log "approved: install it from the console's Plugins page or with fundament_plugin_installation"
)
