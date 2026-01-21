# Build terraform provider
build:
    go build -o terraform-provider-fundament .

# Install terraform provider locally for testing
install: build
    #!/usr/bin/env bash
    OS=$(go env GOOS)
    ARCH=$(go env GOARCH)
    PLUGIN_DIR=~/.terraform.d/plugins/registry.opentofu.org/fundament/fundament/0.1.0/${OS}_${ARCH}
    mkdir -p "$PLUGIN_DIR"
    cp terraform-provider-fundament "$PLUGIN_DIR/"

# Run terraform provider tests
test:
    go test -v ./...

# Clean terraform provider build artifacts
clean:
    rm -f terraform-provider-fundament
