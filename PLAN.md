# Extract a standalone `openfsc-operator`, slim the OpenFSC plugin to install/use it

## Context

The OpenFSC plugin on branch `939-openfsc-plugin-2nd-attempt` is the only Fundament plugin that embeds its own controller-runtime operator (Peer/Inway/Outway reconcilers, self-applied CRDs, helm-exec installs) inside the plugin pod. Every other plugin (`external-dns`, `cert-manager`) is a thin installer around a real operator. Embedding means reconciliation pauses whenever the plugin pod is down, RBAC is the union of everything, leader election/HA/webhooks are unavailable, and the operator can't be reused outside Fundament (FSC is a Dutch government standard — a standalone operator has ecosystem value).

Goal: build a standalone `openfsc-operator` (kubebuilder-style, owns the CRDs plus a new top-level `Directory` CR that deploys the OpenFSC core), and slim the plugin to the external-dns pattern: install operator + prerequisites, create the Directory CR from config, surface status in the console.

**Branching**: new branch `openfsc-operator` from `master`. `master` contains no OpenFSC code at all — the entire plugin (~6.5k lines) lives only on `939-openfsc-plugin-2nd-attempt`. Port files via `git checkout 939-openfsc-plugin-2nd-attempt -- plugins/openfsc/<path>` (then move/adapt), so the two branches can be compared side by side.

## Key decisions

1. **Layout**: top-level `openfsc-operator/` mirroring `plugin-controller/` (`cmd/main.go` + `pkg/...` + `Dockerfile` + own helm chart). Same root Go module `github.com/fundament-oss/fundament` (single `go.mod`, no `go.work` — verified). Standalone-ness comes from no `plugin-sdk` imports + own image/chart, not module separation.
2. **API**: keep `openfsc.fundament.io` group and the Peer/Inway/Outway CRDs (console assets, `definition.yaml`, finalizer `openfsc.fundament.io/gateway` all reference it; rename is a possible upstream follow-up). Add a cluster-scoped `Directory` CR:
   - Spec: `GroupID`, `PeerID`, `Namespace` (default `fsc`), `ControllerURL`, `Postgres{Instances,Image,StorageClass,StorageSize}`, `AutoSignGrants []string` — derived from current `installer.go` + `values-fundament.yaml` + `config.go` (`FUNP_*`).
   - Status: `Phase`, `Message`, `ControllerURL`, `ObservedGeneration`, `Conditions` (`Ready`, `PrerequisitesMet`).
   - DirectoryReconciler also owns the `self` Peer (ports `seedSelfPeer` + readiness from Manager/Controller Deployments). Controller admin address/serverName/cert-secret are derived from the Directory (operator installs the umbrella, so it knows the names) — the `FUNP_CONTROLLER_*` env knobs disappear from the plugin.
3. **Deployment mechanism**: Helm SDK (`helm.sh/helm/v3` as library) for the vendored umbrella `open-fsc-1.43.0.tgz` and the per-gateway `open-fsc-inway`/`open-fsc-outway` charts (charts `go:embed`-ded); native server-side apply (field owner `openfsc-operator`) for the small `openfsc-directory` helper resources (group CA Issuer/Certificates + CNPG Cluster). **Decision point in Phase 0**: if `helm.sh/helm/v3` won't resolve against root `k8s.io v0.35.x` / `controller-runtime v0.23.3`, fall back to exec-helm inside the operator (port `runHelm`/`helmEnv` verbatim, alpine image) — rest of plan unchanged.
4. **Prerequisites**: operator never installs other operators. It preflights the cert-manager + CNPG CRDs and sets `PrerequisitesMet=False` with a clear message. The plugin keeps installing cert-manager, CloudNativePG, and the `basic-csi` StorageClass (sandbox concern), as today.
5. **Packaging**: operator chart at `openfsc-operator/chart/` with the 4 CRDs in `crds/` (no more runtime `applyCRDs`), Deployment (leader election **on**), ServiceAccount, scoped ClusterRole. CI: add `openfsc-operator` to the image matrix in `.github/workflows/build.yml` (same pattern as other images). Sandbox: operator chart is `COPY`-ed into the plugin image so the plugin installs it offline; operator image ref passed via `PluginInstallation.spec.config` → `FUNP_OPERATOR_IMAGE` (config→env mapping verified in `plugin-controller/pkg/controller/resources.go:139-147`); new `plugins/openfsc/Justfile` recipe `operator-push` builds/pushes the operator image to `localhost:5112`.
6. **Slimmed plugin** (modeled on `plugins/cert-manager/plugin.go`):
   - `Install`: prereqs → install operator chart (via existing `plugin-sdk/pluginruntime/helpers/helm`) into `openfsc-system` → wait CRDs Established → create/update `Directory` `default` from `FUNP_*` → seed `default-inway`/`default-outway`.
   - `Reconcile`: `crd.VerifyAll` + map Directory status → `host.ReportStatus`.
   - `Uninstall`: delete Directory + gateway CRs (operator finalizers tear down), uninstall operator release; leave prereqs.
   - Console assets, `console.go`, `scripts/console-preview/`, definition uiHints: unchanged (add `directories` to `crds:`/`allowedResources`). `config.go` shrinks to `GroupID`, `DirectoryPeerID`, `Namespace`, `ControllerURL`, `OperatorImage`.

