_default:
    @just --list

# Watch for changes to .d2 files and re-generate .svgs
watch-d2:
    d2 --theme=0 --dark-theme=200 --watch docs/assets/*.d2

# Format all code and text in this repo
fmt:
    @find . -type f \( -name "*.md" -o -name "*.d2" \) -exec sed -i 's/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/g' {} +
    d2 fmt docs/assets/*.d2
    # TODO md fmt
    # TODO go fmt

# --- Cluster commands ---

# Create a local k3d cluster for development with local registry
cluster-create:
    k3d cluster create --config=deploy/k3d/config.yaml

# Start the cluster (creates if it doesn't exist)
cluster-start:
    @k3d cluster list fundament > /dev/null 2>&1 && k3d cluster start fundament || just cluster-create

# Stop the cluster without deleting it
cluster-stop:
    k3d cluster stop fundament

# Delete the local k3d cluster and registry
cluster-delete:
    k3d cluster delete fundament
    @k3d registry delete registry.localhost 2>/dev/null || true

# --- Deployment commands ---

# Update helm dependencies
helm-deps:
    helm dependency update deploy/charts/ingress-nginx
    helm dependency update deploy/charts/db
    helm dependency update deploy/charts/fundament

# Deploy to local k3d cluster (development mode with hot-reload, keeps resources on exit)
dev *flags:
    SKAFFOLD_DEFAULT_REPO="localhost:5111" \
    skaffold dev --profile env-local --cleanup=false {{ flags }}

dev-hotreload:
    @just dev --profile hotreload

# Deploy to an environment (e.g. local, production)
deploy env:
    skaffold run --profile env-{{ env }}

# Delete deployment, can also be used to remove the deployment created by `just dev`.
undeploy env:
    skaffold delete --profile env-{{ env }}

# View logs from all pods
logs:
    kubectl logs -n fundament -l app.kubernetes.io/instance=fundament --all-containers -f

# Open a shell to the PostgreSQL database
db-shell:
    kubectl exec -it -n fundament fundament-db-1 -- psql -U postgres -d fundament

generate:
    just sqlc && just proto

# Generate protobuf code for authn-api
proto:
    cd authn-api/proto && buf generate
    cd organization-api/proto && buf generate

# Generate all sqlc code
sqlc:
    cd authn-api/pkgs/storage/sqlc && sqlc generate
    cd organization-api/pkgs/storage/sqlc && sqlc generate

# Lint all Go code
lint:
    cd authn-api && golangci-lint run ./...
    cd organization-api && golangci-lint run ./...
