mod terraform-provider
mod e2e
mod cluster-worker

_default:
    @just --list

# Watch for changes to .d2 files and re-generate .svgs
watch-d2:
    d2 --theme=0 --dark-theme=200 --watch docs/assets/*.d2

# Format all code and text in this repo
fmt:
    @find . -type f \( -name "*.md" -o -name "*.adoc" -o -name "*.d2" \) -exec sed -i 's/enterprise/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/g' {} +
    d2 fmt docs/assets/*.d2
    go fmt ./...
    # TODO md fmt

# --- Cluster commands ---

# Create a local k3d cluster for development with local registry
cluster-create:
    k3d cluster create --config=deploy/k3d/config.yaml

# Start the cluster (creates if it doesn't exist)
cluster-start:
    @k3d cluster list fundament > /dev/null 2>&1 && k3d cluster start fundament || just cluster-create

# Stop the cluster without deleting it
cluster-stop:
    k3d cluster stop fundament

# Delete the local k3d cluster and registry
cluster-delete:
    k3d cluster delete fundament
    @k3d registry delete registry.localhost 2>/dev/null || true

# --- Deployment commands ---

# Update helm dependencies
helm-deps:
    helm dependency update deploy/charts/ingress-nginx
    helm dependency update deploy/charts/db
    helm dependency update deploy/charts/fundament

# Deploy to local k3d cluster (development mode, keeps resources on exit)
dev *flags:
    SKAFFOLD_DEFAULT_REPO="localhost:5111" \
    skaffold dev --profile env-local --cleanup=false {{ flags }}

# Deploy to local k3d cluster with hot-reload
dev-hotreload:
    @just dev --profile hotreload

# Deploy to an environment (e.g. local, production)
deploy env:
    skaffold run --profile env-{{ env }}

# Delete deployment, can also be used to remove the deployment created by `just dev`.
undeploy env:
    skaffold delete --profile env-{{ env }}

# View logs from all pods
logs:
    kubectl logs -n fundament -l app.kubernetes.io/instance=fundament --all-containers -f

# Open a shell to the PostgreSQL database
db-shell:
    #!/usr/bin/env bash
    set -euo pipefail
    PASSWORD=$(kubectl get secret -n fundament fundament-db-fun-fundament-api -o jsonpath='{.data.password}' |  {{ if os() == "macos" { "base64 -D" } else { "base64 -d" } }})
    kubectl exec -it -n fundament fundament-db-1 -- env PGPASSWORD="$PASSWORD" psql -h localhost -U fun_fundament_api -d fundament

generate:
    cd db && trek generate --stdout
    go generate -x ./...
    cd console-frontend && buf generate
    cd e2e && buf generate
    just fmt

# Lint all Go code
lint:
    golangci-lint run --new-from-rev $(git rev-parse origin/master) ./...

# Run funops against the local development instance/database
funops *args:
    #!/usr/bin/env bash
    set -euo pipefail
    PASSWORD=$(kubectl --context k3d-fundament get secret -n fundament fundament-db-fun-operator -o jsonpath='{.data.password}' | {{ if os() == "macos" { "base64 -D" } else { "base64 -d" } }})
    DATABASE_URL="postgresql://fun_operator:${PASSWORD}@localhost:54328/fundament" go run ./funops/cmd/funops {{ args }}

# Run functl CLI
functl *args:
    go run ./functl/cmd/functl {{ args }}

# --- Cluster Worker ---

# Set up local Gardener for testing real Gardener client
local-gardener:
    just -f cluster-worker/justfile local-gardener
