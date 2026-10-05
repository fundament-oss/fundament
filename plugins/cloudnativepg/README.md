# CloudNativePG plugin

Installs the CloudNativePG operator and lets project members create PostgreSQL databases from the console.

- Helm chart `cloudnative-pg` 0.24.0 (operator v1.26.0) from `https://cloudnative-pg.github.io/charts`, release `cnpg` in `cnpg-system`, chart default values
- CRD: Cluster (`clusters.postgresql.cnpg.io`)
- Console: create and read-only detail pages in `console/`; the list is the console's generated one, labelled Databases
- Config: none

## Scope

A database is a CNPG `Cluster` with 1 instance, no backups, and CNPG's default bootstrap: database `app`, owned by user `app`, credentials in Secret `<name>-app`. The create form sets name, namespace, storage size, PostgreSQL version (17.4 or 16.8) and StorageClass.

Project admins cannot update or delete the `Cluster` resource, from the console or with kubectl. To change or delete one, an organization admin uses kubectl, or a later version of this plugin adds it.

That protects the resource, not the data. The built-in `admin` role still covers what the operator creates in the namespace, so a project admin can:

- delete PVC `<name>-1` and pod `<name>-1`, which destroys a single-instance database;
- edit or delete Secret `<name>-app`;
- create a `Cluster` with kubectl, bypassing the form's limits: more instances, another image, backups, a custom bootstrap. Only a ResourceQuota on the namespace caps its size.

## Shared operator

openfsc installs the same `cnpg` release. Both call `plugins/internal/cnpg`, which:

- keeps the chart's default values, so neither plugin flips the other's settings;
- skips the install when the release is already deployed at its chart version or newer, so an older plugin never downgrades the operator or its CRDs;
- refuses while the release is `pending-*`. When a pod is killed during `helm --wait`, the release stays pending. Recover a stuck upgrade with `helm -n cnpg-system rollback cnpg`. A stuck first install (`pending-install`) has no earlier revision to roll back to: run `helm -n cnpg-system uninstall cnpg`, which is safe because it never deployed.

## User RBAC

The plugin applies two ClusterRoles on every start, which aggregate into the roles Fundament binds in project namespaces:

| ClusterRole | Aggregates into | Verbs on `clusters` |
|---|---|---|
| `fundament-cloudnativepg-admin` | `admin` (project admins) | get, list, watch, create |
| `fundament-cloudnativepg-view` | `view` (project viewers) | get, list, watch |

The chart's `rbac.aggregateClusterRoles` stays off: it would give `admin` every verb, delete included. The plugin's own `permissions.rbac` has the same four verbs, so its console pages cannot change or delete either.

## Uninstall

The plugin has no uninstall step. Uninstalling it removes only the console pages; the operator, the two ClusterRoles and every database stay.

## Known limitations

- Only organization admins can list StorageClasses. A project admin gets a text field instead of a list: empty means the cluster default, or they type a StorageClass name an organization admin gave them.
- The generated list lists databases across all namespaces, which only organization admins may do, so it fails to load for a project admin who is not also an organization admin. Every namespaced plugin kind shares this platform limitation.

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
