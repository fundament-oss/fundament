# #967 — Plugin proxy e2e against real local Gardener

Test plan for [issue #967](https://gitlab.com/digilab.overheid.nl/miscellaneous/issues/-/work_items/967).
Base: `feature/deploy-remote-stacked` (PR 314), rebased onto current master first —
PR #294 has merged as `77b78278`.

Scope is the **master-state model**: the plugin path flows through `kube-api-proxy`
(console assets and the `pluginmetadata.v1.PluginMetadataService` Connect RPC ride the
shoot apiserver service-proxy path), and a plugin-audience token on a cluster path is
rejected with 401. The FUN-17 gateway work (PR #297 public plugin-proxy, PR #310
kube-api-proxy scopes) is out of scope: #310's real mode is fail-closed stubs by design,
and #297's real mode targets a Service named `runtime` while plugin-controller creates
`plugin-<name>` — both get re-verified against this baseline when they land.

Round 1 runs on a Mac (local k3d + kind Gardener); round 2 repeats phases 2–5 on the
Hetzner box provisioned by `deploy-remote/` on this branch.

## Phase 1 — bring-up (issue item 1)

- [ ] `just cluster-start`
- [ ] `just cluster-worker gardener-up` (~10–15 min first run; watch `just cluster-worker gardener-status`)
- [ ] `just dev -p local-gardener`
- [ ] Log in as `alice@acme-corp.com` and create an API key (console or functl); put it
      in a gitignored env file in `e2e/terraform/` (`FUNDAMENT_API_KEY`)
- [ ] `terraform apply` the `e2e/terraform/` fixture (in-repo provider): creates the
      `acme-corp` cluster (`fundament_cluster`, built-in wait-for-running), does the
      shoot-side prep (PluginInstallation CRD + plugin-controller + image push — nothing
      on master puts these on a shoot), installs the cert-manager plugin
      (`fundament_plugin_installation`), and applies cert-manager's
      `test-resources.yaml` CRs (self-signed ClusterIssuer + Certificate → Ready) as the
      plugin smoke test
- [ ] Shoot reaches `ready` (`fundament_cluster` waits; cross-check `just cluster-worker shoots`)
- [ ] usersync has created SA `fundament-{userID}` in `fundament-system` on the shoot
      (kube-api-proxy returns 503 until then)

Known macOS fragilities: `local.gardener.cloud` resolver; after a Docker restart, stale
k3d ports/context (fix: `k3d kubeconfig merge`).

## Phase 2 — cluster connection (issue item 2)

- [ ] Obtain a user JWT for `alice@acme-corp.com` / `password` (Bruno
      `Authentication/Password-based login`, `functl cluster token`, or the e2e
      `exchangeToken` helper)
- [ ] `GET /clusters/{clusterID}/api/v1/namespaces` with `Authorization: Bearer <jwt>`
      → **200**, real shoot namespaces (expect `fundament-system` present)

## Phase 3 — plugin path (issue item 3)

- [ ] cert-manager plugin installed by the phase-1 Terraform apply (the
      `fundament_plugin_installation` CR goes through kube-api-proxy's cluster route —
      itself part of the surface under test) → plugin-controller creates
      `plugin-cert-manager` namespace/SA/RoleBinding/Deployment/Service on the shoot;
      pod passes `/readyz`
- [ ] Console asset, no auth:
      `GET /clusters/{id}/api/v1/namespaces/plugin-cert-manager/services/http:plugin-cert-manager:8080/proxy/console/index.html`
      → **200** HTML from the real pod
- [ ] Metadata RPC, with user JWT: Connect POST to
      `.../proxy/pluginmetadata.v1.PluginMetadataService/GetDefinition`
      → definition response from the real pod

## Phase 4 — permissions (issue item 4)

- [ ] User without `can_view` on the cluster (Dex static user outside acme-corp) → **403**
- [ ] Plugin-audience token (`aud=fundament-plugin`, via `MintPluginToken` or forged with
      `JWT_SECRET` as in `kube-api-proxy/pkg/proxy/server_test.go`) on
      `/clusters/{id}/...` → **401**
- [ ] Expired JWT → **401**; garbage JWT → **401**
- [ ] Sanity: non-UUID clusterID → **400**; non-`api|apis|openapi/` path → **404**

## Phase 5 — shoot-side RBAC (issue item 5)

The per-user SA is intentionally `cluster-admin` (the gate is OpenFGA `can_view`), so
"the SA's bound Role" means the **plugin SA**, bound to the built-in `admin` ClusterRole
namespaced to its own namespace.

- [ ] `kubectl auth can-i --as=system:serviceaccount:plugin-cert-manager:plugin-cert-manager
      get pods -n plugin-cert-manager` → **yes**
- [ ] Same SA, another namespace (`-n fundament-system`) → **no**
- [ ] Same SA, cluster-scoped resource (`get nodes`, `get clusterroles`) → **no**

## Phase 6 — teardown (issue item 6)

- [ ] `terraform destroy` (plugin installation + cluster), then `just cluster-worker gardener-down`
- [ ] Record each checkbox on the issue with the command + observed status code

## Round 2 — Hetzner

- [ ] Provision via `deploy-remote/hetzner.sh` (this branch); apply the `e2e/terraform/`
      fixture against the box
- [ ] Repeat phases 2–5; script the request list from round 1

## Candidate follow-ups (separate branches)

- `@real-gardener`-tagged e2e feature encoding phases 2–4 (adapt the step plumbing from
  PR #297's `e2e/steps/plugin-proxy.steps.ts`)
- Feedback on PR #297: `runtime:8080` vs `plugin-<name>` Service name; `local-gardener`
  skaffold profile will need `pluginProxy.mode: real` once real mode lands
