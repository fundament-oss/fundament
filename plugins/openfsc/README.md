# OpenFSC plugin

Thin installer around the standalone [openfsc-operator](../../openfsc-operator)
plus read-only console pages. The plugin:

- installs the prerequisites the operator preflights but never installs itself:
  cert-manager and CloudNativePG,
- helm-installs the operator chart vendored into the plugin image at
  `/operator-chart` (the FSCInstallation CRD included), and
- serves the console pages listing every FSCInstallation with its per-gateway
  status.

The plugin never creates FSCInstallations. Teams declare their own in their
namespaces (kubectl/GitOps); see
`openfsc-operator/config/samples/openfsc_v1_fscinstallation.yaml`. All
reconciliation lives in the operator, so installations keep running when the
plugin pod is down. The plugin status summarizes the cluster
(`N installations: A active, P pending, E error`); uninstalling refuses while
FSCInstallations exist, because removing the operator first would strand their
finalizers.

## Configuration

`PluginInstallation.spec.config` entries surface as `FUNP_<KEY>` env vars:

| Env var | Default | Purpose |
| --- | --- | --- |
| `FUNP_OPERATOR_IMAGE` | `ghcr.io/fundament-oss/fundament/openfsc-operator:latest` | openfsc-operator image deployed by the vendored chart |

## Sandbox flow

From `plugins/` (see `plugins/README.md` for cluster setup):

```shell
just openfsc operator-push    # build the operator image, record it in .sandbox-config
just plugin-install openfsc   # prerequisites + operator (picks up .sandbox-config)
just openfsc test             # sample FSCInstallation (Self mode + gateways) reaches Active
just openfsc console-preview  # view the console pages against the live cluster
just openfsc test-cleanup     # remove the sample installation
```

`console-preview` serves `console/` with a kubectl-backed stand-in for the
plugin SDK (`scripts/console-preview/`), so the templates render the same CRs
the real console would — no console host needed.
