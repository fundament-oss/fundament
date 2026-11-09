#!/usr/bin/env bash
# Redeploys fundament from the checkout without resetting the databases, so
# clusters and their shoots survive.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
use_toolchain
deploy --profile keep-db
