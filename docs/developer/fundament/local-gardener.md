---
title: Running with a local Gardener
sidebar:
  label: Local Gardener
  order: 2
---

`just dev-hotreload` runs the platform in mock mode: a cluster you create exists only as a row in the database. With a local Gardener, creating a cluster makes Gardener build a real Kubernetes cluster on your machine.

Gardener terms used on this page:

- **Shoot**: a Kubernetes cluster that Gardener manages. Every cluster created in Fundament is a shoot.
- **Virtual garden**: Gardener's API, where projects and shoots are created. The platform talks to it with a kubeconfig.
- **Seed**: the cluster that runs each shoot's control plane (API server, etcd) as pods.

Locally, the platform runs in a k3d cluster and all of Gardener in one kind cluster next to it:

```
 k3d-fundament: the platform               kind gardener-operator-local: Gardener
┌──────────────────────────────┐          ┌──────────────────────────────────────┐
│ organization-api             │          │ virtual garden   projects, shoots    │
│ cluster-worker ──────────────┼─────────►│ seed             shoot control planes│
│ kube-api-proxy               │kubeconfig│ shoot nodes      run as pods too     │
└──────────────────────────────┘          └──────────────────────────────────────┘
        just cluster-worker gardener-connect joins the two clusters' networks
```

This differs from a real installation in three ways:

- Gardener's API, the seed and the shoots' nodes share one cluster on your machine. An installation can spread them over several clusters.
- A shoot's nodes are pods inside the kind cluster, not machines. Clusters are created and deleted in minutes, and nothing outside your machine is touched.
- The platform reaches Gardener only after `gardener-connect` has joined the networks of the two clusters. In an installation it calls Gardener's API like any other endpoint.

Before you start:

- Install the tools from [Getting started](./getting-started.md), and run the `just` commands below from the fundament repository root.
- `gardener-start` clones Gardener, at the version pinned as `GARDENER_VERSION` in `cluster-worker/mod.just`, into `~/.cache/fundament/gardener` on Linux or `~/Library/Caches/fundament/gardener` on macOS. Set `GARDENER_DIR` to use another directory.

- Give Docker at least 8 CPUs, 8 GiB of memory and 120 GiB of disk for Gardener, on top of what the platform uses. Each shoot needs more.

## Requirements

### Linux

Tested on Ubuntu 24.04 with Docker from the Ubuntu archive.

- Docker Compose and Buildx, which Ubuntu's `docker.io` package does not include:

  ```shell
  sudo apt install docker-compose-v2 docker-buildx
  ```

- `gardener-start` asks for your `sudo` password to add `registry.local.gardener.cloud` to `/etc/hosts` and IP addresses to the loopback interface `lo`. Run it in a terminal where it can ask.
- Raise the limits on file watches, or the clusters do not come up. A reboot resets them:

  ```shell
  sudo sysctl -w fs.inotify.max_user_instances=8192 fs.inotify.max_user_watches=524288
  ```

- If your machine uses systemd-resolved (Ubuntu does), create this directory before `gardener-start`. Gardener puts its DNS settings for `*.local.gardener.cloud` there, and skips that step when the directory is missing:

  ```shell
  sudo mkdir -p /etc/systemd/resolved.conf.d
  ```


### macOS

This guide has not been tested on macOS. Gardener is reported to work with Docker Desktop, and does not work with Colima.

- Docker Desktop with the resources above.
- The GNU versions of `sed` and `tar` plus `iproute2mac`, which Gardener's scripts need: `brew install gnu-sed gnu-tar iproute2mac`, with the GNU tools first in your `PATH`.
- `gardener-start` asks for your `sudo` password to edit `/etc/hosts` and `/etc/resolver/local.gardener.cloud`.

## 1. Start the platform cluster

```shell
just cluster-start
```

This creates the platform's cluster `k3d-fundament`, or starts it if it exists. Gardener connects to it in the next step, which stops right away if the cluster is not running.

## 2. Start Gardener

:::note[This takes a while]
The first run takes about 15 minutes, with long stretches where nothing new is printed. Messages such as `seeds.core.gardener.cloud "local" not found` while it waits are normal. Let it run until it prints `Gardener started! (operator path)`; if it stops with an error before that, run it again.
:::

```shell
just cluster-worker gardener-start
```

