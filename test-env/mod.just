# fundament e2e environment with a real Gardener, in a VM.
#
# On the host, the verbs start a VM at a provider (TEST_ENV_PROVIDER, default qemu)
# and drive the environment in it over SSH: up, sync, redeploy, test, attach, run,
# collect, ssh, tunnel, trust, status, down, doctor. Inside the VM the same verbs
# act on the environment directly, and the gardener-* recipes manage the local
# Gardener.
# Docs: docs/developer/fundament/test-env.md

set shell := ["bash", "-uc"]

# The NixOS image writes this marker. The gardener-* recipes change the machine
# (/etc/hosts, loopback addresses, docker networks), so they run only in the VM;
# FUNDAMENT_TEST_ENV=1 overrides that for debugging.
in_vm := if path_exists("/etc/fundament-test-env") == "true" { "true" } else if env("FUNDAMENT_TEST_ENV", "") == "1" { "true" } else { "false" }
host := source_directory() / "host" / "cli.sh"
box := source_directory() / "box"

# The Gardener of fire, the fundament-poc partition: metal-stack v1.2.0 ships v1.147.5.
GARDENER_VERSION := "v1.147.5"

# Gardener clone, outside the repository: Gardener keeps its registry cache and backup
# buckets inside the clone, written as root by its containers.
GARDENER_DIR := env_var_or_default("GARDENER_DIR", cache_directory() / "fundament" / "gardener")

# KinD cluster name used by the operator path
KIND_CLUSTER := "gardener-local"
KIND_CONTAINER := KIND_CLUSTER + "-control-plane"
# kubeconfig context gardener-connect adds for the virtual garden
GARDEN_CONTEXT := "kind-gardener-local-garden"

_default:
    @just --list test-env

# Fresh VM with the environment up (host); bring the environment up (VM)
up:
    @if {{in_vm}}; then "{{box}}/up.sh"; else "{{host}}" up; fi

# Run the e2e scenarios against the real Gardener; the bundle lands in test-env/bundles/
test tags="":
    @if {{in_vm}}; then "{{box}}/test.sh" {{quote(tags)}}; else "{{host}}" test {{quote(tags)}}; fi

# Pick up an up or test that goes on in the VM after a disconnect
attach: _require-host
    @"{{host}}" attach

# Bundle the e2e report and the logs of fundament, Gardener and the shoots
collect:
    @if {{in_vm}}; then "{{box}}/collect.sh"; else "{{host}}" collect; fi

# Redeploy fundament without resetting the databases, so shoots survive (syncs first from the host)
redeploy:
    @if {{in_vm}}; then "{{box}}/redeploy.sh"; else "{{host}}" redeploy; fi

# Copy the working tree into the VM
sync: _require-host
    @"{{host}}" sync

# Fresh VM, full run of one commit. The VM stays when the run is red.
run sha: _require-host
    @"{{host}}" run {{quote(sha)}}

# Shell in the VM's checkout, or run a command there
[positional-arguments]
ssh *cmd: _require-host
    @"{{host}}" ssh "$@"

# Forward the console to https://console.fundament.localhost:8443
tunnel: _require-host
    @"{{host}}" tunnel

# Trust the VM's CA on this host, for a VM that did not get your mkcert CA
trust: _require-host
    @"{{host}}" trust

# Provider state of the VM
status: _require-host
    @"{{host}}" status

# Destroy the VM and its disks
down: _require-host
    @"{{host}}" down

# Check the host requirements
doctor: _require-host
    @"{{host}}" doctor

_require-vm:
    @{{in_vm}} || { echo "This recipe changes the machine and runs only in the test-env VM: just test-env ssh" >&2; exit 1; }

_require-host:
    @! {{in_vm}} || { echo "This recipe runs on the host, not in the VM." >&2; exit 1; }

# Show pinned Gardener version
versions:
    @echo "Gardener: {{GARDENER_VERSION}} (operator path)"

# Publish a first-party plugin (path under plugins/) and approve it, so it can be installed on a shoot
[positional-arguments]
plugin-publish path *flags: _require-vm
    @"{{box}}/plugin-publish.sh" "$@"

