#!/usr/bin/env bash
# Devbox user-layer bootstrap. Runs ON the box as the fundament user (pushed by
# `devbox up` / `devbox stack`); operates ONLY in $HOME, which is the persistent
# volume — so everything it installs survives box destruction. Idempotent by
# design: a re-run updates, never duplicates, and NEVER touches your checkout
# or working tree (the repo on the volume is the source of truth, not origin).
# This is also the whole "persistent machine adoption" story: any box with the
# system layer (docker, nix-ld, dev packages) + this script = a dev environment.
set -euo pipefail
export PATH="$HOME/.nix-profile/bin:/run/current-system/sw/bin:$PATH"
# Prebuilt node via nix-ld — never compile V8 from source (~50 min).
export MISE_NODE_COMPILE=0
REPO="$HOME/fundament"

echo "== 1. repo on the volume (clone once; existing checkout/WIP untouched) =="
if [ ! -d "$REPO/.git" ]; then
  [ -e "$REPO" ] && { echo "   removing incomplete $REPO"; rm -rf "$REPO"; }
  git clone https://github.com/fundament-oss/fundament.git "$REPO"
else
  # Fetch only — your branch, uncommitted work, and stashes are yours.
  git -C "$REPO" fetch origin --prune 2>/dev/null \
    || echo "   WARN: fetch failed (offline/auth?) — continuing with the local repo"
fi

echo "== 2. mise toolchain (cached on the volume; fast when warm) =="
command -v mise >/dev/null 2>&1 || nix profile install nixpkgs#mise
mise trust --yes "$REPO/mise.toml" 2>/dev/null || true
(cd "$REPO" && mise install)

echo "== 3. Claude Code (native installer into ~/.local — persists + auto-updates) =="
if [ ! -x "$HOME/.local/bin/claude" ]; then
  curl -fsSL https://claude.ai/install.sh | bash
else
  echo "   claude present: $("$HOME/.local/bin/claude" --version 2>/dev/null || echo '?') (self-updates)"
fi
# Make sure login shells see it (idempotent).
if ! grep -qs 'local/bin' "$HOME/.bashrc" 2>/dev/null; then
  echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"
fi

echo "== 4. per-dev CA — name-constrained to fundament.localhost =="
# Generated with openssl (mkcert can't emit name constraints, but reuses an
# existing CA found in its CAROOT — verified upstream behavior). The critical
# nameConstraints extension means clients (browsers, Go, curl) reject anything
# this CA signs outside *.fundament.localhost, even if the key leaks; the IP
# exclusions stop unconstrained IP SANs. Trust on the laptop is cycled by
# devbox up/down, not permanent.
CAROOT=$(cd "$REPO" && mise exec -- mkcert -CAROOT)
mkdir -p "$CAROOT"
if [ ! -f "$CAROOT/rootCA.pem" ]; then
  echo "   generating constrained CA in $CAROOT"
  cat > "$CAROOT/openssl-ca.cnf" <<'EOF'
[req]
distinguished_name = dn
x509_extensions = v3_ca
prompt = no

[dn]
O  = fundament devbox
CN = fundament devbox CA

[v3_ca]
basicConstraints = critical, CA:TRUE
keyUsage = critical, keyCertSign, cRLSign
subjectKeyIdentifier = hash
nameConstraints = critical, @name_constraints

[name_constraints]
permitted;DNS.0 = fundament.localhost
excluded;IP.0 = 0.0.0.0/0.0.0.0
excluded;IP.1 = 0:0:0:0:0:0:0:0/0:0:0:0:0:0:0:0
EOF
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 \
    -out "$CAROOT/rootCA-key.pem" 2>/dev/null
  chmod 600 "$CAROOT/rootCA-key.pem"
  openssl req -x509 -new -nodes -key "$CAROOT/rootCA-key.pem" -days 3650 \
    -config "$CAROOT/openssl-ca.cnf" -extensions v3_ca -out "$CAROOT/rootCA.pem"
  echo "   $(openssl x509 -in "$CAROOT/rootCA.pem" -noout -subject)"
else
  echo "   CA present in $CAROOT (stable across box recreations)"
fi

echo "== 5. optional personal dotfiles (set DEVBOX_DOTFILES=<git-url> to enable) =="
if [ -n "${DEVBOX_DOTFILES:-}" ] && [ ! -d "$HOME/.dotfiles" ]; then
  git clone "$DEVBOX_DOTFILES" "$HOME/.dotfiles" \
    && { [ -x "$HOME/.dotfiles/install.sh" ] && "$HOME/.dotfiles/install.sh" || true; } \
    || echo "   WARN: dotfiles clone failed — continuing"
fi

echo "== bootstrap-dev done =="
echo "   Claude Code: run 'claude' in an ssh session for the one-time OAuth login"
echo "   (credentials persist on the volume; future boxes are already logged in)"
