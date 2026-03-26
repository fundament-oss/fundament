# Cluster Worker Development Tasks
# Run with: just <recipe> (from cluster-worker directory)

set shell := ["bash", "-uc"]

# Pin Gardener version for team consistency (kind is managed by mise.toml)
GARDENER_VERSION := "v1.117.0"

# Gardener directory is in the repo root's .dev folder (shared infrastructure)
# Note: justfile_directory() returns the root when invoked via 'mod'
GARDENER_DIR := env_var_or_default("GARDENER_DIR", justfile_directory() + "/.dev/gardener")
KUBECONFIG_PATH := GARDENER_DIR / "example/provider-local/seed-kind/base/kubeconfig"

# List available recipes
default:
    @just --list cluster-worker

# ============================================================================
# Local Gardener (Optional)
# ============================================================================

# Show pinned Gardener version
versions:
    @echo "Gardener: {{GARDENER_VERSION}}"

# Check if prerequisites for local Gardener are installed
check-prerequisites:
    #!/usr/bin/env bash
    set -e
    missing=()
    command -v docker >/dev/null || missing+=("docker")
    command -v kind >/dev/null || missing+=("kind (mise use -g kind)")
    command -v kubectl >/dev/null || missing+=("kubectl")
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed --version 2>/dev/null | grep -q GNU || missing+=("GNU sed (brew install gnu-sed)")
        tar --version 2>/dev/null | grep -q GNU || missing+=("GNU tar (brew install gnu-tar)")
        command -v ip >/dev/null || missing+=("iproute2mac (brew install iproute2mac)")
    fi
    if [ ${#missing[@]} -gt 0 ]; then
        echo "Missing prerequisites:"
        printf '  - %s\n' "${missing[@]}"
        echo ""
        echo "Install missing tools and try again"
        exit 1
    fi
    echo "All prerequisites installed"

# Start local Gardener (~15 min first time, instant if already running)
gardener-up: check-prerequisites
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"

    if kind get clusters 2>/dev/null | grep -q gardener-local; then
        echo "Gardener already running"
        echo "Check status: just cluster-worker gardener-status"
        GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-connect-k3d
        GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-secret
        exit 0
    fi

    # Clone repo at pinned version if needed
    if [ ! -d "$GARDENER_DIR" ]; then
        echo "Cloning Gardener {{GARDENER_VERSION}}..."
        git clone --depth 1 --branch "{{GARDENER_VERSION}}" https://github.com/gardener/gardener.git "$GARDENER_DIR"
    else
        # Verify version matches
        cd "$GARDENER_DIR"
        CURRENT_VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "unknown")
        if [ "$CURRENT_VERSION" != "{{GARDENER_VERSION}}" ]; then
            echo "Warning: Gardener version mismatch (have: $CURRENT_VERSION, want: {{GARDENER_VERSION}})"
            echo "Run 'rm -rf $GARDENER_DIR' and re-run to update"
        fi
        cd - > /dev/null
    fi

    # Ensure hosts entry
    if ! grep -q "registry.local.gardener.cloud" /etc/hosts; then
        echo "Adding registry.local.gardener.cloud to /etc/hosts (requires sudo)..."
        echo "127.0.0.1 registry.local.gardener.cloud" | sudo tee -a /etc/hosts
    fi

    echo "Starting Gardener (this takes 10-15 minutes on first run)..."
    cd "$GARDENER_DIR"
    make kind-up
    make gardener-up
    echo ""
    GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-connect-k3d
    GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-secret
    echo "Gardener started!"

# Stop local Gardener
gardener-down:
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"

    if ! kind get clusters 2>/dev/null | grep -q gardener-local; then
        echo "Gardener is not running"
        exit 0
    fi

    cd "$GARDENER_DIR"
    make gardener-down 2>/dev/null || true
    make kind-down
    echo "Gardener stopped"

# Delete Gardener kind cluster (for clean restart)
gardener-delete:
    #!/usr/bin/env bash
    if kind get clusters 2>/dev/null | grep -q gardener-local; then
        echo "Deleting existing gardener-local cluster..."
        kind delete cluster --name gardener-local
    fi

# Fix stale port mappings by starting a socat port forward
gardener-fix-ports:
    #!/usr/bin/env bash
    set -e

    # Check if gardener is running
    if ! docker ps --format '{{{{.Names}}' | grep -q gardener-local-control-plane; then
        echo "Error: Gardener not running. Start with: just cluster-worker gardener-up"
        exit 1
    fi

    # Get the port from existing kubeconfig
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    KUBECONFIG="$GARDENER_DIR/example/provider-local/seed-kind/base/kubeconfig"
    PORT=$(grep -oE 'server: https://127.0.0.1:[0-9]+' "$KUBECONFIG" | grep -oE '[0-9]+$' || echo "6443")

    # Check if connection already works
    if timeout 3 kubectl --kubeconfig "$KUBECONFIG" cluster-info &>/dev/null; then
        echo "Connection already working. Nothing to fix."
        exit 0
    fi

    # Check if port is in use (by broken docker mapping)
    if lsof -i :$PORT &>/dev/null; then
        echo "Port $PORT is allocated but not working."
        echo "This usually means Docker needs a restart or the kind cluster needs recreation."
        echo ""
        echo "Options:"
        echo "  1. Restart Docker Desktop"
        echo "  2. Recreate: just cluster-worker gardener-delete && just cluster-worker gardener-up"
        exit 1
    fi

    # Port is free, start socat forward
    docker rm -f gardener-port-forward 2>/dev/null || true
    echo "Starting port forward: localhost:$PORT -> gardener-local-control-plane:6443"
    docker run -d --rm --name gardener-port-forward \
        --network kind \
        -p $PORT:6443 \
        alpine/socat TCP-LISTEN:6443,fork,reuseaddr TCP:gardener-local-control-plane:6443

    echo "Port forward active. Test with: just cluster-worker gardener-status"
    echo "Stop with: docker rm -f gardener-port-forward"

# Check Gardener status
gardener-status:
    #!/usr/bin/env bash
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    KUBECONFIG="$GARDENER_DIR/example/provider-local/seed-kind/base/kubeconfig"

    echo "=== Local Gardener Status ==="
    echo ""

    if ! kind get clusters 2>/dev/null | grep -q gardener-local; then
        echo "Status: NOT RUNNING"
        echo "Start with: just cluster-worker gardener-up"
        exit 1
    fi

    echo "Status: RUNNING"
    KIND_IP=$(docker inspect gardener-local-control-plane --format '{{{{range $net, $conf := .NetworkSettings.Networks}}{{{{if eq $net "kind"}}{{{{$conf.IPAddress}}{{{{end}}{{{{end}}' 2>/dev/null || echo "unknown")
    echo "API Server: https://$KIND_IP:6443 (internal)"
    echo ""

    echo "=== Shoots ==="
    kubectl --kubeconfig "$KUBECONFIG" get shoots -n garden-local 2>/dev/null || echo "No shoots (run 'just cluster-worker gardener-fix-ports' if connection refused)"
    echo ""

    echo "=== Network ==="
    if docker network inspect k3d-fundament --format '{{{{range .Containers}}{{{{.Name}} {{{{end}}' 2>/dev/null | grep -q gardener-local-control-plane; then
        echo "k3d network: Connected"
    else
        echo "k3d network: NOT connected (run: just cluster-worker gardener-connect-k3d)"
    fi

# Connect Docker networks so k3d can reach Gardener
gardener-connect-k3d:
    #!/usr/bin/env bash
    set -e

    KIND_CONTAINER="gardener-local-control-plane"
    K3D_NETWORK="k3d-fundament"
    KIND_NETWORK="kind"

    if ! docker ps --format '{{{{.Names}}' | grep -q "$KIND_CONTAINER"; then
        echo "Error: Gardener is not running. Start with: just cluster-worker gardener-up"
        exit 1
    fi

    if ! docker network ls --format '{{{{.Name}}' | grep -q "^${K3D_NETWORK}$"; then
        echo "Error: k3d network not found. Is the k3d cluster running?"
        exit 1
    fi

    # Connect kind to k3d network
    if docker network inspect "$K3D_NETWORK" --format '{{{{range .Containers}}{{{{.Name}} {{{{end}}' | grep -q "$KIND_CONTAINER"; then
        echo "- $KIND_CONTAINER already connected to $K3D_NETWORK"
    else
        docker network connect "$K3D_NETWORK" "$KIND_CONTAINER"
        echo "- Connected $KIND_CONTAINER to $K3D_NETWORK"
    fi

    # Connect k3d to kind network
    K3D_SERVER=$(docker ps --filter "name=k3d-fundament-server" --format "{{{{.Names}}" | head -1)
    if [ -n "$K3D_SERVER" ]; then
        if docker network inspect "$KIND_NETWORK" --format '{{{{range .Containers}}{{{{.Name}} {{{{end}}' | grep -q "$K3D_SERVER"; then
            echo "- $K3D_SERVER already connected to $KIND_NETWORK"
        else
            docker network connect "$KIND_NETWORK" "$K3D_SERVER"
            echo "- Connected $K3D_SERVER to $KIND_NETWORK"
        fi
    fi

    KIND_IP=$(docker inspect "$KIND_CONTAINER" --format '{{{{range $net, $conf := .NetworkSettings.Networks}}{{{{if eq $net "kind"}}{{{{$conf.IPAddress}}{{{{end}}{{{{end}}')
    echo ""
    echo "Gardener API: https://$KIND_IP:6443"

# Create kubeconfig secret for cluster-worker to connect to Gardener
# Depends on gardener-connect-k3d to ensure networks are connected
gardener-secret: gardener-connect-k3d
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    KUBECONFIG_PATH="$GARDENER_DIR/example/provider-local/seed-kind/base/kubeconfig"

    # Generate kubeconfig from kind if missing
    if [ ! -f "$KUBECONFIG_PATH" ]; then
        echo "Kubeconfig not found, generating from kind cluster..."
        mkdir -p "$(dirname "$KUBECONFIG_PATH")"
        kind get kubeconfig --name gardener-local > "$KUBECONFIG_PATH"
    fi

    # Always use kind network IP - the TLS cert SANs only include kind network IP,
    # not the k3d-fundament network IP. This works because gardener-connect-k3d
    # connects the k3d server to the kind network, allowing pods to reach this IP.
    KIND_IP=$(docker inspect gardener-local-control-plane --format '{{{{range $net, $conf := .NetworkSettings.Networks}}{{{{if eq $net "kind"}}{{{{$conf.IPAddress}}{{{{end}}{{{{end}}')

    if [ -z "$KIND_IP" ]; then
        echo "Error: Could not get kind network IP. Is Gardener running?"
        exit 1
    fi

    # Ensure fundament namespace exists
    kubectl create namespace fundament --dry-run=client -o yaml | kubectl apply -f -

    # Create kubeconfig with kind network IP (not localhost)
    echo "Creating gardener-kubeconfig secret (server: https://$KIND_IP:6443)..."
    TEMP_KUBECONFIG=$(mktemp)
    sed "s|https://127.0.0.1:[0-9]*|https://$KIND_IP:6443|g" "$KUBECONFIG_PATH" > "$TEMP_KUBECONFIG"
    kubectl create secret generic gardener-kubeconfig \
        --from-file=kubeconfig="$TEMP_KUBECONFIG" \
        -n fundament --dry-run=client -o yaml | kubectl apply -f -
    rm "$TEMP_KUBECONFIG"
    echo "Secret created: gardener-kubeconfig"

# ============================================================================
# mini-lab (metal-stack Gardener)
# ============================================================================

MINILAB_DIR := env_var_or_default("MINILAB_DIR", justfile_directory() + "/.dev/mini-lab")
MINILAB_CONTAINER := "metal-control-plane-control-plane"

# Connect mini-lab Docker containers to k3d-fundament network
minilab-connect-k3d:
    #!/usr/bin/env bash
    set -e
    K3D_NETWORK="k3d-fundament"

    if ! docker ps --format '{{{{.Names}}' | grep -q "^{{MINILAB_CONTAINER}}$"; then
        echo "Error: mini-lab not running (container {{MINILAB_CONTAINER}} not found)"
        echo "Start with: cd {{MINILAB_DIR}} && MINI_LAB_FLAVOR=gardener make"
        exit 1
    fi

    if ! docker network ls --format '{{{{.Name}}' | grep -q "^${K3D_NETWORK}$"; then
        echo "Error: k3d network not found. Is the k3d cluster running?"
        exit 1
    fi

    # Connect mini-lab kind container to k3d network
    if docker network inspect "$K3D_NETWORK" --format '{{{{range .Containers}}{{{{.Name}} {{{{end}}' | grep -q "{{MINILAB_CONTAINER}}"; then
        echo "- {{MINILAB_CONTAINER}} already connected to $K3D_NETWORK"
    else
        docker network connect "$K3D_NETWORK" "{{MINILAB_CONTAINER}}"
        echo "- Connected {{MINILAB_CONTAINER}} to $K3D_NETWORK"
    fi

    K3D_IP=$(docker inspect "{{MINILAB_CONTAINER}}" --format '{{{{index .NetworkSettings.Networks "k3d-fundament" "IPAddress"}}')

    # Start socat proxy for virtual garden API if not already running
    # The virtual garden kube-apiserver is a ClusterIP service, so we proxy through socat
    VGARDEN_CLUSTERIP=$(docker exec "{{MINILAB_CONTAINER}}" kubectl get svc virtual-garden-kube-apiserver -n garden -o jsonpath='{.spec.clusterIP}')
    if ! docker exec "{{MINILAB_CONTAINER}}" pgrep -f "socat.*TCP-LISTEN:30443" > /dev/null 2>&1; then
        echo "- Starting socat proxy for virtual garden API (port 30443)..."
        docker exec "{{MINILAB_CONTAINER}}" bash -c "apt-get update -qq && apt-get install -qq -y socat >/dev/null 2>&1" || true
        docker exec -d "{{MINILAB_CONTAINER}}" socat TCP-LISTEN:30443,fork,reuseaddr TCP:$VGARDEN_CLUSTERIP:443
        sleep 1
    else
        echo "- socat proxy already running on port 30443"
    fi

    echo ""
    echo "mini-lab virtual garden API: https://$K3D_IP:30443"

# Create kubeconfig + credentials secrets for cluster-worker to connect to mini-lab virtual garden
minilab-secret: minilab-connect-k3d
    #!/usr/bin/env bash
    set -e

    KIND_KUBECONFIG=$(mktemp)
    kind get kubeconfig --name metal-control-plane > "$KIND_KUBECONFIG"

    # Get virtual garden token and CA from the kind cluster
    echo "Extracting virtual garden kubeconfig..."
    SECRET_NAME=$(kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n garden \
        -l managed-by=secrets-manager,manager-identity=gardener-operator,name=generic-token-kubeconfig \
        --no-headers -o custom-columns=NAME:.metadata.name | head -1)
    if [ -z "$SECRET_NAME" ]; then
        echo "Error: Could not find generic-token-kubeconfig secret. Is Gardener running?"
        rm "$KIND_KUBECONFIG"
        exit 1
    fi

    VIRTUAL_KUBECONFIG=$(mktemp)
    kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n garden "$SECRET_NAME" \
        -o jsonpath='{.data.kubeconfig}' | base64 -d > "$VIRTUAL_KUBECONFIG"

    # Set token from shoot-access-virtual-garden
    TOKEN=$(kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n garden shoot-access-virtual-garden \
        -o jsonpath='{.data.token}' | base64 -d)
    kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" config set-credentials garden --token="$TOKEN"

    # Rewrite server URL to the k3d-fundament network IP (port 30443 = socat proxy to virtual garden API)
    K3D_IP=$(docker inspect "{{MINILAB_CONTAINER}}" --format '{{{{index .NetworkSettings.Networks "k3d-fundament" "IPAddress"}}')
    kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" config set-cluster garden \
        --server="https://$K3D_IP:30443" \
        --insecure-skip-tls-verify=true

    # Ensure fundament namespace exists
    kubectl create namespace fundament --dry-run=client -o yaml | kubectl apply -f -

    echo "Creating gardener-kubeconfig secret (server: https://$K3D_IP:30443)..."
    kubectl create secret generic gardener-kubeconfig \
        --from-file=kubeconfig="$VIRTUAL_KUBECONFIG" \
        -n fundament --dry-run=client -o yaml | kubectl apply -f -

    # Create metal-api credentials secret in the virtual garden
    # The extension-provider-metal reads metalAPIURL/metalAPIHMac from this secret
    METAL_API_URL="http://metal-api.metal-control-plane.svc.cluster.local:8080/metal"
    METAL_API_HMAC=$(kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n metal-control-plane metal-api \
        -o jsonpath='{.data.admin_key}' | base64 -d)

    echo "Creating metal-credentials secret in virtual garden..."
    kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" create secret generic metal-credentials \
        -n garden \
        --from-literal=metalAPIURL="$METAL_API_URL" \
        --from-literal=metalAPIHMac="$METAL_API_HMAC" \
        --from-literal=metalAPIKey="" \
        --dry-run=client -o yaml | kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" apply --validate=false -f -

    # Create seed-api-server ConfigMap so the ControlPlane reconciler accepts the
    # Kind cluster's IP-based API server URL instead of panicking.
    echo ""
    echo "Creating seed-api-server ConfigMap..."
    kubectl --kubeconfig="$KIND_KUBECONFIG" create configmap seed-api-server \
        -n garden \
        --from-literal=url=https://metal-control-plane-control-plane:6443 \
        --dry-run=client -o yaml | kubectl --kubeconfig="$KIND_KUBECONFIG" apply -f - 2>/dev/null \
        && echo "  seed-api-server ConfigMap ready" || echo "  Warning: Could not create seed-api-server ConfigMap"

    # Create project, copy credentials, create SecretBinding
    echo ""
    echo "Creating project 'test' and SecretBinding..."
    kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" apply --validate=false -f - <<PROJEOF 2>/dev/null
    apiVersion: core.gardener.cloud/v1beta1
    kind: Project
    metadata:
      name: test
    spec:
      namespace: garden-test
    PROJEOF
    sleep 3

    # Copy credentials to project namespace
    kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" get secret metal-credentials -n garden -o json 2>/dev/null | \
        jq '.metadata.namespace = "garden-test" | del(.metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp, .metadata.managedFields)' | \
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" apply --validate=false -f - 2>/dev/null || true

    # Create SecretBinding
    kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" apply --validate=false -f - <<SBEOF 2>/dev/null || true
    apiVersion: core.gardener.cloud/v1beta1
    kind: SecretBinding
    metadata:
      name: metal-credentials
      namespace: garden-test
    provider:
      type: metal
    secretRef:
      name: metal-credentials
      namespace: garden-test
    SBEOF
    echo "  Project, credentials, and SecretBinding ready"

    rm "$KIND_KUBECONFIG" "$VIRTUAL_KUBECONFIG"
    echo ""
    echo "Setup complete:"
    echo "  - gardener-kubeconfig secret (in k3d fundament namespace)"
    echo "  - metal-credentials secret (in virtual garden)"
    echo "  - seed-api-server ConfigMap (in Kind garden namespace)"
    echo "  - Project 'test' with SecretBinding ready"

# Check mini-lab Gardener connectivity and status
minilab-status:
    #!/usr/bin/env bash
    echo "=== mini-lab Gardener Status ==="
    echo ""

    if ! docker ps --format '{{{{.Names}}' | grep -q "^{{MINILAB_CONTAINER}}$"; then
        echo "Status: NOT RUNNING"
        echo "Start with: cd {{MINILAB_DIR}} && MINI_LAB_FLAVOR=gardener make"
        exit 1
    fi

    echo "Status: RUNNING"

    K3D_NETWORK="k3d-fundament"
    if docker network inspect "$K3D_NETWORK" --format '{{{{range .Containers}}{{{{.Name}} {{{{end}}' 2>/dev/null | grep -q "{{MINILAB_CONTAINER}}"; then
        K3D_IP=$(docker inspect "{{MINILAB_CONTAINER}}" --format '{{{{index .NetworkSettings.Networks "k3d-fundament" "IPAddress"}}' 2>/dev/null)
        echo "k3d network: Connected (virtual garden API: https://$K3D_IP:30443)"
    else
        echo "k3d network: NOT connected (run: just cluster-worker minilab-connect-k3d)"
    fi
    echo ""

    echo "=== Virtual Garden ==="
    KIND_KUBECONFIG=$(mktemp)
    kind get kubeconfig --name metal-control-plane > "$KIND_KUBECONFIG" 2>/dev/null
    SECRET_NAME=$(kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n garden \
        -l managed-by=secrets-manager,manager-identity=gardener-operator,name=generic-token-kubeconfig \
        --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | head -1)
    if [ -n "$SECRET_NAME" ]; then
        VIRTUAL_KUBECONFIG=$(mktemp)
        kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n garden "$SECRET_NAME" \
            -o jsonpath='{.data.kubeconfig}' | base64 -d > "$VIRTUAL_KUBECONFIG"
        TOKEN=$(kubectl --kubeconfig="$KIND_KUBECONFIG" get secret -n garden shoot-access-virtual-garden \
            -o jsonpath='{.data.token}' | base64 -d 2>/dev/null)
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" config set-credentials garden --token="$TOKEN" > /dev/null 2>&1
        K3D_IP=$(docker inspect "{{MINILAB_CONTAINER}}" --format '{{{{index .NetworkSettings.Networks "k3d-fundament" "IPAddress"}}' 2>/dev/null)
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" config set-cluster garden \
            --server="https://$K3D_IP:30443" --insecure-skip-tls-verify=true > /dev/null 2>&1

        echo "Seeds:"
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" get seeds 2>/dev/null || echo "  Could not list seeds"
        echo ""
        echo "Cloud Profiles:"
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" get cloudprofiles 2>/dev/null || echo "  Could not list cloud profiles"
        echo ""
        echo "Projects:"
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" get projects 2>/dev/null || echo "  None"
        echo ""
        echo "Shoots:"
        kubectl --kubeconfig="$VIRTUAL_KUBECONFIG" get shoots -A 2>/dev/null || echo "  None"

        rm "$VIRTUAL_KUBECONFIG"
    else
        echo "  Could not get virtual garden kubeconfig"
    fi
    rm -f "$KIND_KUBECONFIG"

# ============================================================================
# Testing Helpers
# ============================================================================

# Create a test cluster with short name (required due to 21-char limit)
create-test-cluster name:
    #!/usr/bin/env bash
    set -e
    name="{{name}}"
    if [ ${#name} -gt 10 ]; then
        echo "Error: Name '$name' is too long. Use ≤10 chars (shoot names have 21-char limit)"
        exit 1
    fi
    result=$(kubectl exec -n fundament fundament-db-1 -- psql -U postgres -d fundament -t -c "INSERT INTO tenant.clusters (name, organization_id, region, kubernetes_version, status) SELECT '{{name}}', id, 'local', '1.31.1', 'unspecified' FROM tenant.organizations LIMIT 1 ON CONFLICT (organization_id, name, deleted) DO NOTHING RETURNING id, name;")
    echo ""
    if echo "$result" | grep -q '{{name}}'; then
        echo "Cluster '{{name}}' created. Watch progress:"
    else
        echo "Cluster '{{name}}' already exists. Watch progress:"
    fi
    echo "  just cluster-worker shoots"

# Create a test cluster targeting mini-lab metal partition
create-test-cluster-metal name:
    #!/usr/bin/env bash
    set -e
    name="{{name}}"
    if [ ${#name} -gt 10 ]; then
        echo "Error: Name '$name' is too long. Use ≤10 chars (shoot names have 21-char limit)"
        exit 1
    fi
    result=$(kubectl exec -n fundament fundament-db-1 -- psql -U postgres -d fundament -t -c "INSERT INTO tenant.clusters (name, organization_id, region, kubernetes_version, status) SELECT '{{name}}', id, 'mini-lab', '1.30.8', 'unspecified' FROM tenant.organizations LIMIT 1 ON CONFLICT (organization_id, name, deleted) DO NOTHING RETURNING id, name;")
    echo ""
    if echo "$result" | grep -q '{{name}}'; then
        echo "Cluster '{{name}}' created with region 'mini-lab'. Watch progress:"
    else
        echo "Cluster '{{name}}' already exists. Watch progress:"
    fi
    echo "  just cluster-worker shoots"

# Delete a test cluster
delete-test-cluster name:
    kubectl exec -n fundament fundament-db-1 -- psql -U postgres -d fundament -c \
        "UPDATE tenant.clusters SET deleted = NOW() WHERE name = '{{name}}' AND deleted IS NULL;"

# Watch shoots in Gardener
shoots:
    #!/usr/bin/env bash
    if ! docker ps --format '{{{{.Names}}' | grep -q gardener-local-control-plane; then
        echo "Error: Gardener is not running. Start with: just cluster-worker gardener-up"
        exit 1
    fi
    docker exec gardener-local-control-plane kubectl get shoots -A -w

