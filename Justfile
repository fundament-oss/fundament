mod terraform-provider 'terraform-provider'
mod e2e 'e2e'

# KUBECONFIG is set via mise.toml to isolate from other clusters
# This variable is used by cluster-create/cluster-start to write the kubeconfig
KUBECONFIG := justfile_directory() / "deploy/k3d/kubeconfig.yaml"

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
    k3d kubeconfig get fundament > {{ KUBECONFIG }}
    @echo "Kubeconfig written to {{ KUBECONFIG }}"

# Start the cluster (creates if it doesn't exist)
cluster-start:
    @k3d cluster list fundament > /dev/null 2>&1 && k3d cluster start fundament && k3d kubeconfig get fundament > {{ KUBECONFIG }} || just cluster-create

# Stop the cluster without deleting it
cluster-stop:
    k3d cluster stop fundament

# Delete the local k3d cluster and registry
cluster-delete:
    k3d cluster delete fundament
    @k3d registry delete registry.localhost 2>/dev/null || true
    @rm -f {{ KUBECONFIG }}

# Show cluster info (verifies connection)
cluster-info:
    kubectl cluster-info

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

# One-time deploy to local k3d cluster
deploy-local:
    SKAFFOLD_DEFAULT_REPO="localhost:5111" \
    skaffold run --profile env-local

# Delete local deployment
undeploy-local:
    skaffold delete --profile env-local

# View logs from all pods
logs:
    kubectl logs -n fundament -l app.kubernetes.io/instance=fundament --all-containers -f

# View logs from cluster-worker
logs-cluster-worker:
    kubectl logs -n fundament -l app.kubernetes.io/component=cluster-worker -f

# Open a shell to the PostgreSQL database
db-shell:
    #!/usr/bin/env bash
    set -euo pipefail
    PASSWORD=$(kubectl get secret -n fundament fundament-db-fun-fundament-api -o jsonpath='{.data.password}' |  {{ if os() == "macos" { "base64 -D" } else { "base64 -d" } }})
    kubectl exec -it -n fundament fundament-db-1 -- env PGPASSWORD="$PASSWORD" psql -h localhost -U fun_fundament_api -d fundament

# Run a SQL command against the database
db-sql cmd:
    kubectl exec -it -n fundament fundament-db-1 -- psql -U postgres -d fundament -c "{{ cmd }}"

# List pods
pods:
    kubectl get pods -n fundament

generate:
    cd db && trek generate --stdout
    go generate -x ./...
    cd console-frontend && buf generate
    cd e2e && buf generate
    just fmt

# Lint all Go code
lint:
    golangci-lint run --new-from-rev $(git rev-parse origin/master) ./...

# Run functl against the local development instance/database
functl *args:
    #!/usr/bin/env bash
    set -euo pipefail
    PASSWORD=$(kubectl --context k3d-fundament get secret -n fundament fundament-db-fun-operator -o jsonpath='{.data.password}' | {{ if os() == "macos" { "base64 -D" } else { "base64 -d" } }})
    DATABASE_URL="postgresql://fun_operator:${PASSWORD}@localhost:54328/fundament" go run ./functl/cmd/functl {{ args }}

# Run fundament CLI
fundament *args:
    go run ./fundament-cli/cmd/fundament {{ args }}

# --- Local Gardener (for testing real client) ---

# Directory where gardener repo is cloned
gardener_dir := justfile_directory() / ".dev/gardener"
gardener_kubeconfig := justfile_directory() / ".dev/gardener-kubeconfig.yaml"

# Clone the Gardener repo (needed for local setup)
gardener-clone:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -d "{{ gardener_dir }}" ]; then
        echo "Gardener repo already exists at {{ gardener_dir }}"
        echo "To update: cd {{ gardener_dir }} && git pull"
    else
        mkdir -p "{{ justfile_directory() }}/.dev"
        echo "Cloning gardener/gardener to {{ gardener_dir }}..."
        git clone --depth 1 https://github.com/gardener/gardener.git "{{ gardener_dir }}"
    fi

