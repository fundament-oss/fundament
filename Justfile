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
dev:
	SKAFFOLD_DEFAULT_REPO="localhost:5111" \
	skaffold dev -p local --cleanup=false

# Deploy to local k3d cluster (one-time deployment)
deploy-local:
    skaffold run -p local

# Deploy to sandbox environment
deploy-sandbox:
    skaffold run -p sandbox

# Delete deployment from local cluster
undeploy-local:
    skaffold delete -p local

# Delete deployment from sandbox
undeploy-sandbox:
    skaffold delete -p sandbox

# View logs from all pods
logs:
    kubectl logs -n fundament -l app.kubernetes.io/instance=fundament --all-containers -f

# Open a shell to the PostgreSQL database
db-shell:
    kubectl exec -it -n fundament deployment/fundament-db-postgresql -- psql -U postgres -d fundament
