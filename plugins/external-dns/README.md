# External DNS plugin

Installs ExternalDNS, reading records from DNSEndpoint resources.

- Helm chart `external-dns` 1.16.1 from `https://kubernetes-sigs.github.io/external-dns/`, release `external-dns`, `sources={crd}`
- CRD: DNSEndpoint (from the chart)
- Console: no custom pages
- Config: none. The chart's default provider is `aws`, with no credentials: ExternalDNS runs and logs Route 53 errors, and writes no records.

## Flow

After steps 1–3 of [Testing plugins locally](../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. `PLUGIN_REGISTRY=localhost:5112 just plugins publish external-dns --create`
2. Install `system--external-dns` (step 5) with the printed version and hash.
3. `just plugins external-dns test`
4. `just plugins external-dns test-cleanup`

## Recipes

| Recipe | Does |
|---|---|
| `test` | Applies DNSEndpoint `test-dns` (`test.fundament.local` A `127.0.0.1`) in `fundament` and lists it. Shows the CRD accepts the object; no record is written. |
| `test-cleanup` | Deletes it |
