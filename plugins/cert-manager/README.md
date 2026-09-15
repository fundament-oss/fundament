# Cert Manager plugin

Installs cert-manager and serves console pages for its resources.

- Helm chart `cert-manager` v1.17.2 from `https://charts.jetstack.io`, release `cert-manager`, `crds.enabled=true`
- CRDs: Certificate, CertificateRequest, Issuer, ClusterIssuer
- Console: list and detail pages in `console/`
- Config: none

## Flow

After steps 1–3 of [Testing plugins locally](../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. `PLUGIN_REGISTRY=localhost:5112 just plugins publish cert-manager`
2. Install `system--cert-manager` (step 5) with the printed version and hash.
3. `just plugins cert-manager test`
4. `just plugins cert-manager test-cleanup`

## Recipes

| Recipe | Does |
|---|---|
| `test` | Applies a self-signed ClusterIssuer and Certificate `test-cert` in `fundament`, waits 60 s for Ready |
| `test-cleanup` | Deletes them and the `test-cert-tls` Secret |
