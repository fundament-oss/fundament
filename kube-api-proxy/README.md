# kube-api-proxy

Reverse proxy that routes kubectl traffic to Gardener-managed Kubernetes clusters. Authenticates users via Fundament JWTs, checks authorization via OpenFGA, and injects per-user ServiceAccount tokens before forwarding requests to the target cluster.

## How it works

```
kubectl ──(JWT)──> kube-api-proxy ──(SA token)──> shoot API server
```

1. User runs `kubectl` with a kubeconfig pointing at `https://proxy/clusters/{cluster-id}`
2. The exec credential plugin (`functl cluster token`) provides a Fundament JWT
3. Proxy validates the JWT and checks OpenFGA (`can user:{uid} can_view cluster:{cid}`)
4. Proxy fetches/caches an admin kubeconfig from Gardener for the target cluster
5. Proxy requests a short-lived SA token for `fundament-{user-id}` in `fundament-system` namespace
6. Proxy strips the user's auth headers and injects the SA token as `Authorization: Bearer {sa-token}`
7. Proxy forwards the request to the shoot API server

## Packages

- `cmd/fun-kube-api-proxy` — entrypoint, config, server lifecycle
- `pkg/proxy` — HTTP server, routing, JWT auth, OpenFGA authz
- `pkg/kube` — reverse proxy, admin kubeconfig cache, 401 retry transport, mock client
- `pkg/token` — per-user SA token cache with proactive refresh and singleflight dedup
- `pkg/gardener` — Gardener API client for admin kubeconfig requests

## Modes

- **`mock`** (default): serves mock Kubernetes API responses for console frontend development. No Gardener connection needed.
- **`real`**: connects to Gardener, proxies to real shoot clusters. Requires `GARDENER_KUBECONFIG`.

## Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JWT_SECRET` | yes | | Shared secret for JWT validation |
| `KUBE_API_PROXY_MODE` | no | `mock` | `mock` or `real` |
| `GARDENER_KUBECONFIG` | real mode | | Path to Gardener kubeconfig file |
| `LISTEN_ADDR` | no | `:8081` | HTTP listen address |
| `LOG_LEVEL` | no | `info` | Log level (debug, info, warn, error) |
| `CORS_ALLOWED_ORIGINS` | no | | Space-separated allowed origins |
| `OPENFGA_API_URL` | yes | | OpenFGA API endpoint |
| `OPENFGA_STORE_ID` | yes | | OpenFGA store ID |
| `OPENFGA_AUTHORIZATION_MODEL_ID` | yes | | OpenFGA authorization model ID |

## Caching

Two-level cache minimizes latency and Gardener API calls:

- **Admin kubeconfig cache** (per cluster): singleflight-deduplicated, refreshes at 70% of TTL
- **SA token cache** (per user+cluster pair): 15-minute TTL from TokenRequest API, proactive refresh at 80% of TTL, force-refresh on 401

## Error responses

| Code | Meaning |
|------|---------|
| 401 | Missing or invalid JWT |
| 403 | OpenFGA denies `can_view` for this user+cluster |
| 404 | Path doesn't match `/clusters/{uuid}/{api\|apis\|openapi}` |
| 503 | ServiceAccount not yet created (cluster-worker hasn't synced) |

## Local development

The proxy runs behind ingress-nginx in k3d. HTTPS is required for kubectl exec credential plugins (client-go refuses exec over HTTP). Local dev uses port 8443 mapped to a fixed NodePort 30443.

```bash
# With real Gardener (requires gardener-up first)
just dev -p local-gardener

# Mock mode (default, for frontend development)
just dev
```
