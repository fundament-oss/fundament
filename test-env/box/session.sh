#!/usr/bin/env bash
# Runs a long recipe (up, test) in the tmux session the host opened for it, so
# it outlives the SSH connection. Its output and exit code stay for the host.
set -uo pipefail
name=$1
shift
logs=/var/lib/test-env/logs
mkdir -p "$logs"
rm -f "$logs/$name.rc"
env -u SSH_CLIENT -u SSH2_CLIENT just test-env "$@" 2>&1 | tee "$logs/$name.out"
echo "${PIPESTATUS[0]}" > "$logs/$name.rc"
