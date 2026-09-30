# Review: 51c5e25d7 feat(cloudnativepg)

Checked: `go test` passes for cloudnativepg, internal/cnpg, openfsc and the helm helper; `golangci-lint` reports 0 issues; `go fix -omitzero=false -embedlit=false` is clean. `helm template` of chart 0.24.0 renders only kinds the install-time `permissions.rbac` covers, and `rbac.aggregateClusterRoles` defaults to false. Console calls go through the FUN-17 user ∩ plugin-SA intersection, so the create form can only write to namespaces the user can already reach.

## Findings

1. **Chart 0.24.0 ships operator 1.26.0, not 1.25.1.** `helm show chart cloudnative-pg --version 0.24.0` gives `appVersion: 1.26.0`. The old openfsc comment was already wrong, and this commit copies it into:
   - `plugins/internal/cnpg/cnpg.go` (the `Version` comment)
   - `plugins/cloudnativepg/README.md` (first bullet)
   - `console/clusters-body.js` ("Operator 1.25 runs PostgreSQL 13 to 17"; the conclusion still holds for 1.26)
   - `definition.yaml` `urls.documentation` (`/documentation/1.25/`)
   - the seed row's image (`cloudnative-pg:v1.25.1`)
   - the `helm_test.go` fixture (`appVersion: 1.25.1`)

2. **"Cannot delete, not even with kubectl" is stronger than what RBAC enforces.** The README Scope section, `roles.go` and `TestUserRolesGrantNoModification` ("through any path") all claim this. Project admins hold the built-in `admin` role in their namespaces, so they can delete PVC `<name>-1` together with pod `<name>-1`, which destroys a single-instance database. They can also edit or delete Secret `<name>-app`. And `create` through kubectl skips the form's limits entirely: `instances`, `imageName`, storage size, `bootstrap` and `backup` are all open, and only a ResourceQuota stops that. Either reword this as "no update/delete on the Cluster resource", or say what is and isn't protected.

3. **The recovery advice is wrong for a stuck first install.** `needsInstall` and the README say to run `helm -n cnpg-system rollback cnpg` for any `pending-*` state. For `pending-install` (revision 1, killed during `--wait`) there is no earlier revision to roll back to, so the fix is `helm -n cnpg-system uninstall cnpg`. A first install can also run long enough to be killed, because `exec.CommandContext` kills helm when Start's context is cancelled on pod shutdown. Tailor the message to the state: rollback for `pending-upgrade`/`pending-rollback`, uninstall for `pending-install`. That is safe there because no Clusters exist yet.

4. **With no default StorageClass, a database can end up stuck for good.** The form always preselects "Cluster default":
   - An org admin who can see the classes still gets "Cluster default" first even when none is marked default.
   - A project admin can pick nothing else.

   The PVC then stays Pending and the database shows "Provisioning" forever, and it can't be deleted from the console. The status hint "pick one when creating a database" only helps org admins. Suggestions:
   - When the classes are known and none is default, drop the "Cluster default" option (or make a choice required).
   - When the classes are unknown, offer a free-text StorageClass field.

5. **Every phase other than healthy counts as "Provisioning".** Both `uiHints.statusMapping` and `statusMessage` do this, so failure phases like "Unable to create required cluster objects" or an image pull that never succeeds read as "still starting". A database that can't be deleted and never gets ready looks like it's waiting. Consider "Not ready" as the default label and "not ready" in the summary, or map CNPG's known failure phases to `danger`.

6. **Nit: openfsc wraps the error twice.** `cnpg.Install` already returns `install cloudnative-pg: …`, and `openfsc/installer.go:83` wraps it again with the same prefix.