# Add a kube context shoot-<cluster name> with an admin kubeconfig for the cluster's shoot
shoot-context cluster-id: _require-vm
    #!/usr/bin/env bash
    set -euo pipefail
    shoot=$(kubectl --context {{GARDEN_CONTEXT}} get shoots -A -l fundament.io/cluster-id={{quote(cluster-id)}} \
        -o jsonpath='{range .items[*]}{.metadata.namespace} {.metadata.name} {.metadata.annotations.fundament\.io/cluster-name}{"\n"}{end}')
    [ -n "$shoot" ] || { echo "no shoot carries the cluster-id label {{cluster-id}}" >&2; exit 1; }
    read -r ns name cluster <<< "$shoot"
    dir=$(mktemp -d)
    kubectl --context {{GARDEN_CONTEXT}} create --raw "/apis/core.gardener.cloud/v1beta1/namespaces/$ns/shoots/$name/adminkubeconfig" \
        -f <(echo '{"apiVersion":"authentication.gardener.cloud/v1alpha1","kind":"AdminKubeconfigRequest","spec":{"expirationSeconds":86400}}') \
        | jq -r '.status.kubeconfig' | base64 -d > "$dir/kubeconfig"
    KUBECONFIG="$dir/kubeconfig:$HOME/.kube/config" kubectl config view --flatten > "$dir/merged"
    ctx=$(kubectl --kubeconfig "$dir/kubeconfig" config current-context)
    # A context of that name from an earlier up would make the rename fail.
    kubectl --kubeconfig "$dir/merged" config delete-context "shoot-$cluster" >/dev/null 2>&1 || true
    kubectl --kubeconfig "$dir/merged" config rename-context "$ctx" "shoot-$cluster" >/dev/null
    install -m 0600 "$dir/merged" "$HOME/.kube/config"
    rm -rf "$dir"
    echo "kube context shoot-$cluster ($ns/$name)"

# Start local Gardener via gardener-operator (~15 min first time)
gardener-start: _require-vm
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"

    # The clone is outside the repository, where mise's shims find no tool versions;
    # put the repository's tools (Go for Gardener's build) on PATH before leaving it.
    if command -v mise > /dev/null; then eval "$(mise env -s bash)"; fi

    # The last steps connect Gardener to the platform cluster; fail now, not after the build.
    if ! docker inspect -f '{{{{.State.Running}}' k3d-fundament-server-0 2>/dev/null | grep -q true; then
        echo "Error: the k3d-fundament cluster is not running. Start it first: just cluster-start"
        exit 1
    fi

    # A run that stopped part-way can leave a Garden without its kubeconfig or Seed, so
    # only a registered Seed counts as running; otherwise continue the installation,
    # whose make targets can be re-run.
    KIND_EXISTS=false
    if kind get clusters 2>/dev/null | grep -q "{{KIND_CLUSTER}}"; then
        KIND_EXISTS=true
    fi
    VIRTUAL_KC="$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"
    if [ "$KIND_EXISTS" = true ] && [ -f "$VIRTUAL_KC" ] \
        && kubectl --kubeconfig "$VIRTUAL_KC" --request-timeout=10s get seed local \
            -o jsonpath='{.status.conditions[?(@.type=="GardenletReady")].status}' 2>/dev/null | grep -qx True; then
        echo "Gardener already running ({{KIND_CLUSTER}})"
        echo "Check status: just test-env gardener-status"
        (cd "{{justfile_directory()}}" && GARDENER_DIR="$GARDENER_DIR" just test-env gardener-connect)
        (cd "{{justfile_directory()}}" && GARDENER_DIR="$GARDENER_DIR" just test-env gardener-secret)
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
            echo "Run 'just test-env gardener-delete' and re-run to update"
            exit 1
        fi
        cd - > /dev/null
    fi

    # The test-env VM keeps Gardener's pull-through registry caches and its own
    # built images on a disk that outlives the VM; the backup buckets stay here.
    if [ -n "${GARDENER_REGISTRY_CACHE:-}" ] && [ ! -e "$GARDENER_DIR/dev/local-registry" ]; then
        mkdir -p "$GARDENER_DIR/dev" "$GARDENER_REGISTRY_CACHE"
        ln -s "$GARDENER_REGISTRY_CACHE" "$GARDENER_DIR/dev/local-registry"
    fi

    echo "Starting Gardener via operator (this takes 10-15 minutes on first run)..."
    cd "$GARDENER_DIR"

    # Build Gardener with the Go its go.mod names. Built with a newer Go, the
    # gardener-apiserver serves an OpenAPI schema the virtual garden cannot convert,
    # and every kubectl apply against it fails with a 503.
    export GOTOOLCHAIN="go$(sed -n 's/^go //p' go.mod)"

    # Pre-install helm into Gardener's tools directory if not present.
    # Gardener's Makefile downloads helm via a script that can fail on checksum
    # verification for certain versions. Using a direct binary download is more reliable.
    ARCH="$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')"
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
    # mise.toml exports the fundament registry, which would override the ?=
    # default in Gardener's Makefile; the kind nodes cannot pull from it, so
    # the operator/gardenlet pods would wedge in ImagePullBackOff. The same
    # applies when running Gardener make targets by hand: unset it first.
    unset SKAFFOLD_DEFAULT_REPO
    if [ "$KIND_EXISTS" = false ]; then
        make kind-single-node-up
    fi
    make gardener-up

    echo ""
    (cd "{{justfile_directory()}}" && GARDENER_DIR="$GARDENER_DIR" just test-env gardener-connect)
    (cd "{{justfile_directory()}}" && GARDENER_DIR="$GARDENER_DIR" just test-env gardener-secret)
    echo "Gardener started! (operator path)"