# Set up /etc/hosts entry for local registry (requires sudo)
gardener-hosts:
    #!/usr/bin/env bash
    set -euo pipefail
    if grep -q "registry.local.gardener.cloud" /etc/hosts; then
        echo "Host entry already exists"
    else
        echo "Adding registry.local.gardener.cloud to /etc/hosts (requires sudo)..."
        echo "127.0.0.1 registry.local.gardener.cloud" | sudo tee -a /etc/hosts
    fi

# Create local Gardener environment (kind cluster + Gardener components)
# Requires nix for GNU tools (coreutils, sed, tar, grep, gzip)
gardener-up: gardener-clone
    #!/usr/bin/env bash
    set -euo pipefail
    # Check hosts entry
    if ! grep -q "registry.local.gardener.cloud" /etc/hosts; then
        echo "ERROR: Missing /etc/hosts entry. Run this first:"
        echo "  echo '127.0.0.1 registry.local.gardener.cloud' | sudo tee -a /etc/hosts"
        exit 1
    fi
    echo "Starting Gardener setup in nix-shell (provides GNU tools)..."
    nix-shell "{{ justfile_directory() }}/.dev/shell.nix" --run "cd '{{ gardener_dir }}' && make kind-up && make gardener-up"
    echo ""
    echo "Local Gardener is ready!"
    echo "Run 'just gardener-kubeconfig' to get the admin kubeconfig"

# Get admin kubeconfig for local Gardener
gardener-kubeconfig:
    #!/usr/bin/env bash
    set -euo pipefail
    # Use the kind cluster kubeconfig directly - it has admin access to the Garden
    cp "{{ gardener_dir }}/example/gardener-local/kind/local/kubeconfig" "{{ gardener_kubeconfig }}"
    echo "Kubeconfig written to {{ gardener_kubeconfig }}"

# Stop local Gardener (deletes kind cluster)
gardener-down:
    #!/usr/bin/env bash
    set -euo pipefail
    nix-shell "{{ justfile_directory() }}/.dev/shell.nix" --run "cd '{{ gardener_dir }}' && make kind-down"
    rm -f "{{ gardener_kubeconfig }}"
    echo "Local Gardener stopped"

# Check local Gardener status
gardener-status:
    #!/usr/bin/env bash
    set -euo pipefail
    if kind get clusters 2>/dev/null | grep -q "gardener-local"; then
        echo "Local Gardener is running"
        KUBECONFIG="{{ gardener_dir }}/example/gardener-local/kind/local/kubeconfig" \
            kubectl get seeds,shoots -A 2>/dev/null || true
    else
        echo "Local Gardener is not running"
        echo "Run 'just gardener-up' to start it"
    fi

# Run cluster-worker against local Gardener
run-cluster-worker-local:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f "{{ gardener_kubeconfig }}" ]; then
        echo "No Gardener kubeconfig found. Run 'just gardener-kubeconfig' first."
        exit 1
    fi
    echo "Starting cluster-worker with local Gardener..."
    echo "Note: Requires port-forward to database: kubectl port-forward -n fundament fundament-db-1 5432:5432"
    GARDENER_MODE=real \
    GARDENER_KUBECONFIG="{{ gardener_kubeconfig }}" \
    GARDENER_NAMESPACE=garden-local \
    GARDENER_PROVIDER_TYPE=local \
    GARDENER_CLOUD_PROFILE=local \
    GARDENER_SECRET_BINDING_NAME=local \
    GARDENER_REGION=local \
    GARDENER_MACHINE_TYPE=local \
    GARDENER_MACHINE_IMAGE_NAME=local \
    GARDENER_MACHINE_IMAGE_VER=1.0.0 \
    GARDENER_KUBERNETES_VERSION=1.31.1 \
    DATABASE_URL="postgres://fun_cluster_worker:$(kubectl get secret -n fundament fundament-db-fun-cluster-worker -o jsonpath='{.data.password}' | base64 -d)@localhost:5432/fundament?sslmode=disable" \
    go run ./cluster-worker/cmd/cluster-worker
