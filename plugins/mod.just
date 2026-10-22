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
    k3d cluster create --config=sandbox/k3d-config.yaml

# Create the cluster with the host's block devices bound in (storage plugin work)
cluster-create-storage:
    k3d cluster create --config=sandbox/k3d-config.yaml {{ storage_volumes }}

# uncordon is a no-op unless storage disks were ever attached.
# Start the cluster (creates if it doesn't exist), clearing any leftover cordon
cluster-start:
    @k3d cluster list {{ cluster }} > /dev/null 2>&1 && k3d cluster start {{ cluster }} || just plugins cluster-create
    CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh uncordon

# drain fails on purpose while a volume is still mapped, because stopping then wedges
# the Docker host — so do not add a `-` prefix here.
# Stop the cluster, evicting Ceph consumers first
cluster-stop:
    CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh drain
    k3d cluster stop {{ cluster }}

# Delete the local K3D cluster, its registry and the sandbox-catalog relays
cluster-delete:
    -@CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh drain
    k3d cluster delete {{ cluster }}
    @k3d registry delete {{ registry }} 2>/dev/null || true
    @docker rm -f fun-catalog-relay fun-authn-relay > /dev/null 2>&1 || true

# Loop-backed block devices for a storage plugin; run without arguments for usage
storage-disks *args:
    CLUSTER={{ cluster }} ../deploy/k3d/storage-disks.sh {{ args }}

# --- Deployment commands ---

# Deploy plugin-controller with file watching (auto-rebuild on changes)
controller-dev *flags:
    skaffold dev -f sandbox/skaffold.yaml --default-repo=localhost:{{ registry_port }} --cleanup=false {{ flags }}

# Deploy plugin-controller (one-time)
deploy:
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


# Local dev only: the two k3d clusters are network-isolated (FUN-19, FUN-22). We
# expose the catalog and authn-api as NodePorts in the management cluster and run a
# small relay container per service on the management cluster's docker network
# that publishes a host port; the sandbox controller reaches them via
# host.k3d.internal (pluginController.catalogApiUrl and authnApiUrl in
# plugins/sandbox/values.yaml are http://host.k3d.internal:38080 and :38081).
#
# We deliberately do NOT bridge the sandbox k3d nodes onto the management network:
# multi-homing a node breaks its registry-mirror DNS (plugin-registry.localhost),
# which stops plugins pulling their images. Only the relay container is bridged.
# Re-run this after recreating either cluster (the node IP is resolved fresh).
# Wire the sandbox plugin-controller to marketplace-catalog-api in k3d-fundament
sandbox-catalog:
    #!/usr/bin/env bash
    set -euo pipefail
    HOSTPORT=38080
    NODEPORT=30080
    RELAY=fun-catalog-relay
    AUTHN_HOSTPORT=38081
    AUTHN_NODEPORT=30081
    AUTHN_RELAY=fun-authn-relay

    # 0. Clean up what the pre-catalog recipe left behind: the old Service holds
    #    the same fixed nodePort, and the old relay still binds HOSTPORT under a
    #    name the rm below no longer matches. Both make this recipe fail.
    kubectl --context k3d-fundament delete service organization-api-sandbox-np \
        -n fundament --ignore-not-found
    docker rm -f fun-orgapi-relay >/dev/null 2>&1 || true

    # 1. Expose marketplace-catalog-api's Connect port as a NodePort in the mgmt cluster.
    #    Path is relative to plugins/ (this Justfile's dir), like the skaffold recipes.
    kubectl --context k3d-fundament apply -f sandbox/marketplace-catalog-api-nodeport.yaml
    kubectl --context k3d-fundament apply -f sandbox/authn-api-nodeport.yaml

    # 2. Resolve a management-cluster node IP on its docker network.
    node_ip=$(docker inspect k3d-fundament-server-0 \
        --format '{{{{ (index .NetworkSettings.Networks "k3d-fundament").IPAddress }}')
    if [ -z "$node_ip" ]; then
        echo "Error: could not resolve k3d-fundament-server-0 IP on the k3d-fundament network"
        exit 1
    fi

    # 3. (Re)start the relay: host 0.0.0.0:HOSTPORT -> node_ip:NODEPORT, attached to
    #    the management network. Only this container is multi-homed; the k3d nodes
    #    are left alone so the plugin registry mirror keeps working.
    docker rm -f "$RELAY" >/dev/null 2>&1 || true
    docker run -d --name "$RELAY" --network k3d-fundament --restart unless-stopped \
        -p "0.0.0.0:${HOSTPORT}:8080" \
        alpine/socat "TCP-LISTEN:8080,fork,reuseaddr" "TCP:${node_ip}:${NODEPORT}" >/dev/null
    docker rm -f "$AUTHN_RELAY" >/dev/null 2>&1 || true
    docker run -d --name "$AUTHN_RELAY" --network k3d-fundament --restart unless-stopped \
        -p "0.0.0.0:${AUTHN_HOSTPORT}:8080" \
        alpine/socat "TCP-LISTEN:8080,fork,reuseaddr" "TCP:${node_ip}:${AUTHN_NODEPORT}" >/dev/null

    echo "- marketplace-catalog-api reachable from the sandbox at http://host.k3d.internal:${HOSTPORT}"
    echo "  (matches pluginController.catalogApiUrl in plugins/sandbox/values.yaml)"
    echo "- authn-api reachable from the sandbox at http://host.k3d.internal:${AUTHN_HOSTPORT}"
    echo "  (matches pluginController.authnApiUrl in plugins/sandbox/values.yaml)"

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

