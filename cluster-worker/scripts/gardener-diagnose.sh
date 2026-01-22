#!/usr/bin/env bash
# Gardener diagnostic script - outputs to stdout and file
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GARDENER_DIR="${GARDENER_DIR:-$SCRIPT_DIR/../../.dev/gardener}"
KUBECONFIG="$GARDENER_DIR/example/provider-local/seed-kind/base/kubeconfig"
OUTPUT_FILE="${1:-/tmp/gardener-diagnose.txt}"

if [ ! -f "$KUBECONFIG" ]; then
    echo "Error: Kubeconfig not found at $KUBECONFIG"
    exit 1
fi

export KUBECONFIG

{
    echo "=== Gardener Diagnostics $(date) ==="
    echo "Kubeconfig: $KUBECONFIG"
    echo ""

    echo "=== Shoots Status ==="
    kubectl get shoots -A -o wide
    echo ""

    echo "=== Shoot Details ==="
    kubectl describe shoots -A
    echo ""

    echo "=== All Pods (shoot namespaces + etcd) ==="
    kubectl get pods -A | grep -E "(shoot--|etcd|garden)" | head -50
    echo ""

    echo "=== Unhealthy Pods ==="
    kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded | head -30
    echo ""

    echo "=== Pod Descriptions (non-running) ==="
    for pod in $(kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' 2>/dev/null | head -10); do
        ns=$(echo "$pod" | cut -d'/' -f1)
        name=$(echo "$pod" | cut -d'/' -f2)
        echo "--- Pod: $ns/$name ---"
        kubectl describe pod -n "$ns" "$name" 2>/dev/null | tail -30
        echo ""
    done

    echo "=== Recent Events (last 50) ==="
    kubectl get events -A --sort-by='.lastTimestamp' | tail -50
    echo ""

    echo "=== PVCs ==="
    kubectl get pvc -A
    echo ""

    echo "=== Nodes ==="
    kubectl get nodes -o wide
    echo ""

    echo "=== Resource Usage ==="
    kubectl top nodes 2>/dev/null || echo "Metrics not available"
    echo ""

} 2>&1 | tee "$OUTPUT_FILE"

echo ""
echo "Output saved to: $OUTPUT_FILE"
