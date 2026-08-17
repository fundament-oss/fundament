#!/usr/bin/env bash
# Prints a user JWT from authn-api for dev tooling that needs a bearer token,
# plugin-publish above all. Defaults to platform-admin@fundament.io: publishing
# needs admin of the system org that owns the first-party plugins, so other dev
# logins are refused.
#
#   export FUNDAMENT_TOKEN=$(./deploy/k3d/dev-token.sh)          # bash/zsh
#   set -gx FUNDAMENT_TOKEN (./deploy/k3d/dev-token.sh)          # fish
set -euo pipefail

AUTHN_URL="${AUTHN_URL:-https://authn.fundament.localhost:8443}"
EMAIL="${FUNDAMENT_EMAIL:-platform-admin@fundament.io}"
PASSWORD="${FUNDAMENT_PASSWORD:-password}"

# -k: the local ingress serves a self-signed certificate.
curl -fsk -X POST "$AUTHN_URL/login/password" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" \
    | yq -p json -r '.access_token'