## New layout

```
openfsc-operator/
  README.md  Dockerfile
  cmd/main.go                    # bootstrap cloned from plugin-controller/cmd/main.go
  pkg/
    api/v1/                      # ported types.go + new Directory; controller-gen regen
    controller/                  # directory.go, directoryresources.go (SSA), peer.go,
                                 # gateway_inway.go, gateway_outway.go, certs.go,
                                 # gatewayvalues.go, status.go, adminclient.go
    controllerclient/client.go   # ported verbatim
    helm/helm.go                 # Helm SDK wrapper (or exec fallback)
  charts/                        # go:embed: open-fsc-1.43.0.tgz, inway/, outway/, values-fundament.yaml
  chart/                         # operator's own chart (crds/ + templates/)
plugins/openfsc/                 # thin: main.go plugin.go config.go console.go installer.go
                                 # definition.yaml Dockerfile Justfile console/ scripts/
```

## Porting map (from `939-openfsc-plugin-2nd-attempt:plugins/openfsc/`)

- **Near-verbatim**: `api/v1/*` (+ Directory types, regen with controller-gen v0.20.1 per mise.toml), `controllerclient/client.go`, reconcilers from `controller.go`, cert provisioning + values maps from `gateways.go`, lazy admin client + `readCertSecret` + `waitEstablished` from `operator.go`, all vendored charts, `console/`, `scripts/console-preview/`, `definition.yaml`, `Justfile`, `main.go`.
- **Rewritten**: `operator.go` → `openfsc-operator/cmd/main.go`; `installer.go` split (prereqs → plugin; directory helper chart → SSA in DirectoryReconciler; umbrella → DirectoryReconciler helm); `plugin.go`/`config.go` → thin versions; `charts/openfsc-directory/` templates → Go objects in `directoryresources.go`.

## Phases (each ends verifiable in sandbox)

- **Phase 0 — skeleton**: branch from master; scaffold operator with no-op reconcilers, regenerated CRDs, chart, Dockerfile; resolve the helm-SDK-vs-exec decision point. Verify: `go build ./openfsc-operator/...`; helm-install the chart on the sandbox cluster; pod Running, lease held, 4 CRDs Established.
- **Phase 1 — DirectoryReconciler**: prereq preflight condition; SSA directory resources (content from `charts/openfsc-directory/templates/`); umbrella install (`shared`, `fullnameOverride=shared`); own the `self` Peer; finalizer teardown. Verify: hand-install prereqs (commands from `installer.go:95-130`), apply Directory CR → `directory/default` Active, `peer/self` Active, `kubectl -n fsc get pods` shows manager/controller/auditlog/txlog/postgres.
- **Phase 2 — gateway reconcilers**: port Inway/Outway reconcilers + cert provisioning + per-CR chart installs + admin-API registration (addresses derived from Directory). Verify: apply Inway+Outway CRs → Active; delete → release + certs removed via finalizer.
- **Phase 3 — slimmed plugin**: thin plugin.go/config.go/installer.go; plugin Dockerfile embeds operator chart; `FUNP_OPERATOR_IMAGE` plumbing + `operator-push` recipe; update `just openfsc test` to wait on directory/peer/gateways. Verify end-to-end: fresh cluster → `just plugin-install openfsc` → `just openfsc test` → `just openfsc console-preview`.
- **Phase 4 — polish**: CI matrix entry, operator README (standalone usage + prereq preflight), `definition_test.go`, lint/tests, `plugins/README.md`.

## Verification

1. `cd plugins && just cluster-create && just deploy`.
2. `just openfsc::operator-push` → `just plugin-install openfsc` (config carries `OPERATOR_IMAGE`).
3. `just openfsc::test`: PluginInstallation Running; `kubectl get directory,peer,inway,outway` all Active; `kubectl -n fsc get pods` complete.
4. Negative checks: delete `inway/default-inway` (finalizer teardown works); kill operator pod (re-elects, plugin unaffected); `plugin-uninstall openfsc` leaves cert-manager/CNPG but removes Directory, gateways, operator.

## Critical reference files

- `plugins/openfsc/installer.go`, `operator.go`, `controller.go`, `gateways.go` (on branch `939-openfsc-plugin-2nd-attempt`) — source of ported logic
- `plugin-controller/cmd/main.go` — operator bootstrap template & repo conventions
- `plugins/cert-manager/plugin.go` — target shape for the thin plugin
- `plugin-controller/pkg/controller/resources.go` — config→`FUNP_*` env mapping
- `plugins/Justfile`, `plugins/openfsc/Justfile` — sandbox flow every phase verifies against
