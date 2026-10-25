#!/usr/bin/env bash
# Brings the environment up inside the test-env VM: the platform in k3d, a local
# Gardener in kind joined to it, fundament deployed once, a baseline cluster
# whose shoot is ready, and a plugin in the catalog to install on it. Stops at
# the first failure. Running it again is a clean start: the old shoots are
# deleted and the databases reset.
set -euo pipefail
. "$(dirname "$0")/lib.sh"

: "${TEST_ENV_CLUSTER:=e2e-base}"
: "${TEST_ENV_MAX_NODES:=4}"

rm -f "$STATE/up.ok"
: > "$STAGE_LOG"
: > "$UP_LOG"
stage "preflight"
[ -e /etc/fundament-test-env ] || die "not a test-env VM"
findmnt -n /var/lib/docker >/dev/null || die "/var/lib/docker is not on the docker disk"
systemctl is-active -q docker || die "docker is not running"

stage "toolchain"
cd "$REPO"
mise trust --yes >/dev/null
logged mise install --quiet
write_env
use_toolchain

stage "platform cluster"
if k3d cluster list fundament >/dev/null 2>&1; then
  logged delete_shoots
  logged just cluster-delete
fi
logged just cluster-create

# fundament's images do not depend on Gardener, so they build while it starts.
build_images > "$STATE/logs/build.log" 2>&1 &
build_pid=$!
# A failed stage ends the build and the plugin publish too, so a rerun starts clean.
trap 'kill $(jobs -p) 2>/dev/null || true' EXIT

stage "local Gardener"
logged just test-env gardener-start
logged wait_gardener_settled

stage "deploy fundament"
log "waiting for the image build (log: $STATE/logs/build.log)"
wait "$build_pid" || { tail -40 "$STATE/logs/build.log"; die "image build failed"; }
logged shoot_plugin_values
logged deploy_built
logged shoot_plugin_services

stage "node quota"
for type in local-small local-medium; do
  logged just funops organization quota set-nodes acme-corp local "$type" "$TEST_ENV_MAX_NODES"
done

# The plugin the e2e suite installs on the baseline cluster; it builds and
# publishes while the cluster comes up.
publish_plugin cert-manager > "$STATE/logs/plugin-publish.log" 2>&1 &
publish_pid=$!

stage "baseline cluster"
(cd e2e && bun install --frozen-lockfile --silent)
(cd e2e && E2E_CLUSTER_NAME="$TEST_ENV_CLUSTER" E2E_CLUSTER_ID_FILE="$STATE/cluster-id" logged bun scripts/ensure-cluster.ts)
logged just test-env shoot-context "$(cat "$STATE/cluster-id")"

stage "catalog plugin"
wait "$publish_pid" || { tail -40 "$STATE/logs/plugin-publish.log"; die "publishing cert-manager failed"; }

stage "e2e settings"
write_e2e_env
echo "$(git rev-parse HEAD) $(date -Is)" > "$STATE/up.ok"
log "environment up: $(stage_summary)"
