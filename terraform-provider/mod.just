_default:
    @just --list terraform-provider

# Build terraform provider
build:
    go build -o terraform-provider-fundament .

# Install terraform provider locally for testing
install: build
    #!/usr/bin/env bash
    set -euo pipefail
    OS=$(go env GOOS)
    ARCH=$(go env GOARCH)
    MIRROR=~/.terraform.d/plugins/registry.opentofu.org/fundament/fundament
    PLUGIN_DIR=${MIRROR}/0.1.0/${OS}_${ARCH}
    # Remove a downloaded release package for this platform so the dev build
    # is the only 0.1.0 in the mirror.
    rm -fv "$MIRROR"/terraform-provider-fundament_*_${OS}_${ARCH}.zip
    mkdir -p "$PLUGIN_DIR"
    cp terraform-provider-fundament "$PLUGIN_DIR/"

# Run terraform provider tests
test:
    go test -v ./...

# Run terraform provider acceptance tests
test-acc:
    FUNDAMENT_ENDPOINT=https://organization.fundament.localhost:8443 \
    TF_ACC=1 \
    TF_ACC_TERRAFORM_PATH=$(which tofu) \
    TF_ACC_PROVIDER_HOST=registry.opentofu.org \
    go test -v ./...

# Generate the resource and data source reference in docs/ from the provider schema
docs:
    #!/usr/bin/env bash
    set -euo pipefail
    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT
    go build -o "$TMP/terraform-provider-fundament" .
    printf 'provider_installation {\n  dev_overrides {\n    "registry.opentofu.org/fundament/fundament" = "%s"\n  }\n  direct {}\n}\n' "$TMP" > "$TMP/tofurc"
    printf 'terraform {\n  required_providers {\n    fundament = { source = "fundament/fundament" }\n  }\n}\n' > "$TMP/main.tf"
    # tfplugindocs looks a schema up by the provider's short name, not by OpenTofu's registry address.
    (cd "$TMP" && TF_CLI_CONFIG_FILE="$TMP/tofurc" tofu providers schema -json) \
        | jq '.provider_schemas |= with_entries(.key = "fundament")' > "$TMP/schema.json"
    tfplugindocs generate --provider-name fundament --providers-schema "$TMP/schema.json"

# Validate every example, and every configuration on the OpenTofu docs page that declares its providers
validate-examples:
    #!/usr/bin/env bash
    set -euo pipefail
    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT
    # A filesystem mirror rather than dev_overrides: tofu init refuses overridden
    # providers, and configurations that also use other providers need init.
    PLUGIN_DIR="$TMP/mirror/registry.opentofu.org/fundament/fundament/0.1.0/$(go env GOOS)_$(go env GOARCH)"
    go build -o "$PLUGIN_DIR/terraform-provider-fundament" .
    printf 'provider_installation {\n  filesystem_mirror {\n    path    = "%s"\n    include = ["registry.opentofu.org/fundament/*"]\n  }\n  direct {\n    exclude = ["registry.opentofu.org/fundament/*"]\n  }\n}\n' "$TMP/mirror" > "$TMP/tofurc"
    export TF_CLI_CONFIG_FILE="$TMP/tofurc"

    mkdir "$TMP/configs"
    for dir in examples/provider/ examples/*/*/; do
        name="${dir%/}"
        dest="$TMP/configs/${name//\//_}"
        cp -r "$dir" "$dest"
        # An example shows one resource; give it the provider requirement it leaves out.
        grep -qs required_providers "$dest"/*.tf ||
            printf 'terraform {\n  required_providers {\n    fundament = { source = "fundament/fundament" }\n  }\n}\n' > "$dest/versions.tf"
    done
    awk -v out="$TMP/configs/docs-page" '
        /^```hcl/ { n++; block = ""; inblock = 1; next }
        /^```/ && inblock {
            inblock = 0
            if (block ~ /required_providers/) { dir = out "-" n; system("mkdir -p " dir); printf "%s", block > (dir "/main.tf"); close(dir "/main.tf") }
            next
        }
        inblock { block = block $0 "\n" }
    ' ../docs/user/opentofu-provider.md

    failed=0
    for dir in "$TMP"/configs/*/; do
        name=$(basename "$dir")
        if (cd "$dir" && tofu init -input=false -backend=false -no-color > init.log 2>&1 && tofu validate -no-color > validate.log 2>&1); then
            echo "ok    $name"
        else
            echo "FAIL  $name"
            cat "$dir"/*.log
            failed=1
        fi
    done
    exit $failed

# Clean terraform provider build artifacts
clean:
    rm -f terraform-provider-fundament
