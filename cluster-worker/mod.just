# Cluster Worker Development Tasks (gardener-operator path)
# Run with: just cluster-worker <recipe>
#
# This uses the gardener-operator deployment method, which manages the entire
# Gardener control plane via a single Garden CRD. This is the recommended
# approach as of Gardener v1.80+ and replaces the legacy make kind-up/gardener-up.

set shell := ["bash", "-uc"]

# Pin Gardener version for team consistency
GARDENER_VERSION := "v1.138.0"

# Gardener directory is in the repo root's .dev folder (shared infrastructure)
GARDENER_DIR := env_var_or_default("GARDENER_DIR", justfile_directory() + "/.dev/gardener")

# KinD cluster name used by the operator path
KIND_CLUSTER := "gardener-operator-local"
KIND_CONTAINER := KIND_CLUSTER + "-control-plane"

# List available recipes
default:
    @just --list cluster-worker

# ============================================================================
# Local Gardener (Operator Path)
# ============================================================================

# Show pinned Gardener version
versions:
    @echo "Gardener: {{GARDENER_VERSION}} (operator path)"

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

# Start local Gardener via gardener-operator (~15 min first time)
gardener-up: check-prerequisites
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"

    # Ensure hosts entry (prompt for sudo early, before long-running steps)
    if ! grep -q "registry.local.gardener.cloud" /etc/hosts; then
        echo "Adding registry.local.gardener.cloud to /etc/hosts (requires sudo)..."
        echo "127.0.0.1 registry.local.gardener.cloud" | sudo tee -a /etc/hosts
    fi

    if kind get clusters 2>/dev/null | grep -q "{{KIND_CLUSTER}}"; then
        echo "Gardener already running ({{KIND_CLUSTER}})"
        echo "Check status: just cluster-worker gardener-status"
        GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-connect
        GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-secret
        exit 0
    fi

    # Clone repo at pinned version if needed
    if [ ! -d "$GARDENER_DIR" ]; then
        echo "Cloning Gardener {{GARDENER_VERSION}}..."
        git -c advice.detachedHead=false clone --depth 1 --branch "{{GARDENER_VERSION}}" \
            https://github.com/gardener/gardener.git "$GARDENER_DIR"
    else
        cd "$GARDENER_DIR"
        if ! git tag --points-at HEAD 2>/dev/null | grep -qx "{{GARDENER_VERSION}}"; then
            CURRENT_VERSION=$(git describe --tags 2>/dev/null || echo "unknown")
            echo "Error: Gardener version mismatch (have: $CURRENT_VERSION, want: {{GARDENER_VERSION}})"
            echo "Run 'rm -rf $GARDENER_DIR' and re-run to update"
            exit 1
        fi
        cd - > /dev/null
    fi

    echo "Starting Gardener via operator (this takes 10-15 minutes on first run)..."
    cd "$GARDENER_DIR"

    # Pre-install helm into Gardener's tools directory if not present.
    # Gardener's Makefile downloads helm via a script that can fail on checksum
    # verification for certain versions. Using a direct binary download is more reliable.
    ARCH="$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/')"
    HELM_DIR="hack/tools/bin/$ARCH"
    if [ ! -x "$HELM_DIR/helm" ]; then
        mkdir -p "$HELM_DIR"
        HELM_VERSION=$(grep 'HELM_VERSION' hack/tools.mk | head -1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "v3.17.3")
        echo "Pre-installing helm $HELM_VERSION..."
        curl -sSfL "https://get.helm.sh/helm-${HELM_VERSION}-${ARCH}.tar.gz" | tar xz -C "$HELM_DIR" --strip-components=1 "${ARCH}/helm"
        chmod +x "$HELM_DIR/helm"
    fi

    # Unset SKAFFOLD_DEFAULT_REPO so Gardener uses its own registry
    # (registry.local.gardener.cloud:5001), not the fundament k3d registry.
    unset SKAFFOLD_DEFAULT_REPO
    make kind-single-node-up
    make operator-up
    make operator-seed-up

    echo ""
    GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-connect
    GARDENER_DIR="$GARDENER_DIR" just cluster-worker gardener-secret
    echo "Gardener started! (operator path)"

# Stop local Gardener
gardener-down:
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"

    if ! kind get clusters 2>/dev/null | grep -q "{{KIND_CLUSTER}}"; then
        echo "Gardener is not running"
        exit 0
    fi

    cd "$GARDENER_DIR"
    make operator-seed-down 2>/dev/null || true
    make kind-single-node-down
    echo "Gardener stopped"

