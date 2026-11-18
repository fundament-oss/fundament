# gardener-extension-tenant-metrics

A Gardener extension that makes each shoot's Prometheus collect container
metrics from tenant namespaces (`tnt-*`). Gardener itself collects them for
`kube-system` only and has no setting to widen that, so without this
extension the console's per-namespace table is empty.

## How it works

- It runs on the seed. gardenlet creates an `Extension` of type
  `fundament-tenant-metrics` in the control-plane namespace of every shoot that
  has workers.
- For each one the controller copies Gardener's own `shoot-cadvisor`
  `ScrapeConfig` to `fundament-tenant-cadvisor`, with its own job name and the
  namespace filter set to tenant namespaces. The copy follows the installed
  Gardener version, also after an upgrade.
- The copy is owned by the `Extension`, so it goes when the shoot goes.
- Pod counts per namespace can be derived from these container metrics.
  Requests and limits are not available: they come from kube-state-metrics,
  which Gardener runs for `kube-system` only.

## Build and publish

`.github/workflows/build.yml` builds the image with the other services, as
`ghcr.io/fundament-oss/fundament/gardener-extension-tenant-metrics`. The
`publish-chart` job packages `chart/` with that image pinned by digest and
pushes it next to the fundament chart, with the same version:

- master: `oci://ghcr.io/fundament-oss/fundament/charts/gardener-extension-tenant-metrics`,
  version `0.1.0-master.<run>`
- PR: `oci://ghcr.io/fundament-oss/fundament/charts-pr/gardener-extension-tenant-metrics`,
  version `0.1.0-pr<number>.<run>`

## Install

With gardener-operator, apply an `operator.gardener.cloud/v1alpha1`
`Extension` named `fundament-tenant-metrics` to the runtime cluster. It points
`.spec.deployment.extension.helm.ociRepository` at the published chart and
declares the resource `kind: Extension`, `type: fundament-tenant-metrics`,
`autoEnable: [shoot]`, `workerlessSupported: false`.

Without gardener-operator, apply to the virtual garden, together:

1. A `ControllerDeployment` named `fundament-tenant-metrics` that carries the
   chart, packaged from `chart/` with the value `image` set, or the published
   one.
2. `deploy/controller-registration.yaml`.

Gardener refuses to reconcile shoots while a registered extension is not
installed and healthy on the seed. Register it only with a working image, and
never without its chart.

## Cost

About 12 series per tenant container in the shoot's Prometheus, which Gardener
sizes for the control plane and system components.
