# Gateway API (Envoy Gateway) plugin

Installs Envoy Gateway and the Gateway API CRDs, and serves create forms for Gateway API and Envoy Gateway resources.

- Helm chart `oci://docker.io/envoyproxy/gateway-helm`, release `eg`
- CRDs: Gateway API (bundle v1.5.1, experimental channel) and Envoy Gateway policies, from the chart
- GatewayClass `eg`
- Console: Vite app in `console-ui/`, built into the image by the Dockerfile

| Config (`FUNP_`) | Default |
|---|---|
| `ENVOY_GATEWAY_VERSION` | `v1.8.3` |
| `GATEWAY_NAMESPACE` | `envoy-gateway-system` |
| `GATEWAY_CLASS_NAME` | `eg` |

## Flow

After steps 1–3 of [Testing plugins locally](../../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. After changes in `console-ui/`: `just plugins envoy-gateway typecheck` and `ui-test`.
2. `PLUGIN_REGISTRY=localhost:5112 just plugins publish gateway-api/envoy-gateway`
3. Install `system--gateway-api-envoy` (step 5) with the printed version and hash.
4. `just plugins envoy-gateway test`
5. `just plugins envoy-gateway test-cleanup`

## Recipes

| Recipe | Does |
|---|---|
| `typecheck`, `ui-test` | bun typecheck and unit tests of `console-ui/`; no cluster |
| `test` | Waits for Running, GatewayClass `eg` Accepted, creates Gateway `demo` (HTTP, port 80) in `eg-demo`, waits 300 s for Programmed |
| `test-cleanup` | Deletes Gateway `demo` and `eg-demo` |

## Limits

- The sandbox has one node: a Gateway gets an address only while no other LoadBalancer Service holds its port. gateway-api-istio's `istio-ingressgateway` holds 80 and 443.
- Uninstall refuses while any Gateway or Route exists, including gateway-api-istio's `fundament-gateway`.
