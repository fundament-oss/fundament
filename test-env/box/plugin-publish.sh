#!/usr/bin/env bash
# Publishes a first-party plugin to the VM's marketplace and approves it, so
# every organization can install it on a shoot: from the console's Plugins
# page, or with fundament_plugin_installation. Publishes as the platform
# maintainer (platform-admin@fundament.io, the system organization) and approves
# with `funops plugin approve` as a seeded DCIM user. Extra arguments go to `functl plugin publish`.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
path=${1:?usage: just test-env plugin-publish <path under plugins/> [functl publish flags]}
shift

[ -f "$STATE/up.ok" ] || die "the environment is not up; run: just test-env up"
use_toolchain
publish_plugin "$path" "$@"