# Delete local Gardener completely: kind cluster, registry/DNS containers, clone and contexts (also on a host that runs one)
gardener-delete:
    #!/usr/bin/env bash
    set -e
    # Gardener before v1.142 named the cluster gardener-operator-local.
    for cluster in "{{KIND_CLUSTER}}" gardener-operator-local; do
        if kind get clusters 2>/dev/null | grep -qx "$cluster"; then
            echo "Deleting $cluster cluster..."
            kind delete cluster --name "$cluster"
        fi
    done
    GARDENER_DIR="{{GARDENER_DIR}}"
    dirs=("${GARDENER_DIR/#\~/$HOME}")
    # Older versions cloned Gardener into .dev/gardener.
    OLD_DIR="{{justfile_directory()}}/.dev/gardener"
    [ -d "$OLD_DIR" ] && dirs+=("$OLD_DIR")
    for dir in "${dirs[@]}"; do
        (cd "{{justfile_directory()}}" && GARDENER_DIR="$dir" just test-env _gardener-infra-down)
    done
    TARGET_KC="${KUBECONFIG:-}"
    TARGET_KC="${TARGET_KC%%:*}"
    TARGET_KC="${TARGET_KC:-$HOME/.kube/config}"
    for name in "{{GARDEN_CONTEXT}}" "kind-{{KIND_CLUSTER}}" kind-gardener-operator-local; do
        kubectl config --kubeconfig "$TARGET_KC" delete-context "$name" >/dev/null 2>&1 || true
        kubectl config --kubeconfig "$TARGET_KC" delete-cluster "$name" >/dev/null 2>&1 || true
        kubectl config --kubeconfig "$TARGET_KC" delete-user "$name" >/dev/null 2>&1 || true
    done
    for dir in "${dirs[@]}"; do
        [ -d "$dir" ] || continue
        # Gardener's containers write the registry cache and backup buckets as root;
        # a container removes them without sudo.
        docker run --rm -v "$(dirname "$dir"):/parent" alpine rm -rf "/parent/$(basename "$dir")"
        echo "Removed $dir"
    done

# Remove the registry and DNS containers kind-up starts with Docker Compose. They
# outlive the kind cluster, so this also runs when the cluster is already gone.
_gardener-infra-down:
    #!/usr/bin/env bash
    set -e
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    INFRA_DIR="$GARDENER_DIR/dev-setup/infra"
    if [ -f "$INFRA_DIR/docker-compose.yaml" ]; then
        docker compose -f "$INFRA_DIR/docker-compose.yaml" down --volumes
    elif docker ps -aq --filter "label=com.docker.compose.project.working_dir=$INFRA_DIR" | grep -q .; then
        # The clone is gone; its containers are recognised by their compose directory.
        docker compose -p infra down --volumes
    fi

