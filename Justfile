mod terraform-provider
mod e2e
mod cluster-worker
mod deploy-remote

_default:
    @just --list

# Format all code and text in this repo
fmt:
    @find . -type f \( -name "*.md" -o -name "*.adoc" -o -name "*.drawio.svg" \) -exec perl -pi -e 's/enterprise/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/g' {} +
    go fmt ./...
    # TODO md fmt

# --- Cluster commands ---

# Ensure the k3d docker network exists with a fixed subnet: off 172.18.0.0/16 (which
# Gardener's local kind cluster reserves — auto-allocation would grab it) and on
# 172.19.0.0/16 so the ingress-nginx externalIP (172.19.0.2, see
# deploy/k3d/resources/ingress-nginx.yaml) stays valid. Fails loudly on subnet conflict.
_ensure-k3d-network:
    @docker network inspect k3d-fundament > /dev/null 2>&1 || docker network create --subnet=172.19.0.0/16 k3d-fundament > /dev/null

# Create a local k3d cluster for development with local registry
cluster-create:
    just _ensure-k3d-network
    k3d cluster create --config=deploy/k3d/config.yaml
    just setup-certs

# Start the cluster (creates if it doesn't exist)
cluster-start:
    @just _ensure-k3d-network
    @k3d cluster list fundament > /dev/null 2>&1 && k3d cluster start fundament || just cluster-create

# Stop the cluster without deleting it
cluster-stop:
    k3d cluster stop fundament

# Delete the local k3d cluster and registry
cluster-delete:
    k3d cluster delete fundament
    @k3d registry delete registry.localhost 2>/dev/null || true
    @docker network rm k3d-fundament 2>/dev/null || true

# Install mkcert CA as a cert-manager ClusterIssuer for local HTTPS
setup-certs:
    #!/usr/bin/env bash
    set -e
    which mkcert > /dev/null 2>&1 || { echo "mkcert not installed. Run: mise install"; exit 1; }
    which certutil > /dev/null 2>&1 || { echo "certutil not installed. See docs/development-setup.md for installation instructions."; exit 1; }
    TRUST_STORES=system,nss mkcert -install
    echo "Waiting for cert-manager to become available..."
    deadline=$(( $(date +%s) + 120 ))
    until kubectl get ns cert-manager > /dev/null 2>&1; do
        [ "$(date +%s)" -ge "$deadline" ] && { echo "Timed out waiting for cert-manager namespace"; exit 1; }
        sleep 5
    done
    kubectl wait --for=condition=Available deployment/cert-manager deployment/cert-manager-webhook -n cert-manager --timeout=300s
    echo "Waiting for cert-manager webhook to be ready..."
    for i in $(seq 1 12); do
        helm upgrade --install mkcert-setup charts/mkcert-setup \
            --namespace cert-manager \
            --set-file ca.cert="$(mkcert -CAROOT)/rootCA.pem" \
            --set-file ca.key="$(mkcert -CAROOT)/rootCA-key.pem" && break
        [ "$i" -eq 12 ] && { echo "cert-manager webhook did not become ready in time"; exit 1; }
        echo "Webhook not ready yet, retrying in 5s... ($i/12)"
        sleep 5
    done

# --- Deployment commands ---

# Update helm dependencies
helm-deps:
    helm dependency update deploy/charts/ingress-nginx
    helm dependency update deploy/charts/db
    helm dependency update deploy/charts/fundament

# Deploy to local k3d cluster (development mode, keeps resources on exit)
dev *flags:
    SKAFFOLD_DEFAULT_REPO="localhost:5111" \
    skaffold dev --kube-context k3d-fundament --profile env-local --cleanup=false {{ flags }}

# Deploy to local k3d cluster with hot-reload
dev-hotreload:
    @just dev --profile hotreload

# Deploy with hot-reload and kube-api-proxy debugger (dlv on localhost:2345)
dev-debug:
    #!/usr/bin/env bash
    set -e
    # Wait for the deployment to be ready, then keep port-forwarding 2345 in the background.
    # Restart the forward if the pod restarts (air rebuild).
    (while true; do
        kubectl wait --for=condition=available --timeout=120s deployment/kube-api-proxy -n fundament >/dev/null 2>&1 || true
        kubectl port-forward -n fundament deployment/kube-api-proxy 2345:2345 >/dev/null 2>&1 || true
        sleep 2
    done) &
    PF_PID=$!
    trap "kill $PF_PID 2>/dev/null" EXIT
    just dev --profile hotreload --profile debug

# Deploy to an environment (e.g. local, production)
deploy env:
    skaffold run --profile env-{{ env }}

