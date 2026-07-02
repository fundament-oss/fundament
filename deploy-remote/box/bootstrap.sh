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
if [ ! -d "$REPO/.git" ]; then
  # Self-heal a leftover partial/non-git dir (git clone refuses a non-empty target).
  [ -e "$REPO" ] && { echo "   removing incomplete $REPO"; rm -rf "$REPO"; }
  git clone https://github.com/fundament-oss/fundament.git "$REPO"
fi
cd "$REPO"

echo "== 2. apply k3d/gardener coexistence patches =="
# Single-concern patches, applied independently: when master gains one of the changes
# (e.g. the 127.0.0.1 ingress bind), that patch reads "already applied" and the others
# still land. A patch that fits NEITHER state is upstream drift — fail HERE, not 20 min
# later when k3d grabs 172.18.0.0/16 (the subnet Gardener's kind cluster reserves).
for f in "$PATCHES"/*.patch; do
  [ -f "$f" ] || { echo "   FATAL: no patches found in $PATCHES"; exit 1; }
  name=$(basename "$f")
  if git apply --reverse --check "$f" 2>/dev/null; then echo "   $name: already applied"
  elif git apply --check "$f" 2>/dev/null; then git apply "$f" && echo "   $name: applied"
  else echo "   FATAL: $name does not apply — upstream drifted; regenerate the patch"; exit 1; fi
done

echo "== 3. install mise toolchain (baseline ships gcc/make/python so Node builds from source) =="
# mise ships in baseline.nix; fall back to a per-user install if it ever doesn't.
command -v mise >/dev/null 2>&1 || nix profile install nixpkgs#mise
mise trust --yes "$REPO/mise.toml" 2>/dev/null || mise trust --yes 2>/dev/null || true
mise install
echo "== bootstrap done =="
