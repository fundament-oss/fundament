---
title: Preview environments
sidebar:
  label: Preview environments
  order: 3
---

A pull request gets its own Fundament deployment on digikluster at
`https://<service>.pr<N>.preview.fundament.digilab.reviews`. The PR comment "PR Environment"
lists the URLs and the test users.

## Which pull requests get one

**Gets a preview**

- An open pull request from a branch in this repository, a few minutes after `publish-chart`
  finishes.

**Gets no preview**

- A draft. Mark it ready for review to get one.
- A pull request labelled `no-preview-env`. Remove the label to get one.
- A pull request from a fork. Its CI runs without write access, so it cannot publish the
  images and chart a preview installs, and code from outside never reaches the cluster
  unreviewed. To preview a fork's change, a maintainer reviews it and pushes it to a branch
  in this repository.
- A closed or merged pull request. Its preview is removed within a minute.

A pull request has a preview exactly while it carries the label `flux-preview`, which CI adds
and removes.

## What it runs

- The chart this PR built: `publish-chart` pushes
  `oci://ghcr.io/fundament-oss/fundament/charts-pr/fundament:0.1.0-pr<N>.<run>` with every
  image pinned by digest. The preview follows the newest build of the PR.
- `values.yaml` and `values-sandbox.yaml` from that chart, plus the PR's hostnames.
- The database is reset and refilled with test data on every push.
- `e2e-tests` and `terraform-acc-tests` run against the preview once `/version` on
  `organization.pr<N>…` reports the chart version.

## Adding a service

1. Add the image to the `build` matrix in `.github/workflows/build.yml` and to `images:` in
   `values.yaml`. `publish-chart` fails for an image in `images:` that has no build.
2. Add the chart templates: Deployment, Service, and `<service>-httproute.yaml` if it is
   reachable from outside.
3. Set `enabled: true` for it in `values-sandbox.yaml` so previews run it.
4. **Outside this repository:** the preview's hostnames live in the platform repository
   `flux/core`, in `tenants/digikluster/fundament-preview/resourceset.yaml`. The new service's
   `httproute` (`enabled`, `parentRefs`, `hostnames`), its `externalUrls` entry and, if
   browsers call it cross-origin, `corsAllowedOrigins` go there. Ask the platform team; until
   that change is merged the service runs in the preview without a URL.
5. The preview is namespace admin: Kubernetes' built-in `admin` role, plus HTTPRoutes,
   GRPCRoutes, CNPG clusters and ServiceMonitors. So NetworkPolicies, roles and the usual
   workload kinds ship with the chart. The platform team is needed for other CRDs,
   cluster-scoped objects, privileged pods (the namespace enforces pod security `baseline`),
   and for ResourceQuota, LimitRange, Endpoints and EndpointSlices, which stay read-only.

> **TODO:** derive every hostname, URL and route from one `domain` value in the chart, so
> step 4 disappears and a new service needs no change outside this repository. Tracked in
> [digilab.overheid.nl/miscellaneous/issues#1157](https://gitlab.com/digilab.overheid.nl/miscellaneous/issues/-/work_items/1157).

## Debugging

With a digikluster kubeconfig (see the team's deployment and infra docs):

```bash
export NS=tn-fundament-preview-<N>
kubectl -n $NS get helmrelease,helmchart,pods
kubectl -n $NS describe helmrelease fundament
kubectl -n $NS get events --sort-by=.lastTimestamp
kubectl -n $NS logs deploy/organization-api
```

- `helmchart` not ready: `publish-chart` has not pushed a chart for this PR yet.
- Namespace missing: check the PR carries `flux-preview`, then
  `kubectl -n tn-fundament-preview get resourcesetinputprovider fundament-pull-requests -o jsonpath='{.status.exportedInputs}'`.
- Migrations run as Job `db-migrations-<helm-revision>`; OpenFGA's store and model as
  `openfga-bootstrap-<helm-revision>`.