# Create/update the Secret plugin-proxy uses to reach the k3d-fundament-plugin
# sandbox cluster. Bridges the two k3d Docker networks (each cluster runs on
# its own by default) by connecting the plugin cluster's serverlb container to
# the main cluster's network, then rewrites the kubeconfig's server URL to
# that in-network IP so plugin-proxy pods can dial it. Idempotent.
plugin-sandbox-kubeconfig:
    #!/usr/bin/env bash
    set -euo pipefail
    KUBECONFIG_TMP=$(mktemp)
    trap 'rm -f "$KUBECONFIG_TMP"' EXIT
    k3d kubeconfig get fundament-plugin > "$KUBECONFIG_TMP"

    # Attach the plugin cluster's serverlb to the main cluster's Docker
    # network (no-op if already connected) so its container IP is reachable
    # from plugin-proxy pods.
    docker network connect k3d-fundament k3d-fundament-plugin-serverlb 2>/dev/null || true

    # Get the IP the plugin serverlb now holds on k3d-fundament. The k3s API
    # server listens on port 6443 inside the container regardless of any host
    # port mapping.
    ip=$(docker inspect k3d-fundament-plugin-serverlb \
        --format '{{{{ (index .NetworkSettings.Networks "k3d-fundament").IPAddress }}')
    if [ -z "$ip" ]; then
        echo "Error: could not resolve k3d-fundament-plugin-serverlb IP on k3d-fundament network"
        exit 1
    fi
    echo "- plugin sandbox reachable at https://${ip}:6443 from k3d-fundament pods"

    # Replace the host-side server URL (https://0.0.0.0:PORT) with the
    # in-network address (https://IP:6443).
    sed -i.bak -E "s|server: https://0\\.0\\.0\\.0:[0-9]+|server: https://${ip}:6443|" "$KUBECONFIG_TMP"
    rm -f "${KUBECONFIG_TMP}.bak"

    # Skip TLS verification — k3d's server cert doesn't cover the in-network
    # IP. Local-dev shortcut; production uses proper cluster CAs via Gardener.
    # Delete any certificate-authority-data line and add insecure-skip-tls-verify
    # right after the server line (a stable anchor regardless of yaml indent).
    sed -i.bak -E '/^[[:space:]]*certificate-authority-data:/d' "$KUBECONFIG_TMP"
    sed -i.bak -E 's|^([[:space:]]*)server: (.*)|\1server: \2\n\1insecure-skip-tls-verify: true|' "$KUBECONFIG_TMP"
    rm -f "${KUBECONFIG_TMP}.bak"
    kubectl --context k3d-fundament -n fundament create secret generic plugin-sandbox-kubeconfig \
        --from-file=kubeconfig="$KUBECONFIG_TMP" \
        --dry-run=client -o yaml | \
        kubectl --context k3d-fundament apply -f -

    # The FUN-17 "user half" runs a SubjectAccessReview on the sandbox for the
    # caller's per-user SA (system:serviceaccount:fundament-system:fundament-<id>).
    # On real clusters cluster-worker provisions those SAs and their RBAC, but
    # it is disabled in the sandbox — so grant the whole fundament-system SA
    # group cluster-admin here, otherwise every plugin request 403s. Sandbox
    # only; production relies on Gardener + per-user RBAC.
    kubectl --context k3d-fundament-plugin create clusterrolebinding fundament-sandbox-user-access \
        --clusterrole=cluster-admin \
        --group=system:serviceaccounts:fundament-system \
        --dry-run=client -o yaml | \
        kubectl --context k3d-fundament-plugin apply -f -

    echo "plugin-sandbox-kubeconfig Secret updated. Restart plugin-proxy:"
    echo "  kubectl --context k3d-fundament -n fundament rollout restart deployment/plugin-proxy"

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
    PASSWORD=$(kubectl get secret -n fundament fundament-db-fun-operator -o jsonpath='{.data.password}' |  {{ if os() == "macos" { "base64 -D" } else { "base64 -d" } }})
    kubectl exec -it -n fundament fundament-db-1 -- env PGPASSWORD="$PASSWORD" psql -h localhost -U fun_operator -d fundament

generate:
    cd db && trek generate --stdout
    go generate -x ./...
    cd console-frontend && buf generate
    cd console-frontend && bunx openapi-ts
    cd e2e && buf generate
    cd dcim-frontend && buf generate
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
[positional-arguments]
functl *args:
    #!/usr/bin/env bash
    exec go run ./functl/cmd/functl "$@"

# Build+push a plugin image, then publish its definition (with the pushed digest)
# to organization-api. Requires PLUGIN_REGISTRY (e.g. localhost:5112) and
# FUNDAMENT_ORG_API_URL; FUNDAMENT_TOKEN for the authenticated endpoint.
plugin-publish name:
    #!/usr/bin/env bash
    set -euo pipefail
    : "${PLUGIN_REGISTRY:?PLUGIN_REGISTRY is required (e.g. localhost:5112)}"
    : "${FUNDAMENT_ORG_API_URL:?FUNDAMENT_ORG_API_URL is required}"
    tag=$(git describe --always --dirty)
    repo="${PLUGIN_REGISTRY}/{{ name }}-plugin"
    docker build -t "${repo}:${tag}" -f "plugins/{{ name }}/Dockerfile" .
    digest=$(docker push "${repo}:${tag}" | grep -oE 'sha256:[a-f0-9]{64}' | head -1)
    [ -n "${digest}" ] || { echo "could not resolve pushed digest"; exit 1; }
    go run ./plugins/cmd/plugin-publish --plugin '{{ name }}' --image "${repo}@${digest}"
