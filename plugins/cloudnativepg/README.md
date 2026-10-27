# CloudNativePG plugin

Installs the CloudNativePG operator and lets project members create PostgreSQL databases from the console.

- Helm chart `cloudnative-pg` 0.24.0 (operator v1.26.0) from `https://cloudnative-pg.github.io/charts`, release `fundament-cloudnativepg` in `cnpg-system`, chart default values
- CRDs: Cluster (`clusters.postgresql.cnpg.io`), ClusterImageCatalog (`clusterimagecatalogs.postgresql.cnpg.io`)
- ClusterImageCatalog `fundament-postgresql`: CloudNativePG's [standard bookworm catalog](https://github.com/cloudnative-pg/artifacts/blob/main/image-catalogs/catalog-standard-bookworm.yaml), fetched when the image is built. The plugin refuses to start with an entry that is not digest-pinned or not tagged with its own major
- Console: create and detail pages in `console/`; the list is the console's generated one, labelled Databases. The detail page deletes a database after the user types its name
- Config: none

## Scope

A database is a CNPG `Cluster` with 1 instance, no backups, and CNPG's default bootstrap: database `app`, owned by user `app`, credentials in Secret `<name>-app`. The create form sets name, namespace, storage size (1Gi to 1Ti, 5Gi by default), PostgreSQL major version (17 or 16) and StorageClass. CNPG's PodDisruptionBudget is off (`enablePDB: false`): for a single instance it allows no disruption and would block every node drain.

Project admins can delete a `Cluster`, from the console or with kubectl, but cannot update it. To change one, an organization admin uses kubectl. Deleting a `Cluster` deletes its PVC with it, and there are no backups, so the data is gone.

Blocking updates protects the resource, not the data. The built-in `admin` role still covers what the operator creates in the namespace, so a project admin can:

- delete PVC `<name>-1` and pod `<name>-1`, which destroys a single-instance database;
- edit or delete Secret `<name>-app`;
- create a `Cluster` with kubectl, bypassing the form's limits: more instances, another image, backups, a custom bootstrap. Only a ResourceQuota on the namespace caps its size.

## PostgreSQL versions

A database refers to its major version in the ClusterImageCatalog `fundament-postgresql` (`imageCatalogRef`), not to an exact image. The Dockerfile fetches CloudNativePG's catalog, the current digest-pinned image per major, republished weekly, when the plugin image is built, and the plugin applies it on start and on every reconcile (`cnpg/catalog.go`), with the majors operator 1.26 runs (up to 17).

So every plugin upgrade moves every database to the current minor of its major. CNPG restarts a database when its catalog entry changes, within about half a minute of the upgrade. The plugin only raises catalog entries: a plugin rollback keeps the newer images, and keeps majors that a newer plugin added.

Not yet: fetching the catalog at runtime, so fixes do not wait for a plugin release, and letting the owner pick the moment of the restart ("Update to 17.x" on the detail page). CNPG's own way to defer an update, `primaryUpdateStrategy: supervised`, is rejected for a single instance.

## Operator ownership

A cluster runs one CloudNativePG operator, and every operator version change restarts every database on the cluster. The plugin does not share the operator, so that no other writer changes its version at a moment the plugin does not know about. It installs its own release, `fundament-cloudnativepg`, rather than the chart's usual `cnpg`, which openfsc installs (`cnpg/`):

