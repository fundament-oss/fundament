#!/usr/bin/env bash
# Runs the e2e scenarios inside the test-env VM against the real Gardener, then
# always writes a bundle. Exits with the scenarios' result.
set -uo pipefail
. "$(dirname "$0")/lib.sh"
tags=${1:-}

[ -f "$STATE/up.ok" ] || die "the environment is not up; run: just test-env up"
use_toolchain
# A bundle on every exit, also when the setup below fails.
trap '"$(dirname "$0")/collect.sh"' EXIT
set -a
# shellcheck disable=SC1090
. "$E2E_ENV_FILE"
set +a
mkdir -p "$STATE/bin"
go build -o "$STATE/bin/functl" ./functl/cmd/functl || die "functl build failed"

cd e2e || exit 1
rm -rf reports
bun install --frozen-lockfile --silent || die "bun install failed"
rc=0
bun run test:full ${tags:+--tags "$tags"} || rc=$?
exit $rc