# Preflight needs `just plugins deploy` and `just functl org set <id>` done already.
# Iterate on a plugin with hot reload (publish a -dev version, install, skaffold dev)
dev name org="acme-corp":
    #!/usr/bin/env bash
    set -euo pipefail
    cd "$(git rev-parse --show-toplevel)"

    # 1. Preflight.
    if [ -z "$(kubectl --context {{ kube_context }} -n fundament get deploy -l app.kubernetes.io/component=plugin-controller -o name 2>/dev/null)" ]; then
        echo "plugin-controller is not deployed in the sandbox — run 'just plugins deploy' first"
        exit 1
    fi
    # A controller from before dev mode existed ignores spec.dev and deploys the
    # air image under prod lockdown — fail fast instead of a silent crash loop.
    if [ -z "$(kubectl --context {{ kube_context }} -n fundament get deploy -l app.kubernetes.io/component=plugin-controller -o jsonpath='{.items[0].spec.template.spec.containers[0].env[?(@.name=="PLUGIN_CONTROLLER_ALLOW_DEV_MODE")].value}' 2>/dev/null)" ]; then
        echo "the sandbox plugin-controller was deployed without dev mode (PLUGIN_CONTROLLER_ALLOW_DEV_MODE) — run 'just plugins deploy' to redeploy it from this checkout"
        exit 1
    fi

    # Helm never upgrades charts/fundament/crds/, so apply the current CRD by hand
    # (spec.dev is a recent addition).
    kubectl --context {{ kube_context }} apply -f charts/fundament/crds/plugininstallations.plugins.fundament.io.yaml

    # functl's config file path, resolved by functl itself (functl/pkg/config.ConfigPath)
    # rather than re-implementing its FUNCTL_CONFIG_DIR/XDG_CONFIG_HOME/~/.config fallback here.
    functl_config_path=$(go run ./functl/cmd/functl config path) || {
        echo "could not resolve functl's config path — run 'go run ./functl/cmd/functl config path' to see why"
        exit 1
    }
    functl_org=$(yq '.organization // ""' "${functl_config_path}" 2>/dev/null || echo "")
    sandbox_org=$(yq '.pluginController.organizationId' plugins/sandbox/values.yaml) || {
        echo "could not read pluginController.organizationId from plugins/sandbox/values.yaml"
        exit 1
    }
    # yq prints the string "null" with exit 0 when the key is missing: a key
    # genuinely absent must fail the same way a read error does.
    if [ -z "${sandbox_org}" ] || [ "${sandbox_org}" = "null" ]; then
        echo "could not read pluginController.organizationId from plugins/sandbox/values.yaml"
        exit 1
    fi
    if [ "${functl_org}" != "${sandbox_org}" ]; then
        echo "functl's organization (${functl_org:-<none>}) does not match pluginController.organizationId in plugins/sandbox/values.yaml (${sandbox_org}) — drafts are org-scoped by RLS; run 'just functl org set ${sandbox_org}'"
        exit 1
    fi

    # 2. The plugin's name comes from its definition, not its path: nested plugins
    # (plugins/storage/ceph-rook → ceph-rook) differ.
    plugin_name=$(yq '.metadata.name' "plugins/{{ name }}/definition.yaml")
    if [ -z "${plugin_name}" ] || [ "${plugin_name}" = "null" ]; then
        echo "could not read metadata.name from plugins/{{ name }}/definition.yaml"
        exit 1
    fi

    # The namespace passed to skaffold -n below must equal the controller's:
    # common/kubename hashes installation names once "plugin-" + name exceeds a
    # 63-char DNS label, and sync would silently track the wrong namespace.
    installation="{{ org }}--${plugin_name}"
    if [ "${#installation}" -gt 56 ]; then
        echo "installation name ${installation} is ${#installation} chars; the dev loop supports at most 56 (the controller hashes longer namespaces) — use a shorter organization or plugin name"
        exit 1
    fi

    # The dev version a draft is published under: metadata.version with the 'v'
    # stripped (the registry stores versions normalized the same way), '-dev'
    # appended unless already present (matches injectDevVersion's idempotence).
    raw_version=$(yq '.metadata.version' "plugins/{{ name }}/definition.yaml")
    raw_version="${raw_version#v}"
    case "${raw_version}" in
        *-dev) dev_version="${raw_version}" ;;
        *) dev_version="${raw_version}-dev" ;;
    esac

    # 3. Initial publish against the hotreload Dockerfile and dev image repo.
    repo="localhost:{{ registry_port }}/${plugin_name}-plugin-dev"
    tag=$(git describe --always --dirty)
    digest=$("{{ just_executable() }}" --justfile "{{ justfile() }}" plugins::_image-push "${repo}" "${tag}" plugins/hotreload/Dockerfile --build-arg "PLUGIN_PATH={{ name }}")
    go run ./functl/cmd/functl plugin publish "plugins/{{ name }}/definition.yaml" --image "${repo}@${digest}" --dev --create

    # 4. Render both templates.
    root="$(pwd)"
    render_dir="${root}/plugins/sandbox/dev/.render/{{ name }}"
    mkdir -p "${render_dir}"
    sed \
        -e "s|@NAME@|${plugin_name}|g" \
        -e "s|@PATH@|{{ name }}|g" \
        -e "s|@ROOT@|${root}|g" \
        plugins/sandbox/dev/skaffold.yaml.tmpl > "${render_dir}/skaffold.yaml"
    sed \
        -e "s|@NAME@|${plugin_name}|g" \
        -e "s|@ORG@|{{ org }}|g" \
        -e "s|@VERSION@|${dev_version}|g" \
        plugins/sandbox/dev/installation.yaml.tmpl > "${render_dir}/installation.yaml"

    # 5. cd into the render dir: skaffold resolves manifests.rawYaml against its
    # own working directory, not the -f file's directory. -n tracks the plugin
    # namespace for sync (the CR itself is cluster-scoped and contributes none);
    # --status-check=false because skaffold deploys only the CR.
    cd "${render_dir}"
    echo "Note: changes to go.mod/go.sum or definition.yaml are NOT hot-reloaded — restart 'just plugins dev {{ name }}' to rebuild and republish."
    exec skaffold dev -f skaffold.yaml --default-repo=localhost:{{ registry_port }} \
        -n "plugin-${installation}" --status-check=false --cleanup=false

# Remove a dev-mode PluginInstallation left behind by `dev`
dev-down name org="acme-corp":
    #!/usr/bin/env bash
    set -euo pipefail
    cd "$(git rev-parse --show-toplevel)"
    plugin_name=$(yq '.metadata.name' "plugins/{{ name }}/definition.yaml")
    kubectl --context {{ kube_context }} delete plugininstallation "{{ org }}--${plugin_name}" --ignore-not-found
