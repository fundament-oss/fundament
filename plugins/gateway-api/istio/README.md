# Gateway API (Istio) plugin

Installs Istio as a Gateway API implementation and a default Gateway.

- Helm charts from `https://istio-release.storage.googleapis.com/charts` in `istio-system`: `istio-base`, `istiod` (sidecar injection off for `minimal`, on for `full`), `istio-ingressgateway` (LoadBalancer on 80 and 443)
- Gateway `fundament-gateway` (class `istio`)
- Needs the Gateway API CRDs: the plugin checks for them, the Istio charts don't ship them. Install gateway-api-envoy first.
- Console: no custom pages

| Config (`FUNP_`) | Default |
|---|---|
| `ISTIO_PROFILE` | `minimal` (or `full`) |
| `ISTIO_VERSION` | `1.26.0` |
| `GATEWAY_NAME` | `fundament-gateway` |
| `GATEWAY_NAMESPACE` | `istio-system` |

## Flow

After steps 1–3 of [Testing plugins locally](../../../docs/developer/plugins/testing-plugins-locally.md) and gateway-api-envoy installed, from the repository root:

1. `just plugins envoy-gateway test-cleanup`: the install waits for the `istio-ingressgateway` address, which it gets only while ports 80 and 443 are free.
2. `PLUGIN_REGISTRY=localhost:5112 just plugins publish gateway-api/istio --create`
3. Install `system--gateway-api-istio` (step 5) with the printed version and hash.
4. `just plugins status`: Running.

## Limits

- `fundament-gateway` gets no address in the sandbox: `istio-ingressgateway` holds port 80 on the single node.
- Uninstall refuses while Gateways or Routes other than `fundament-gateway` exist, including gateway-api-envoy's.
- Uninstall leaves the GatewayClasses `istio` and `istio-remote`.