# Delete Gardener kind cluster (for clean restart)
gardener-delete:
    #!/usr/bin/env bash
    if kind get clusters 2>/dev/null | grep -q "{{KIND_CLUSTER}}"; then
        echo "Deleting {{KIND_CLUSTER}} cluster..."
        kind delete cluster --name "{{KIND_CLUSTER}}"
    fi

# Check Gardener status
gardener-status:
    #!/usr/bin/env bash
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    RUNTIME_KC="$GARDENER_DIR/dev-setup/kubeconfigs/runtime/kubeconfig"
    VIRTUAL_KC="$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"

    echo "=== Local Gardener Status (operator) ==="
    echo ""

    if ! kind get clusters 2>/dev/null | grep -q "{{KIND_CLUSTER}}"; then
        echo "Status: NOT RUNNING"
        echo "Start with: just cluster-worker gardener-up"
        exit 1
    fi

    echo "Status: RUNNING"
    K3D_IP=$(docker inspect "{{KIND_CONTAINER}}" --format '{{{{range $net, $conf := .NetworkSettings.Networks}}{{{{if eq $net "k3d-fundament"}}{{{{$conf.IPAddress}}{{{{end}}{{{{end}}' 2>/dev/null || echo "not connected")
    echo "API Server: https://$K3D_IP:6443 (k3d-fundament network)"
    echo ""

    echo "=== Garden ==="
    if [ -f "$RUNTIME_KC" ]; then
        kubectl --kubeconfig "$RUNTIME_KC" get garden -n garden 2>/dev/null || echo "(garden not ready)"
    else
        echo "Runtime kubeconfig not found at $RUNTIME_KC"
    fi
    echo ""

    echo "=== Shoots ==="
    if [ -f "$VIRTUAL_KC" ]; then
        kubectl --kubeconfig "$VIRTUAL_KC" get shoots --all-namespaces 2>/dev/null || echo "(no shoots or virtual garden not ready)"
    elif [ -f "$RUNTIME_KC" ]; then
        # Fall back to runtime kubeconfig (shoots may be visible there too)
        kubectl --kubeconfig "$RUNTIME_KC" get shoots --all-namespaces 2>/dev/null || echo "(no shoots)"
    else
        echo "No kubeconfig found"
    fi
    echo ""

    echo "=== Network ==="
    if docker network inspect k3d-fundament --format '{{{{range .Containers}}{{{{.Name}} {{{{end}}' 2>/dev/null | grep -q "{{KIND_CONTAINER}}"; then
        echo "k3d network: Connected"
    else
        echo "k3d network: NOT connected (run: just cluster-worker gardener-connect)"
    fi

# Connect Docker networks so k3d can reach Gardener
gardener-connect:
    #!/usr/bin/env bash
    set -e

    KIND_CONTAINER="{{KIND_CONTAINER}}"
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
    K3D_IP=$(docker inspect "$KIND_CONTAINER" --format '{{{{range $net, $conf := .NetworkSettings.Networks}}{{{{if eq $net "'"$K3D_NETWORK"'"}}{{{{$conf.IPAddress}}{{{{end}}{{{{end}}')

    # Ensure the k3d network IP is in the API server cert SANs.
    # The cert is generated at kind cluster creation and only includes IPs
    # that existed at that time. Since we connect the k3d network afterwards,
    # the k3d IP is typically missing. Regenerate the cert to include both IPs.
    CERT_SANS=$(docker exec "$KIND_CONTAINER" cat /etc/kubernetes/pki/apiserver.crt 2>/dev/null | openssl x509 -noout -ext subjectAltName 2>/dev/null || echo "")
    if ! echo "$CERT_SANS" | grep -q "$K3D_IP"; then
        echo "- Regenerating API server cert to include k3d IP ($K3D_IP)..."
        # Must pass --service-cidr matching the actual cluster config, otherwise
        # kubeadm uses the default (10.96.0.0/12) and the cert gets the wrong
        # service IP in its SANs, breaking in-cluster TLS (e.g., gardenlet).
        SVC_CIDR=$(docker exec "$KIND_CONTAINER" kubectl get cm -n kube-system kubeadm-config -o jsonpath='{.data.ClusterConfiguration}' | grep serviceSubnet | awk '{print $2}')
        docker exec "$KIND_CONTAINER" sh -c "
            rm -f /etc/kubernetes/pki/apiserver.crt /etc/kubernetes/pki/apiserver.key
            kubeadm init phase certs apiserver --apiserver-cert-extra-sans=$K3D_IP,$KIND_IP,127.0.0.1,localhost --service-cidr=$SVC_CIDR
        "
        # Restart the API server so it picks up the new cert.
        # Killing it is safe — kubelet will restart it immediately.
        # docker exec returns 143 (SIGTERM) when the killed process was its entrypoint,
        # so we suppress that exit code.
        docker exec "$KIND_CONTAINER" pkill -f 'kube-apiserver' || true
        echo "- Waiting for API server to restart..."
        sleep 5
        echo "- API server cert updated with SANs: $KIND_IP, $K3D_IP"
    else
        echo "- k3d IP ($K3D_IP) already in API server cert SANs"
    fi

    API_IP="$K3D_IP"
    echo ""
    echo "Gardener API: https://$API_IP:6443"

    # Write discovered IPs to generated values file (used by local-gardener Skaffold profile)
    GARDENER_VALUES="{{GARDENER_DIR}}/../../charts/fundament/values-gardener-local.yaml"
    printf '# Generated by: just cluster-worker gardener-connect\ngardener:\n  apiServerIP: "%s"\nshootProxy:\n  gardenerNodeIP: "%s"\n' "$API_IP" "$K3D_IP" > "$GARDENER_VALUES"
    echo "- Wrote gardener values: apiServerIP=$API_IP, shootProxy.gardenerNodeIP=$K3D_IP"

    # Annotate CoreDNS so Reloader restarts it when coredns-custom ConfigMap changes
    if kubectl --context k3d-fundament -n kube-system get deploy coredns &>/dev/null; then
        kubectl --context k3d-fundament -n kube-system annotate deployment coredns \
            reloader.stakater.com/auto=true --overwrite 2>/dev/null
        echo "- Annotated CoreDNS for Reloader auto-restart"
    fi

