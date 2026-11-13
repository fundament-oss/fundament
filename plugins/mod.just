mod cert-manager
mod cloudnativepg
mod external-dns
mod openfsc
mod envoy-gateway 'gateway-api/envoy-gateway'
mod ceph-rook 'storage/ceph-rook'

cluster := "fundament-plugin"
registry := "plugin-registry.localhost"
registry_port := "5112"
export kube_context := "k3d-" + cluster

# Both binds are required; see docs/developer/fundament/k3d-block-devices.md.
# Off by default: they expose the host's devices.
storage_volumes := "--volume /dev:/dev@server:0 --volume /run/udev:/run/udev:ro@server:0"

# Recipes run in plugins/, which has no justfile: a bare `just <recipe>` resolves the root Justfile.
_default:
    @just --list plugins

# --- Cluster commands ---

# Create a local K3D cluster for plugin development with local registry
cluster-create:
    mkdir -p "$(dirname "${KUBECONFIG:-.}")"
    k3d cluster create --config=sandbox/k3d-config.yaml

# Create the cluster with the host's block devices bound in (storage plugin work)
cluster-create-storage:
    mkdir -p "$(dirname "${KUBECONFIG:-.}")"
    k3d cluster create --config=sandbox/k3d-config.yaml {{ storage_volumes }}

# uncordon is a no-op unless storage disks were ever attached.
# Start the cluster (creates if it doesn't exist), clearing any leftover cordon
cluster-start:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! k3d cluster list {{ cluster }} > /dev/null 2>&1; then
        just plugins cluster-create
    else
        # Port bindings are fixed at create time; see cluster-start in the root Justfile.
        if docker inspect k3d-{{ cluster }}-serverlb {{ registry }} \
            --format '{{{{ range $p, $b := .HostConfig.PortBindings }}{{{{ range $b }}[{{{{ .HostIp }}]{{{{ end }}{{{{ end }}' 2>/dev/null \
            | grep -qE '\[(0\.0\.0\.0|::)?\]'; then
            echo "This cluster publishes its ports on every network address. Recreate it to bind them to loopback: just plugins cluster-delete && just plugins cluster-create && just plugins deploy && just plugin-sandbox-kubeconfig && just plugins sandbox-catalog" >&2
            exit 1
        fi
        k3d cluster start {{ cluster }}
        # k3d writes the context only at create; merge adds a missing entry and refreshes the
        # API port, which changes on restart.
        mkdir -p "$(dirname "${KUBECONFIG:-.}")"
        k3d kubeconfig merge {{ cluster }} --kubeconfig-merge-default --kubeconfig-switch-context > /dev/null
    fi
    CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh uncordon

# drain fails on purpose while a volume is still mapped, because stopping then wedges
# the Docker host — so do not add a `-` prefix here.
# Stop the cluster, evicting Ceph consumers first
cluster-stop:
    CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh drain
    k3d cluster stop {{ cluster }}

# The relays sit on the cluster's docker network and block its removal, so they go first.
# Delete the local K3D cluster, its registry and the sandbox-catalog relays
cluster-delete:
    -@CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh drain
    @docker rm -f fun-catalog-relay fun-authn-relay > /dev/null 2>&1 || true
    k3d cluster delete {{ cluster }}
    @k3d registry delete {{ registry }} 2>/dev/null || true

# Loop-backed block devices for a storage plugin; run without arguments for usage
storage-disks *args:
    CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh {{ args }}

# --- Deployment commands ---

# Deploy plugin-controller with file watching (auto-rebuild on changes)
controller-dev *flags:
    skaffold dev -f sandbox/skaffold.yaml --default-repo=localhost:{{ registry_port }} --cleanup=false {{ flags }}

# Deploy plugin-controller (one-time)
deploy:
    kubectl --context {{ kube_context }} apply -f ../charts/fundament/crds/
    skaffold run -f sandbox/skaffold.yaml --default-repo=localhost:{{ registry_port }}

# Remove the deployment
undeploy:
    skaffold delete -f sandbox/skaffold.yaml

# --- Plugin commands ---

# To install a plugin, publish its definition with `publish` (below), then
# reference the published pluginVersion/definitionHash from a PluginInstallation.

