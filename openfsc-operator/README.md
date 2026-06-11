# openfsc-operator

A standalone Kubernetes operator for [OpenFSC](https://fsc-standaard.nl)
(Federated Service Connectivity, the Dutch government standard for
inter-organisational API federation). It deploys and supervises a
self-contained FSC directory peer and its gateways through four cluster-scoped
CRDs in the `openfsc.fundament.io` group:

| Kind | Purpose |
| --- | --- |
| `Directory` | Deploys the OpenFSC core (Manager, Controller, auditlog, txlog-api) into `spec.namespace`, plus a self-signed group CA, the Manager's group certificate and a CloudNativePG cluster. The Manager functions as the group's Directory. |
| `Peer` | A member of the FSC group. The operator owns the `self` Peer representing the deployed directory and tracks its readiness. |
| `Inway` | A provider gateway. The operator mints its mTLS certificates and installs the vendored `open-fsc-inway` chart, one Helm release per resource. |
| `Outway` | A consumer gateway, provisioned the same way from `open-fsc-outway`. |

The operator embeds the digilab OpenFSC umbrella chart
([gitlab.com/digilab.overheid.nl/platform/helm-charts/open-fsc](https://gitlab.com/digilab.overheid.nl/platform/helm-charts/open-fsc),
version 1.43.0) and the inway/outway charts in its binary (`charts/`) and
installs them with the Helm SDK — releases land in regular Helm Secret storage,
fully visible to the `helm` CLI. No network fetch happens at runtime.

The operator is independent of Fundament: it has no plugin-sdk dependency and
can be installed on any cluster from its chart. The Fundament OpenFSC plugin
(`plugins/openfsc`) is a thin installer around it.

## Prerequisites

The operator depends on, but never installs, two other operators:

- [cert-manager](https://cert-manager.io) (`certificates.cert-manager.io`,
  `issuers.cert-manager.io`)
- [CloudNativePG](https://cloudnative-pg.io) (`clusters.postgresql.cnpg.io`)

A `Directory` reports the `PrerequisitesMet` condition as `False` with the
missing CRDs in its message until both are present; reconciliation resumes
automatically once they are installed. The CNPG cluster's StorageClass defaults
to `basic-csi` (`spec.postgres.storageClass`).

## Install

```sh
helm install openfsc-operator ./chart --namespace openfsc-system --create-namespace
```

The chart ships the CRDs in `crds/`, a scoped ClusterRole and a Deployment with
leader election enabled (`leaderElect`, `replicaCount` values). Then create a
directory:

```yaml
apiVersion: openfsc.fundament.io/v1
kind: Directory
metadata:
  name: default
spec:
  groupID: fsc-demo
  peerID: "12345678901234567899"
  namespace: fsc
```

and watch it come up:

```sh
kubectl get directory,peer,inway,outway
kubectl -n fsc get pods
```

`Inway`/`Outway` resources can be created at any time; the operator provisions
their certificates and workloads once the directory is up, and a finalizer
tears them back down on delete.

## Configuration

Pod environment (set via chart values):

| Variable | Default | Description |
| --- | --- | --- |
| `LEADER_ELECT` | `true` | leader election, for running multiple replicas |
| `LOG_LEVEL` | `info` | slog level: debug, info, warn, error |
| `METRICS_PORT` | `8080` | Prometheus metrics (`0` disables) |
| `HEALTH_PORT` | `8081` | `/livez` and `/readyz` probes |

## Layout

```
cmd/main.go            manager bootstrap
pkg/api/v1             CRD types (controller-gen: `go generate ./...`)
pkg/controller         Directory/Peer/Inway/Outway reconcilers
pkg/controllerclient   typed client for the Controller Administration API
pkg/helm               Helm SDK wrapper (embedded-chart installs)
charts/                vendored OpenFSC charts (go:embed)
chart/                 the operator's own Helm chart (CRDs in crds/)
```
