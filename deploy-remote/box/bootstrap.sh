#!/usr/bin/env bash
# Runs ON the Hetzner NixOS box (pushed there by `hetzner.sh stack`).
# Clones fundament, applies the k3d/gardener coexistence patch, installs the mise
# toolchain. Idempotent — safe to re-run.
set -euo pipefail
export PATH="$HOME/.nix-profile/bin:$PATH"
# Use mise's PREBUILT node, not a from-source build: on NixOS mise otherwise compiles
# node (V8 → ~50 min). The prebuilt binary runs fine via nix-ld (enabled in baseline).
export MISE_NODE_COMPILE=0
REPO="$HOME/fundament"
PATCHES="$HOME/patches"

echo "== 1. clone fundament (public HTTPS) =="
[ -d "$REPO/.git" ] || git clone https://github.com/fundament-oss/fundament.git "$REPO"
cd "$REPO"

echo "== 2. apply k3d/gardener coexistence patch =="
f="$PATCHES/k3d-gardener-coexist.patch"
if [ ! -f "$f" ]; then echo "   MISSING $f"
elif git apply --reverse --check "$f" 2>/dev/null; then echo "   already applied"
elif git apply --check "$f" 2>/dev/null; then git apply "$f" && echo "   applied"
else echo "   WARN: k3d-gardener-coexist does NOT apply cleanly against master"; fi

echo "== 3. install mise toolchain (baseline ships gcc/make/python so Node builds from source) =="
# mise ships in baseline.nix; fall back to a per-user install if it ever doesn't.
command -v mise >/dev/null 2>&1 || nix profile install nixpkgs#mise
mise trust --yes "$REPO/mise.toml" 2>/dev/null || mise trust --yes 2>/dev/null || true
mise install
echo "== bootstrap done =="