- On every start and every reconcile it checks that the operator is its own: the Helm annotations on every `postgresql.cnpg.io` CRD name its release (Helm refuses the install when any of the chart's CRDs names another), and no other Deployment runs the operator image. No CRD, or its own: it installs, upgrades or repairs as below. CRDs kept from its own uninstall still carry its annotations, so a reinstall counts as its own.
- Someone else's at start: the plugin reports Failed, names the owner and how to hand the operator over (below), and retries every 30 seconds, so it goes ahead once the hand-over is done. Removing the other install alone is not enough: its kept CRDs still name its release, and the plugin does not take them over by itself. Someone else's later: it reports Degraded, names the owner, and stops repairing and upgrading.
- It keeps the chart's default values.
- It never downgrades: it installs the newer of its pinned chart version and the release's own, whatever state the release is in, so a plugin rollback keeps a newer operator and its CRDs.
- It skips the install when the release is deployed at that version and the operator Deployment and CRDs exist. When one was deleted, it reinstalls at the deployed version to restore it, at startup and on every reconcile (every 5 minutes, so the plugin reports Running until the next reconcile notices).
- It waits while the release is `pending-*` or `uninstalling`. For up to 10 minutes that is another operation in progress, such as an earlier pod's install during a rolling update, and the plugin reports Installing. Past that, the pod running the install was most likely killed during `helm --wait` and the release stays pending. Recover a stuck upgrade with `helm -n cnpg-system rollback fundament-cloudnativepg`. A stuck first install (`pending-install`) has no earlier revision to roll back to; clear it, or a stuck uninstall, with `helm -n cnpg-system uninstall fundament-cloudnativepg`. That is safe: the chart marks its CRDs `helm.sh/resource-policy: keep`, so they stay, and so does every database. The plugin then installs the operator again.

This cannot prevent a takeover: organization admins are cluster-admin on the cluster, and `kubectl apply` of the CNPG manifests or `helm install --take-ownership` skip Helm's ownership check. It makes one visible, and the plugin does not fight it.

openfsc and this plugin are mutually exclusive on one cluster until openfsc stops bundling CloudNativePG ([miscellaneous#1289](https://gitlab.com/digilab.overheid.nl/miscellaneous/issues/-/work_items/1289)). To hand an operator someone else installed over to the plugin, remove the other install, then point the kept CRDs at the plugin's release. Helm adopts an existing object only when it carries the release's annotations and Helm's `managed-by` label:

```shell
helm -n cnpg-system uninstall cnpg   # the CRDs and databases stay
crds=$(kubectl get crd -o name | grep postgresql.cnpg.io)
kubectl label --overwrite $crds app.kubernetes.io/managed-by=Helm
kubectl annotate --overwrite $crds \
  meta.helm.sh/release-name=fundament-cloudnativepg meta.helm.sh/release-namespace=cnpg-system
```

An operator installed from CloudNativePG's manifests rather than a Helm release has no `helm uninstall`. Delete everything it created except the CRDs: deleting a CRD deletes every database of its kind. An extra operator Deployment next to the plugin's only needs that Deployment deleted.

## User RBAC

The plugin applies two ClusterRoles on every start, which aggregate into the roles Fundament binds in project namespaces:

| ClusterRole | Aggregates into | Verbs on `clusters` |
|---|---|---|
| `fundament-cloudnativepg-admin` | `admin` (project admins) | get, list, watch, create, delete |
| `fundament-cloudnativepg-view` | `view` (project viewers) | get, list, watch |

The chart's `rbac.aggregateClusterRoles` stays off: it would give `admin` every verb, update and patch included. The plugin's own `permissions.rbac` has the same five verbs, so its console pages cannot change a database either.

A project viewer still gets the create form, since the console offers Create to everyone who can see the list; submitting it says the project admin role is needed.

## Uninstall

Uninstalling refuses while databases depend on the plugin, and names them: without the plugin they would keep running with no console page to find or delete them. With the operator its own, that is every database, since the operator goes too. With someone else's operator, it is only the databases on the plugin's image catalog, which cannot start once the catalog is gone; the others belong to whoever runs that operator. With none left, it deletes the two ClusterRoles, the image catalog and, when it is its own, the operator release. The CRDs stay (`resource-policy: keep`).

Once the uninstall goes ahead, the plugin stops installing and repairing: a reconcile in between would put back what the uninstall removes.

plugin-controller skips the plugin's uninstall when its pod is unreachable, so the refusal only holds while the plugin is up.

## Known limitations

- A database whose volume cannot be provisioned (a mistyped StorageClass, a size the provisioner refuses) stays "Setting up primary". The detail page shows CNPG's phase reason and the Ready condition's message, but the PVC's own events are not visible to project members.

## Flow

After steps 1–3 of [Testing plugins locally](../../docs/developer/plugins/testing-plugins-locally.md), from the repository root:

1. `PLUGIN_REGISTRY=localhost:5112 just plugins publish cloudnativepg`
2. Install `system--cloudnativepg` (step 5) with the printed version and hash.
3. `just plugins cloudnativepg test`
4. `just plugins cloudnativepg test-cleanup`

Connect to a database from inside the cluster:

```shell
kubectl --context k3d-fundament-plugin -n cnpg-test get secret test-db-app -o jsonpath='{.data.password}' | base64 -d
kubectl --context k3d-fundament-plugin -n cnpg-test run psql --rm -it --image=postgres:17 -- psql postgresql://app:<password>@test-db-rw.cnpg-test.svc/app
```

## Recipes

| Recipe | Does |
|---|---|
| `test` | Creates namespace `cnpg-test` and database `test-db` in it, waits 300 s for Ready |
| `test-cleanup` | Deletes them |