- The first run clones Gardener, creates the kind cluster and installs Gardener in it. Later runs only reconnect the two clusters.
- At the end it joins the clusters' networks and DNS (`gardener-connect`) and gives the platform its kubeconfig for Gardener, the Secret `gardener-kubeconfig` (`gardener-secret`).
- `gardener-connect` also adds two contexts to your kubeconfig, next to `k3d-fundament`; see [Reach the clusters](#5-reach-the-clusters).

Gardener normally publishes SSH access to shoot nodes (bastions) on port 22, which clashes with an SSH server on your machine. `gardener-start` moves it to port 2222.

```shell
just cluster-worker gardener-status
```

About a minute after `gardener-start` finishes, the kind cluster restarts its control plane once, and `gardener-status` shows `(garden not found)` for that minute. Continue when it lists `local` with `Succeeded`.

## 3. Deploy the platform

```shell
just cluster-worker dev
```

- It runs `just dev-hotreload -p local-gardener` and stops right away if Gardener is not running. The `local-gardener` profile is real mode: cluster-worker, kube-api-proxy and plugin-proxy talk to Gardener instead of mocks, and organization-api reads metrics and logs from the shoots. A plain `just dev-hotreload` deploys mock mode again.
- It stays attached and syncs your edits into the running services. Adding, moving or deleting a file redeploys.

:::caution[A redeploy deletes your clusters]
Every deploy resets the databases. cluster-worker then finds shoots without a cluster in the database and deletes them. While you have clusters you want to keep, stop it with `Ctrl-C` once the deploy is done. See [Change a service without a redeploy](#change-a-service-without-a-redeploy).
:::
- The profile turns on immediate cluster deletion (`clusterWorker.gardenerImmediateClusterDeletion`): deleting a cluster does not wait for what runs in it to clean up. To see how a deletion behaves without it, turn it off without redeploying:

  ```shell
  kubectl --context k3d-fundament -n fundament set env deploy/cluster-worker GARDENER_IMMEDIATE_CLUSTER_DELETION=false
  ```

## 4. Create a cluster

In the console, <https://console.fundament.localhost:8443>, create a cluster in region `local`.

Or over the API, logged in as one of the local test users:

```shell
curl -s https://authn.fundament.localhost:8443/login/password -H 'Content-Type: application/json' -d '{"email":"alice@acme-corp.com","password":"password"}'
```

```shell
curl -s https://organization.fundament.localhost:8443/organization.v1.ClusterService/CreateCluster -H "Authorization: Bearer $TOKEN" -H 'Fun-Organization: 019b4000-0000-7000-8000-000000000001' -H 'Content-Type: application/json' -d '{"name":"demo","region":"local","kubernetesVersion":"1.33.0"}'
```

- Set `$TOKEN` to `access_token` from the login response. `Fun-Organization` is one of the IDs in its `user.organization_ids`.
- A shoot takes about 5 minutes to reach `Create Succeeded (100%)`.

## 5. Reach the clusters

To follow progress, each in its own terminal; both keep running until you stop them with Ctrl-C:

```shell
just cluster-worker shoots
```

```shell
just cluster-worker logs
```

- `shoots` lists every shoot and adds a line whenever one changes.
- `logs` follows cluster-worker's log.

A new shoot takes a few minutes before every health check is green: its first etcd backup and its monitoring start after the cluster itself.

`gardener-connect` adds two contexts to your kubeconfig. The current context stays `k3d-fundament`, so name the context in every command:

| Context | Cluster |
|---|---|
| `kind-gardener-local-garden` | virtual garden: projects and shoots |
| `kind-gardener-operator-local` | the kind cluster, including the seed with one `shoot--<project>--<name>` namespace per shoot |

```shell
kubectl --context kind-gardener-local-garden get shoots -A
```

To work inside a shoot, request an admin kubeconfig for it, valid for a day:

```shell
echo '{"apiVersion":"authentication.gardener.cloud/v1alpha1","kind":"AdminKubeconfigRequest","spec":{"expirationSeconds":86400}}' | kubectl --context kind-gardener-local-garden create --raw /apis/core.gardener.cloud/v1beta1/namespaces/<project-namespace>/shoots/<shoot>/adminkubeconfig -f - | jq -r .status.kubeconfig | base64 -d > shoot.kubeconfig
```

## Metrics and logs

The `local-gardener` profile reads each shoot's own Prometheus and Vali in the seed, as an installation does (`organizationApi.prometheusURL` and `organizationApi.logsURL` set to `per-shoot`). The console then shows a cluster's resource usage and logs without "Mock" labels.

- They arrive once the shoot's monitoring runs, a few minutes after the cluster itself.
- CPU and memory totals are the kind node's, because a shoot's nodes are pods on it.
- Mock mode generates metrics and logs instead.

## Change a service without a redeploy

To replace a service's image without a deploy, so your shoots stay, build it and set the image:

```shell
skaffold build --kube-context k3d-fundament --profile env-local -p local-gardener --default-repo=localhost:5111 -b cluster-worker -q
```

```shell
kubectl --context k3d-fundament -n fundament set image deploy/cluster-worker cluster-worker=<tag from the build>
```

To change a setting, use `kubectl --context k3d-fundament -n fundament set env deploy/<service> NAME=value`.

## After a reboot

A reboot removes the IP addresses on `lo` and resets the file-watch limits. Raise the limits again (see Linux above), then:

```shell
just cluster-worker gardener-connect
```

It restores the addresses, asking for `sudo`, and reconnects the two clusters.

## Remove

```shell
just cluster-worker gardener-delete
```

This deletes the kind cluster, the registry and DNS containers Gardener runs next to it, the Gardener clone and the two kubeconfig contexts. The changes from Requirements stay: the `/etc/hosts` entries, the DNS settings and the addresses on `lo`.

## Too small a machine

`deploy-remote/README.md` describes running the same setup on a rented server.
