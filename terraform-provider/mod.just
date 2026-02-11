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
    PLUGIN_DIR=~/.terraform.d/plugins/registry.opentofu.org/fundament/fundament/0.1.0/${OS}_${ARCH}
    mkdir -p "$PLUGIN_DIR"
    cp terraform-provider-fundament "$PLUGIN_DIR/"

    # Clean up terraform state in all examples so they pick up the new provider
    EXAMPLES_DIR="{{ source_directory() }}/examples"
    if [[ -d "$EXAMPLES_DIR" ]]; then
        echo "Cleaning terraform state in ${EXAMPLES_DIR}..."
        find "$EXAMPLES_DIR" -name '.terraform' -type d -exec rm -rf {} + 2>/dev/null || true
        find "$EXAMPLES_DIR" -name '.terraform.lock.hcl' -type f -delete 2>/dev/null || true

        # Run tofu init in each example directory containing .tf files
        echo "Initializing examples..."
        for dir in $(find "$EXAMPLES_DIR" -name '*.tf' -exec dirname {} \; | sort -u); do
            echo "  tofu init in ${dir#$EXAMPLES_DIR/}..."
            (cd "$dir" && tofu init -input=false > /dev/null)
        done
        echo "Done."
    fi

# Run terraform provider tests
test:
    go test -v ./...

# Clean terraform provider build artifacts
clean:
    rm -f terraform-provider-fundament
