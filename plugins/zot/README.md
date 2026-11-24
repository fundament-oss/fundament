# zot plugin

Installs [zot](https://zotregistry.dev), an OCI-native container image registry, in the cluster.

- Helm chart `zot` 0.1.135 (zot v2.1.23) from `https://zotregistry.dev/helm-charts`, release `zot` in namespace `zot`
- Values: the chart defaults, with `service.type=ClusterIP`. The chart's default is a NodePort, which opens the registry on every node.
- Address: `zot.zot.svc:5000`, plain HTTP, inside the cluster only
- CRDs: the console CRD `registries.zot.fundament.io` of the plugin. zot has no CRDs.
- Console: the item **Registry** in the organization sidebar, with the generated list and detail pages
- Config: none
- Logos: `logo.svg` and `menu.svg` come from the zot icon in the [CNCF artwork repository](https://github.com/cncf/artwork/tree/main/projects/zot/icon), Apache-2.0. The files crop the icon to the mark. `menu.svg` is in one colour, because the console tints menu icons.

## Console

The console shows only CRDs in the sidebar. At the organization level, it shows only cluster-scoped CRDs. zot has no CRDs, so the plugin applies its own cluster-scoped CRD from `crds/`.

The plugin keeps one `Registry` object, `zot`. Each time the plugin reports a status, it writes these fields into the object:

| Field | What it is |
|---|---|
| `status.phase` | The plugin phase, such as `running` or `degraded` |
| `status.message` | The plugin status message |
| `status.address` | `zot.zot.svc:5000` |
| `status.image` | The image of the zot Deployment |
| `status.available` | Available replicas out of the wanted replicas, such as `1/1` |

The object does not control zot. A console user reads the object, so the user does not need access to the namespace `zot`. If you delete the object, the plugin creates it again at the next Reconcile. A failed write to the object does not change the plugin status.

## Lifecycle

The plugin follows the operator pattern of the other first-party plugins, though zot itself is only a Helm release. zot keeps running while the plugin pod is down.

| Event | What the plugin does |
|---|---|
| Start | Applies the console CRD. Installs or upgrades the release to the pinned chart version. It skips the install when the release is deployed at that version and the Deployment exists. It retries every 30 seconds until the install succeeds. |
| Reconcile | Installs again when the Deployment was deleted, because Helm still reports the release as deployed. Reports the status. |
| Uninstall | Deletes the console CRD and the `Registry` object. Then runs `helm uninstall zot`. The namespace `zot` stays. |
| Install, Upgrade | Do nothing. Start installs and upgrades on each start of the pod. |

The plugin is `Running` when the zot Deployment has all replicas available, and `Degraded` when it has not.

When the release is `pending-*` or `uninstalling` for less than 10 minutes, the plugin reports `Installing` and waits. After 10 minutes, the pod that ran the operation was most likely killed. Clear a stuck upgrade with `helm -n zot rollback zot`. Clear a stuck first install or uninstall with `helm -n zot uninstall zot`. The plugin then installs zot again.

## Known limitations

- No persistence: zot stores images in an `emptyDir`. A restart of the zot pod, and an uninstall, delete every image.
- No authentication and no TLS. Every workload that can reach the Service can push and pull.
- No access from outside the cluster: no Ingress and no HTTPRoute.
- Nodes pull images through the container runtime on the host, which does not resolve `zot.zot.svc`. To run images from zot, set up a registry mirror or a node-reachable address first.

## Flow

After steps 1–3 of [Testing plugins locally](../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. `PLUGIN_REGISTRY=localhost:5112 just plugins publish zot --create --submit`, then approve it ([step 5](../../docs/developer/plugins/testing-plugins-locally.md#5-install)): the sandbox installs a `system` version only once it is approved.
2. Install `system--zot` (step 5) with the printed version and hash.
3. `just plugins zot test`
4. `just plugins zot test-cleanup`

## Recipes

| Recipe | Does |
|---|---|
| `test` | Copies `busybox:1.37` into zot as `test/busybox:1.37` from a pod in the cluster, and checks that the catalog lists it |
| `test-cleanup` | Deletes `test/busybox:1.37` from zot |
