# Cert Manager plugin

Installs cert-manager and serves console pages for its resources.

- Helm chart `cert-manager` v1.21.3 from `https://charts.jetstack.io`, release `cert-manager`, `crds.enabled=true`, and `config.gatewayAPI.enabled=true` while the Gateway API CRDs are installed
- CRDs: Certificate, CertificateRequest, Issuer, ClusterIssuer
- Console: list and detail pages in `console/`
- Config: none

## Gateway API

cert-manager stops at start when its Gateway API support is on and the Gateway API CRDs are missing, so the plugin turns the support on only while they are installed (by the Envoy Gateway plugin; the Istio charts do not ship them). When they appear or go away, the next reconcile (every 5 minutes) reinstalls the release with the matching setting. The plugin status ends in `Gateway API support is off` while it is off.

With it on, a Gateway gets its certificates from the annotation `cert-manager.io/cluster-issuer: <issuer>` (or `cert-manager.io/issuer`): cert-manager creates a Certificate for every HTTPS listener with a `hostname` and a `tls.certificateRefs` Secret. An ACME issuer solves HTTP-01 through a Gateway with a `gatewayHTTPRoute` solver:

```yaml
solvers:
  - http01:
      gatewayHTTPRoute:
        parentRefs:
          - name: <gateway>
            namespace: <gateway namespace>
            kind: Gateway
            sectionName: <name of the HTTP listener on port 80>
```

In a ClusterIssuer, such a solver lets everyone who can create a Certificate pass the challenge for any hostname that Gateway serves. On a shared cluster, put it in a namespaced Issuer instead.

See [cert-manager: Gateway](https://cert-manager.io/docs/usage/gateway/) and [HTTP01 with Gateway API](https://cert-manager.io/docs/configuration/acme/http01/#configuring-the-http-01-gateway-api-solver).

## Flow

After steps 1–3 of [Testing plugins locally](../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. `PLUGIN_REGISTRY=localhost:5112 just plugins publish cert-manager --submit`, then approve it ([step 5](../../docs/developer/plugins/testing-plugins-locally.md#5-install)): the sandbox installs a `system` version only once it is approved.
2. Install `system--cert-manager` (step 5) with the printed version and hash.
3. `just plugins cert-manager test`
4. `just plugins cert-manager test-cleanup`

## Recipes

| Recipe | Does |
|---|---|
| `test` | Applies a self-signed ClusterIssuer and Certificate `test-cert` in `fundament`, waits 60 s for Ready |
| `test-cleanup` | Deletes them and the `test-cert-tls` Secret |