# Delete a PluginInstallation CR (installation = metadata.name, e.g. system--cert-manager)
uninstall installation:
    kubectl --context {{ kube_context }} delete plugininstallation {{ installation }}

# Stream plugin-controller's logs, or one plugin's given its installation name
logs installation="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "{{ installation }}" ]; then
        exec kubectl --context {{ kube_context }} -n fundament logs -l app.kubernetes.io/component=plugin-controller -f
    fi
    # Addressed by the Deployment's constant name, so a hashed namespace works too.
    exec kubectl --context {{ kube_context }} -n "plugin-{{ installation }}" logs deployment/plugin -f

# Show all PluginInstallation CRs
status:
    kubectl --context {{ kube_context }} get plugininstallations


# Local dev only: the two k3d clusters are network-isolated (FUN-19, FUN-22). The
# catalog and authn-api are NodePorts in the management cluster; one relay container
# per service sits on both clusters' docker networks and publishes no host port, so
# nothing outside docker reaches it. The sandbox addresses each relay through a Service
# with an EndpointSlice (sandbox/relay-services.yaml), the fixed names in
# pluginController.catalogApiUrl and authnApiUrl of plugins/sandbox/values.yaml.
#
# Only the relay containers are multi-homed: multi-homing a sandbox node breaks its
# registry-mirror DNS (plugin-registry.localhost), which stops plugins pulling their
# images. Re-run this, with plugin-sandbox-kubeconfig, after recreating either cluster or
# restarting docker: the relays' addresses on the sandbox network are resolved fresh.
# Wire the sandbox plugin-controller to marketplace-catalog-api and authn-api in k3d-fundament
sandbox-catalog:
    #!/usr/bin/env bash
    set -euo pipefail
    sandbox_net=k3d-{{ cluster }}

    # 0. A leftover organization-api-sandbox-np Service holds the same fixed nodePort,
    #    and a leftover fun-orgapi-relay a host port; remove them.
    kubectl --context k3d-fundament delete service organization-api-sandbox-np \
        -n fundament --ignore-not-found
    docker rm -f fun-orgapi-relay >/dev/null 2>&1 || true

    # 1. Expose marketplace-catalog-api's Connect port and authn-api as NodePorts in the
    #    mgmt cluster. Paths are relative to plugins/ (this Justfile's dir).
    kubectl --context k3d-fundament apply -f sandbox/marketplace-catalog-api-nodeport.yaml
    kubectl --context k3d-fundament apply -f sandbox/authn-api-nodeport.yaml

    # 2. The Services the sandbox addresses the relays by.
    kubectl --context {{ kube_context }} get namespace fundament >/dev/null 2>&1 \
        || kubectl --context {{ kube_context }} create namespace fundament >/dev/null
    kubectl --context {{ kube_context }} apply -f sandbox/relay-services.yaml >/dev/null

    # 3. (Re)start each relay: sandbox network :8080 -> k3d-fundament-server-0:NODEPORT.
    #    Docker's DNS on k3d-fundament resolves the node per connection.
    relay() { # container nodeport service
        local name=$1 nodeport=$2 service=$3 ip
        docker rm -f "$name" >/dev/null 2>&1 || true
        docker run -d --name "$name" --network k3d-fundament --restart unless-stopped \
            alpine/socat "TCP-LISTEN:8080,fork,reuseaddr" "TCP:k3d-fundament-server-0:${nodeport}" >/dev/null
        docker network connect "$sandbox_net" "$name"
        ip=$(docker inspect "$name" --format "{{{{ (index .NetworkSettings.Networks \"$sandbox_net\").IPAddress }}")
        if [ -z "$ip" ]; then
            echo "Error: could not resolve $name's IP on the $sandbox_net network" >&2
            exit 1
        fi
        printf '{"apiVersion":"discovery.k8s.io/v1","kind":"EndpointSlice","metadata":{"name":"%s","namespace":"fundament","labels":{"kubernetes.io/service-name":"%s","endpointslice.kubernetes.io/managed-by":"just-plugins-sandbox-catalog"}},"addressType":"IPv4","ports":[{"name":"http","port":8080,"protocol":"TCP"}],"endpoints":[{"addresses":["%s"]}]}' \
            "$service" "$service" "$ip" | kubectl --context {{ kube_context }} apply -f - >/dev/null
        echo "- http://$service.fundament.svc:8080 in the sandbox relays to NodePort $nodeport"
    }
    relay fun-catalog-relay 30080 fundament-catalog-relay
    relay fun-authn-relay 30081 fundament-authn-relay
    echo "  (pluginController.catalogApiUrl and authnApiUrl in plugins/sandbox/values.yaml)"

