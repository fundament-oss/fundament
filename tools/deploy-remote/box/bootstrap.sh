#!/usr/bin/env bash
# Runs ON the Hetzner NixOS box (pushed there by `hetzner.sh stack`).
# Clones fundament, checks out the ref the operator deployed from, installs the mise
# toolchain. Idempotent — safe to re-run.
set -euo pipefail
export PATH="$HOME/.nix-profile/bin:$PATH"
# Use mise's PREBUILT node, not a from-source build: on NixOS mise otherwise compiles
# node (V8 → ~50 min). The prebuilt binary runs fine via nix-ld (enabled in baseline).
export MISE_NODE_COMPILE=0
REPO="$HOME/fundament"

echo "== 1. clone fundament (public HTTPS) =="
if [ ! -d "$REPO/.git" ]; then
  # Self-heal a leftover partial/non-git dir (git clone refuses a non-empty target).
  [ -e "$REPO" ] && { echo "   removing incomplete $REPO"; rm -rf "$REPO"; }
  git clone https://github.com/fundament-oss/fundament.git "$REPO"
fi
cd "$REPO"

echo "== 2. check out the deployed ref =="
# hetzner.sh stack passes the branch the operator deployed from, so the box tests
# YOUR branch (as PUSHED — not your local working tree), not whatever master is
# today. Re-runs re-fetch, so a re-run picks up newly pushed commits.
if [ -n "${FUNDAMENT_REF:-}" ]; then
  git fetch origin "$FUNDAMENT_REF" \
    || { echo "   FATAL: cannot fetch '$FUNDAMENT_REF' — is your branch pushed?"; exit 1; }
  git checkout -q FETCH_HEAD
  echo "   at $(git rev-parse --short HEAD) ($FUNDAMENT_REF)"
else
  echo "   FUNDAMENT_REF not set — staying on the default branch"
fi

echo "== 3. install mise toolchain (prebuilt node via nix-ld) =="
# mise ships in baseline.nix; fall back to a per-user install if it ever doesn't.
command -v mise >/dev/null 2>&1 || nix profile install nixpkgs#mise
mise trust --yes "$REPO/mise.toml" 2>/dev/null || mise trust --yes 2>/dev/null || true
mise install
echo "== bootstrap done =="