# Check Gardener status
gardener-status: _require-vm
    #!/usr/bin/env bash
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    RUNTIME_KC="$GARDENER_DIR/dev-setup/kubeconfigs/runtime/kubeconfig"
    VIRTUAL_KC="$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"

    echo "=== Local Gardener Status (operator) ==="
    echo ""

    if ! kind get clusters 2>/dev/null | grep -q "{{KIND_CLUSTER}}"; then
        echo "Status: NOT RUNNING"
        echo "Start with: just test-env gardener-start"
        exit 1
    fi

    echo "Status: RUNNING"
    K3D_IP=$(docker inspect "{{KIND_CONTAINER}}" --format '{{{{range $net, $conf := .NetworkSettings.Networks}}{{{{if eq $net "k3d-fundament"}}{{{{$conf.IPAddress}}{{{{end}}{{{{end}}' 2>/dev/null || echo "not connected")
    echo "API Server: https://$K3D_IP:6443 (k3d-fundament network)"
    echo ""

    echo "=== Garden ==="
    # The on-disk runtime kubeconfig goes stale on restart (kind API port + CA),
    # so pull a live one straight from kind to query the runtime cluster.
    RUNTIME_KC_LIVE="$(mktemp)"
    if kind get kubeconfig --name "{{KIND_CLUSTER}}" > "$RUNTIME_KC_LIVE" 2>/dev/null; then
        kubectl --kubeconfig "$RUNTIME_KC_LIVE" --request-timeout=10s get garden -n garden 2>/dev/null || echo "(garden not found)"
        # Surface any non-healthy conditions (e.g. provider-local extension down).
        kubectl --kubeconfig "$RUNTIME_KC_LIVE" --request-timeout=10s get garden -n garden \
            -o jsonpath='{range .items[0].status.conditions[?(@.status!="True")]}  ! {.type}: {.reason} - {.message}{"\n"}{end}' 2>/dev/null
    else
        echo "(could not get runtime kubeconfig via kind)"
    fi
    rm -f "$RUNTIME_KC_LIVE"
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
        echo "k3d network: NOT connected (run: just test-env gardener-connect)"
    fi
    if ip -o addr show lo 2>/dev/null | grep -q "172.18.255.53"; then
        echo "Gardener DNS loopback (172.18.255.53): present"
    else
        echo "Gardener DNS loopback (172.18.255.53): MISSING"
    fi

