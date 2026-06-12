# openfsc-operator

Kubernetes operator for [OpenFSC](https://fsc-standaard.nl) (Federated Service
Connectivity, the Dutch government FSC standard). It owns one namespaced CRD:

**FSCInstallation** (`openfsc.fundament.io/v1`, `kubectl get fsci`) — one
resource per team namespace turns that namespace into an FSC peer. The operator
installs there:

- the OpenFSC core from the vendored digilab umbrella chart (Manager,
  Controller, auditlog, txlog-api) as Helm release `fsc`,
- a CloudNativePG cluster backing those components,
- one gateway workload per declared `spec.inways[]` / `spec.outways[]` entry
  (Helm releases `fsc-inway-<name>` / `fsc-outway-<name>` from the vendored
  inway/outway charts), and
- the certificates each piece needs.

The directory mode decides where the FSC group lives:

- `directory.mode: Self` — this installation's Manager acts as the group's
  Directory. The operator self-signs a group CA and mints all group
  certificates (the peer ID becomes the certificate subject serialNumber).
  Self-contained; this is the local-development shape, and other installations
  can join the group via External mode pointed at `status.managerAddress`.
- `directory.mode: External` — the installation joins an existing group: the
  spec carries the directory's address and peer ID, a trust-anchor Secret with
  the group CA, and the team's organization certificate as a
  `kubernetes.io/tls` Secret reference.

See `config/samples/openfsc_v1_fscinstallation.yaml` for the canonical Self
mode example.

## Behavior

- One FSCInstallation per namespace: every component/release name in the
  namespace is fixed (prefix `fsc`), so a second resource reports
  `Error: namespace already hosts FSCInstallation <name>` until the first is
  gone (oldest wins). The namespace itself is never created or deleted by the
  operator — it belongs to the team.
- Prerequisites: cert-manager and CloudNativePG must be installed (the
  Fundament OpenFSC plugin does this). The operator never installs them; it
  reports `PrerequisitesMet=False` while their CRDs are missing.
- Status: `phase` is `Active` once the Manager and Controller Deployments are
  Available and every declared gateway has registered with the Controller
  Administration API; per-gateway state lives in `status.inways[]` /
  `status.outways[]`. Conditions: `PrerequisitesMet`, `CertificatesReady`,
  `CoreDeployed`, `Ready`.
- Removing a gateway entry from the spec uninstalls its release and
  certificates (orphan sweep); deleting the resource tears down everything the
  operator provisioned via a finalizer, leaving the namespace in place.
- Helm releases use the default Secret storage driver, fully visible to the
  `helm` CLI.

## Install

The Fundament OpenFSC plugin installs this operator from `chart/` (the CRD
ships in `chart/crds/`). Manual install:

```shell
helm upgrade --install openfsc-operator ./chart \
  --namespace openfsc-system --create-namespace \
  --set image.repository=...,image.tag=...
```

Pod configuration (env): `LEADER_ELECT` (default true), `LOG_LEVEL` (debug,
info, warn, error), `METRICS_PORT` (8080, 0 disables), `HEALTH_PORT` (8081).

## Layout

Scaffolded with the Operator SDK (go/v4 plugin), with one deviation: the repo
is a single Go module, so the scaffold's own `go.mod`, lint config and Makefile
were dropped — the operator builds as packages of the root module and
generation runs through the repo-standard `go generate ./...` (controller-gen
writes the CRD to `chart/crds/` and the RBAC reference to `config/rbac/`).
`PROJECT` is kept so future `operator-sdk create api` runs still work.

- `api/v1/` — the FSCInstallation types (CEL validation rules included; see
  `fscinstallation_validation_test.go` for the envtest suite that pins them).
- `internal/controller/` — the reconciler. `naming.go` is the single source of
  every derived name; `corevalues.go`/`gatewayvalues.go` build the Helm values
  (render-tested against the vendored charts in `corevalues_test.go`).
- `internal/helm/` — Helm SDK wrapper (install/upgrade/uninstall/list).
- `internal/controllerclient/` — typed client for the Controller
  Administration API (mTLS), used to observe gateway registrations.
- `charts/` — vendored charts, embedded in the binary: the digilab umbrella
  (`open-fsc-2.4.0.tgz` from
  gitlab.com/digilab.overheid.nl/platform/helm-charts/open-fsc) and the
  open-fsc-inway/outway charts (2.4.0, from
  `oci://registry-1.docker.io/federatedserviceconnectivity`). No network
  fetches at runtime.
- `chart/` — the operator's own Helm chart (Deployment, scoped ClusterRole,
  leader-election Role, CRDs).

## Testing

```shell
go test ./openfsc-operator/...   # unit + render tests; envtest CEL suite
                                 # (skips unless setup-envtest assets exist:
                                 #  go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.23 use)
```

End-to-end happens in the plugin sandbox (`plugins/`): `just openfsc
operator-push`, `just plugin-install openfsc`, `just openfsc test`. External
mode is covered by validation and values tests only; a full two-group
federation test needs a second group and is out of scope here.
