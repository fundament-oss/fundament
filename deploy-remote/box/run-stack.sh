#!/usr/bin/env bash
# Runs ON the Hetzner NixOS box (after bootstrap.sh). Brings up the full
# fundament + Gardener stack and drives one shoot to "Create Succeeded".
# Re-runnable: cluster-create reuses an existing k3d cluster, gardener-up is
# idempotent. Override the test cluster name with CLUSTER=... (default "smoke").
set -uo pipefail
export PATH="$HOME/.nix-profile/bin:$PATH"
export MISE_NODE_COMPILE=0  # prebuilt node (runs via nix-ld); never build V8 from source
cd ~/fundament
CLUSTER=${CLUSTER:-smoke}
VKC=.dev/gardener/dev-setup/kubeconfigs/virtual-garden/kubeconfig
log() { echo "[$(date +%H:%M:%S)] $*"; }

log "=== STAGE A: cluster-create (k3d + cert-manager) ==="
mise exec -- just cluster-create || log "cluster-create returned nonzero (likely k3d-exists/setup-certs; will verify issuer)"
for i in $(seq 1 20); do
  if mise exec -- kubectl --context k3d-fundament get clusterissuer mkcert-local >/dev/null 2>&1; then
    log "ClusterIssuer mkcert-local present"; break
  fi
  log "issuer not ready (try $i); waiting cert-manager + retrying setup-certs"
  mise exec -- kubectl --context k3d-fundament wait --for=condition=Available deploy/cert-manager deploy/cert-manager-webhook -n cert-manager --timeout=60s 2>/dev/null || true
  mise exec -- just setup-certs >/dev/null 2>&1 || true
  sleep 10
done

log "=== STAGE B: gardener-up (clones gardener, brings up seed; ~10-15 min) ==="
mise exec -- just cluster-worker gardener-up || log "gardener-up returned nonzero"
log "seed: $(mise exec -- kubectl --kubeconfig "$VKC" get seed local --no-headers 2>/dev/null)"

log "=== STAGE C: fundament deploy (skaffold) ==="
mise exec -- bash -c 'export SKAFFOLD_DEFAULT_REPO=localhost:5111; skaffold run --profile env-local --profile local-gardener' \
  || log "skaffold returned nonzero (e.g. organization-api readiness; non-fatal for the shoot path)"

log "=== STAGE D: create test cluster '$CLUSTER' ==="
mise exec -- kubectl --context k3d-fundament exec -n fundament db-1 -c postgres -- \
  psql -U postgres -d fundament -c \
  "INSERT INTO tenant.clusters (organization_id,name,region,kubernetes_version) SELECT id,'$CLUSTER','local','1.31.1' FROM tenant.organizations WHERE name='acme-corp' LIMIT 1 RETURNING id,name;" \
  || log "insert failed (check db pod name / schema)"

log "=== STAGE E: watch shoot '$CLUSTER' -> Create Succeeded ==="
for i in $(seq 1 80); do
  LINE=$(mise exec -- kubectl --kubeconfig "$VKC" get shoots -A --no-headers 2>/dev/null | grep -i "$CLUSTER")
  log "shoot: $LINE"
  echo "$LINE" | grep -qiE "Create Succeeded|100%" && { log "SHOOT SUCCEEDED"; break; }
  sleep 15
done

log "=== SUMMARY ==="
mise exec -- kubectl --kubeconfig "$VKC" get seed local 2>/dev/null
mise exec -- kubectl --kubeconfig "$VKC" get shoots -A 2>/dev/null | grep -iE "$CLUSTER|NAME"
log "=== DONE ==="