# Publishes the pushed digest to marketplace-registry-api via `functl plugin publish`
# (FUN-20). Requires PLUGIN_REGISTRY (e.g. localhost:5112) and a functl session:
# `functl auth login` once, `functl org set <org-id>` for the publishing
# organization. The registry endpoint comes from functl's config
# (registry_url / FUNCTL_REGISTRY_URL; mise sets it for this repo).
# Extra flags pass through, e.g.:
#   just plugins publish gateway-api/envoy-gateway --create --submit
# (--create reserves a missing listing; --submit opens the review round.
# Versions are create-only — republishing means bumping metadata.version.)
# Build and push a plugin image, then publish its definition
publish name *flags:
    #!/usr/bin/env bash
    set -euo pipefail
    : "${PLUGIN_REGISTRY:?PLUGIN_REGISTRY is required (e.g. localhost:5112)}"
    # This Justfile runs recipes from plugins/, but the Docker build context, the
    # plugins/<name>/Dockerfile path, `go run ./plugins/...`, and publish's
    # plugins/<name>/definition.yaml read are all repo-root relative — run from root.
    cd "$(git rev-parse --show-toplevel)"
    tag=$(git describe --always --dirty)
    repo="${PLUGIN_REGISTRY}/{{ name }}-plugin"
    digest=$("{{ just_executable() }}" --justfile "{{ justfile() }}" plugins::_image-push "${repo}" "${tag}" "plugins/{{ name }}/Dockerfile")
    go run ./functl/cmd/functl plugin publish "plugins/{{ name }}/definition.yaml" --image "${repo}@${digest}" {{ flags }}

# Build and push an image from the repo root; prints the pushed manifest digest.
# --provenance/--sbom=false: skip buildkit attestations so the push is a plain
# single-platform manifest, not an OCI index. The attestation index digest
# changes every build and leaves the old (now-untagged) index to be GC'd by the
# registry, which strands the digest a PluginInstallation pinned. See FUN-19.
_image-push repo tag dockerfile *build_flags:
    #!/usr/bin/env bash
    set -euo pipefail
    cd "$(git rev-parse --show-toplevel)"
    docker build --provenance=false --sbom=false -t "{{ repo }}:{{ tag }}" -f "{{ dockerfile }}" {{ build_flags }} . >&2
    # Parse the manifest digest only from the `digest:` line — other sha256
    # tokens in push output are layer/config digests, not the pushed manifest.
    digest=$(docker push "{{ repo }}:{{ tag }}" | grep -oE 'digest: sha256:[a-f0-9]{64}' | grep -oE 'sha256:[a-f0-9]{64}' | head -1)
    [ -n "${digest}" ] || { echo "could not resolve pushed digest" >&2; exit 1; }
    echo "${digest}"

# Hot-reload a plugin in the sandbox; see docs/developer/plugins/testing-plugins-locally.md.
# Needs `just plugins deploy` and a functl login. Extra flags pass through (e.g. --no-cleanup).
# Runs from the invocation dir so relative flag paths (--config) resolve where you typed them.
[no-cd]
[positional-arguments]
dev name *flags:
    #!/usr/bin/env bash
    set -euo pipefail
    root="$(git rev-parse --show-toplevel)"
    exec go run "$root/functl/cmd/functl" plugin dev "$root/plugins/$1" "${@:2}"

# Remove a dev-mode PluginInstallation left behind by `dev`
[positional-arguments]
dev-down name:
    #!/usr/bin/env bash
    set -euo pipefail
    cd "$(git rev-parse --show-toplevel)"
    exec go run ./functl/cmd/functl plugin dev-down "plugins/$1"
