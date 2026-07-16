#!/usr/bin/env bash
# Devbox stack bring-up. Runs ON the box (after bootstrap-dev.sh) with
# STACK_MODE=mock|gardener. Re-runnable, and also the mode SWITCH: mock and
# gardener are deploy profiles on the same k3d cluster, so switching is a
# redeploy — never a box rebuild, never touching the home volume.
#   mock      k3d + fundament with the default mock providers (no Gardener; minutes)
#   gardener  + gardener-up (kind seed, ~10-15 min) + the local-gardener profile
# Fixture data (orgs, API keys, fixture clusters) comes from the db-migrations
# fixtures during the skaffold deploy — identical in both modes, no extra step.
# No `set -e`: stage A/B verify their real success and exit hard themselves
# (mirrors run-stack.sh); the shoot-smoke stages of the TEST role are absent here.
set -uo pipefail
export PATH="$HOME/.nix-profile/bin:/run/current-system/sw/bin:$PATH"
export MISE_NODE_COMPILE=0
cd "$HOME/fundament"
MODE=${STACK_MODE:?STACK_MODE=mock|gardener required}
case "$MODE" in mock|gardener) ;; *) echo "STACK_MODE must be mock|gardener"; exit 1 ;; esac
VKC=.dev/gardener/dev-setup/kubeconfigs/virtual-garden/kubeconfig
log() { echo "[$(date +%H:%M:%S)] $*"; }

log "=== STAGE A: cluster-create (k3d + cert-manager; reuses an existing cluster) ==="
mise exec -- just cluster-create || log "cluster-create returned nonzero (likely k3d-exists/setup-certs; verifying issuer)"
for i in $(seq 1 20); do
  if mise exec -- kubectl --context k3d-fundament get clusterissuer mkcert-local >/dev/null 2>&1; then
    log "ClusterIssuer mkcert-local present"; break
  fi
  log "issuer not ready (try $i); waiting cert-manager + retrying setup-certs"
  mise exec -- kubectl --context k3d-fundament wait --for=condition=Available deploy/cert-manager deploy/cert-manager-webhook -n cert-manager --timeout=60s 2>/dev/null || true
  mise exec -- just setup-certs >/dev/null 2>&1 || true
  sleep 10
done
mise exec -- kubectl --context k3d-fundament get clusterissuer mkcert-local >/dev/null 2>&1 \
  || { log "FATAL: ClusterIssuer mkcert-local never came up"; exit 1; }

if [ "$MODE" = gardener ]; then
  log "=== STAGE B: gardener-up (kind seed; ~10-15 min) ==="
  mise exec -- just cluster-worker gardener-up || log "gardener-up returned nonzero"
  seed=$(mise exec -- kubectl --kubeconfig "$VKC" get seed local --no-headers 2>/dev/null)
  log "seed: $seed"
  echo "$seed" | grep -qw Ready || { log "FATAL: Gardener seed is not Ready"; exit 1; }
fi

log "=== STAGE C: fundament deploy (skaffold, $MODE profile set) ==="
if [ "$MODE" = gardener ]; then
  mise exec -- bash -c 'export SKAFFOLD_DEFAULT_REPO=localhost:5111; skaffold run --profile env-local --profile local-gardener' \
    || log "skaffold returned nonzero (check pods; often app-level readiness, not the deploy)"
else
  mise exec -- bash -c 'export SKAFFOLD_DEFAULT_REPO=localhost:5111; skaffold run --profile env-local' \
    || log "skaffold returned nonzero (check pods; often app-level readiness, not the deploy)"
fi

log "=== DONE ($MODE) ==="
mise exec -- kubectl --context k3d-fundament get pods -n fundament 2>/dev/null | head -20
[ "$MODE" = gardener ] && mise exec -- kubectl --kubeconfig "$VKC" get seed local 2>/dev/null
exit 0