# Create kubeconfig secret for cluster-worker to connect to Gardener
gardener-secret: gardener-connect
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    VIRTUAL_KC="$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"

    # The operator path runs the Gardener API in a virtual garden cluster.
    # The cluster-worker needs the virtual garden kubeconfig (not the runtime one)
    # because shoots, projects, and other Gardener resources live there.
    # The server URL (api.virtual-garden.local.gardener.cloud) is routed from k3d
    # via the shoot-proxy's CoreDNS + socat -> Istio NodePort on the kind node.
    if [ ! -f "$VIRTUAL_KC" ]; then
        echo "Error: Virtual garden kubeconfig not found at $VIRTUAL_KC"
        echo "Is Gardener fully started? Run: just cluster-worker gardener-up"
        exit 1
    fi

    echo "Using virtual garden kubeconfig"

    # Ensure fundament namespace exists
    kubectl create namespace fundament --dry-run=client -o yaml | kubectl apply -f -

    echo "Creating gardener-kubeconfig secret (server: api.virtual-garden.local.gardener.cloud)..."
    kubectl create secret generic gardener-kubeconfig \
        --from-file=kubeconfig="$VIRTUAL_KC" \
        -n fundament --dry-run=client -o yaml | kubectl apply -f -
    echo "Secret created: gardener-kubeconfig"

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
    result=$(kubectl exec -n fundament fundament-db-1 -c postgres -- psql -U postgres -d fundament -t -c "INSERT INTO tenant.clusters (name, organization_id, region, kubernetes_version, status) SELECT '{{name}}', id, 'local', '1.31.1', 'unspecified' FROM tenant.organizations LIMIT 1 ON CONFLICT (organization_id, name, deleted) DO NOTHING RETURNING id, name;")
    echo ""
    if echo "$result" | grep -q '{{name}}'; then
        echo "Cluster '{{name}}' created. Watch progress:"
    else
        echo "Cluster '{{name}}' already exists. Watch progress:"
    fi
    echo "  just cluster-worker shoots"

# Delete a test cluster
delete-test-cluster name:
    kubectl exec -n fundament fundament-db-1 -c postgres -- psql -U postgres -d fundament -c \
        "UPDATE tenant.clusters SET deleted = NOW() WHERE name = '{{name}}' AND deleted IS NULL;"

# Watch shoots in Gardener
shoots:
    #!/usr/bin/env bash
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    VIRTUAL_KC="$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"

    if ! docker ps --format '{{{{.Names}}' | grep -q "{{KIND_CONTAINER}}"; then
        echo "Error: Gardener is not running. Start with: just cluster-worker gardener-up"
        exit 1
    fi

    if [ ! -f "$VIRTUAL_KC" ]; then
        echo "Error: Virtual garden kubeconfig not found at $VIRTUAL_KC"
        echo "Is the operator fully started? Try: just cluster-worker gardener-status"
        exit 1
    fi

    kubectl --kubeconfig "$VIRTUAL_KC" get shoots -A -w

# Follow cluster-worker logs
logs:
    kubectl logs -n fundament deploy/cluster-worker -f