# Connect Docker networks so k3d can reach Gardener
gardener-connect: _require-vm
    #!/usr/bin/env bash
    set -e

    KIND_CONTAINER="{{KIND_CONTAINER}}"
    K3D_NETWORK="k3d-fundament"
    KIND_NETWORK="kind"

    if ! docker ps --format '{{{{.Names}}' | grep -q "$KIND_CONTAINER"; then
        echo "Error: Gardener is not running. Start with: just test-env gardener-start"
        exit 1
    fi

    if ! docker network ls --format '{{{{.Name}}' | grep -q "^${K3D_NETWORK}$"; then
        echo "Error: k3d network not found. Is the k3d cluster running?"
        exit 1
    fi

    # Refresh the k3d-fundament kubeconfig and make it the active context.
    # Docker reassigns the serverlb's published port on every restart, leaving a
    # stale port in the kubeconfig, and k3d writes the server as 0.0.0.0 which is
    # an unreliable connect target on macOS. Write to the kubeconfig kubectl will
    # actually read: the first entry of $KUBECONFIG (the repo .envrc/direnv points
    # this at deploy/k3d/kubeconfig.yaml), else ~/.kube/config. Restore the
    # entry/certs/context with a one-shot merge, then pin the server to 127.0.0.1
    # with the *live* port read straight from docker (ground truth, not k3d's
    # cached value). Right after a Docker restart the serverlb is still
    # stabilizing, so probe /livez -- with a per-attempt timeout so it can't block
    # forever -- until it answers.
    TARGET_KC="${KUBECONFIG:-}"
    TARGET_KC="${TARGET_KC%%:*}"
    TARGET_KC="${TARGET_KC:-$HOME/.kube/config}"
    k3d kubeconfig merge fundament --output "$TARGET_KC" --kubeconfig-switch-context >/dev/null
    waited=false
    for attempt in $(seq 1 30); do
        LB_PORT=$(docker port k3d-fundament-serverlb 6443 2>/dev/null | head -1 | sed 's/.*://')
        if [ -n "$LB_PORT" ]; then
            kubectl config --kubeconfig "$TARGET_KC" set-cluster k3d-fundament \
                --server="https://127.0.0.1:${LB_PORT}" >/dev/null 2>&1 || true
            if kubectl --context k3d-fundament --request-timeout=3s get --raw /livez >/dev/null 2>&1; then
                break
            fi
        fi
        if [ "$attempt" -eq 1 ]; then
            printf '- Waiting for k3d-fundament API to stabilize'
            waited=true
        fi
        printf '.'
        if [ "$attempt" -eq 30 ]; then
            printf '\n'
            echo "Error: k3d-fundament API not reachable after ~2min."
            echo "  Check it:    just test-env gardener-status"
            echo "  Restart it:  k3d cluster stop fundament && k3d cluster start fundament"
            exit 1
        fi
        sleep 2
    done
    if [ "$waited" = "true" ]; then printf '\n'; fi
    echo "- Refreshed k3d-fundament kubeconfig (127.0.0.1:${LB_PORT})"

    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"

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
        # It drains for about a minute before the kubelet starts it again.
        sleep 5
        for i in $(seq 1 90); do
            docker exec "$KIND_CONTAINER" curl -skf -o /dev/null --max-time 2 https://127.0.0.1:6443/readyz && break
            [ "$i" -lt 90 ] || { echo "Error: the kind API server did not come back within 3 minutes"; exit 1; }
            sleep 2
        done
        echo "- API server cert updated with SANs: $KIND_IP, $K3D_IP"
    else
        echo "- k3d IP ($K3D_IP) already in API server cert SANs"
    fi

    API_IP="$K3D_IP"
    echo ""
    echo "Gardener API: https://$API_IP:6443"

    # The seed's and the virtual garden's Istio ingresses are LoadBalancer Services.
    # cloud-controller-manager-local publishes each on a host loopback IP, which the
    # k3d pods reach through the Docker host.
    ingress_ip() { # namespace pattern
        docker exec "$KIND_CONTAINER" kubectl get svc -A --field-selector metadata.name=istio-ingressgateway \
            -o jsonpath='{range .items[*]}{.metadata.namespace} {.status.loadBalancer.ingress[0].ip}{"\n"}{end}' \
            | awk -v ns="$1" '$1 ~ ns && $2 != "" {print $2; exit}'
    }
    SEED_INGRESS_IP=$(ingress_ip '^istio-ingress$')
    GARDEN_INGRESS_IP=$(ingress_ip '^virtual-garden-istio-ingress$')
    if [ -z "$SEED_INGRESS_IP" ] || [ -z "$GARDEN_INGRESS_IP" ]; then
        echo "Error: no load balancer IP on the Istio ingresses (seed: '$SEED_INGRESS_IP', virtual garden: '$GARDEN_INGRESS_IP')"
        echo "  Check them: docker exec $KIND_CONTAINER kubectl get svc -A --field-selector metadata.name=istio-ingressgateway"
        exit 1
    fi

    # Write discovered IPs to generated values file (used by local-gardener Skaffold profile)
    GARDENER_VALUES="{{source_directory()}}/../charts/fundament/values-gardener-local.yaml"
    printf '# Generated by: just test-env gardener-connect\ngardener:\n  apiServerIP: "%s"\nshootProxy:\n  seedIngressIP: "%s"\n  gardenIngressIP: "%s"\n' \
        "$API_IP" "$SEED_INGRESS_IP" "$GARDEN_INGRESS_IP" > "$GARDENER_VALUES"
    echo "- Wrote gardener values: apiServerIP=$API_IP, shootProxy.seedIngressIP=$SEED_INGRESS_IP, shootProxy.gardenIngressIP=$GARDEN_INGRESS_IP"

    # Annotate CoreDNS so Reloader restarts it when coredns-custom ConfigMap changes
    if kubectl --context k3d-fundament -n kube-system get deploy coredns &>/dev/null; then
        kubectl --context k3d-fundament -n kube-system annotate deployment coredns \
            reloader.stakater.com/auto=true --overwrite 2>/dev/null
        echo "- Annotated CoreDNS for Reloader auto-restart"
    fi

    # Add Gardener's two clusters to the same kubeconfig as k3d-fundament, under
    # fixed names. The kind cluster's port and certificates change when it is
    # recreated, so read them fresh every time.
    add_context() {
        local name="$1" dir path
        dir=$(mktemp -d)
        cat > "$dir/full"
        # Gardener's virtual garden kubeconfig also holds the kind cluster; keep the current context.
        kubectl --kubeconfig "$dir/full" config view --minify --raw > "$dir/kubeconfig"
        for path in clusters[0].cluster.certificate-authority-data users[0].user.client-certificate-data users[0].user.client-key-data; do
            kubectl --kubeconfig "$dir/kubeconfig" config view --raw -o jsonpath="{.$path}" | base64 -d > "$dir/${path##*.}"
        done
        kubectl config --kubeconfig "$TARGET_KC" set-cluster "$name" --embed-certs --certificate-authority "$dir/certificate-authority-data" \
            --server "$(kubectl --kubeconfig "$dir/kubeconfig" config view -o jsonpath='{.clusters[0].cluster.server}')" >/dev/null
        kubectl config --kubeconfig "$TARGET_KC" set-credentials "$name" --embed-certs \
            --client-certificate "$dir/client-certificate-data" --client-key "$dir/client-key-data" >/dev/null
        kubectl config --kubeconfig "$TARGET_KC" set-context "$name" --cluster "$name" --user "$name" >/dev/null
        rm -rf "$dir"
        echo "- Added kubeconfig context $name"
    }
    add_context "{{GARDEN_CONTEXT}}" < "$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"
    kind get kubeconfig --name "{{KIND_CLUSTER}}" | add_context "kind-{{KIND_CLUSTER}}"

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
    # via the garden-proxy's CoreDNS entry + socat -> the virtual garden Istio load balancer.
    if [ ! -f "$VIRTUAL_KC" ]; then
        echo "Error: Virtual garden kubeconfig not found at $VIRTUAL_KC"
        echo "Is Gardener fully started? Run: just test-env gardener-start"
        exit 1
    fi

    echo "Using virtual garden kubeconfig"

    # Ensure fundament namespace exists. Pin --context so the secret lands on the
    # fundament k3d cluster regardless of the active context (Docker can reset it
    # to docker-desktop on restart).
    kubectl --context k3d-fundament create namespace fundament --dry-run=client -o yaml | kubectl --context k3d-fundament apply -f -

    echo "Creating gardener-kubeconfig secret (server: api.virtual-garden.local.gardener.cloud)..."
    kubectl --context k3d-fundament create secret generic gardener-kubeconfig \
        --from-file=kubeconfig="$VIRTUAL_KC" \
        -n fundament --dry-run=client -o yaml | kubectl --context k3d-fundament apply -f -
    echo "Secret created: gardener-kubeconfig"

# Watch shoots in Gardener
shoots: _require-vm
    #!/usr/bin/env bash
    GARDENER_DIR="{{GARDENER_DIR}}"
    GARDENER_DIR="${GARDENER_DIR/#\~/$HOME}"
    VIRTUAL_KC="$GARDENER_DIR/dev-setup/kubeconfigs/virtual-garden/kubeconfig"

    if ! docker ps --format '{{{{.Names}}' | grep -q "{{KIND_CONTAINER}}"; then
        echo "Error: Gardener is not running. Start with: just test-env gardener-start"
        exit 1
    fi

    if [ ! -f "$VIRTUAL_KC" ]; then
        echo "Error: Virtual garden kubeconfig not found at $VIRTUAL_KC"
        echo "Is the operator fully started? Try: just test-env gardener-status"
        exit 1
    fi

    kubectl --kubeconfig "$VIRTUAL_KC" get shoots -A -w

# Follow cluster-worker logs
logs: _require-vm
    kubectl --context k3d-fundament logs -n fundament deploy/cluster-worker -f
