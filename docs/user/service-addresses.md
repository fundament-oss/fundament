---
title: Service IP addresses
sidebar:
  order: 7
---

A Kubernetes Service gives a set of pods one stable address. Which address that is, and who can reach it, depends on the type of the Service. This page explains where those addresses come from on a Fundament [cluster](./clusters.md), how to get one that is reachable from the internet, and how long you keep it.

## At a glance

| Service type | Address it gets | Reachable from |
| --- | --- | --- |
| `ClusterIP` (the default) | A cluster IP: a private address from the cluster's service range. | Inside the cluster only. |
| `NodePort` | A cluster IP, plus a port on every node. | Inside the cluster only. Nodes have private addresses, so a node port is not reachable from the internet. |
| `LoadBalancer` | A cluster IP, plus a public address from the installation's public range. | The internet, on the ports of the Service. |
| Headless (`clusterIP: None`) | None. Cluster DNS returns the addresses of the pods instead. | Inside the cluster only. |

All addresses are assigned automatically. You never pick one yourself, and nothing is reachable from the internet until you create a `LoadBalancer` Service or a Gateway.

## Cluster IPs

Every Service that is not headless gets a cluster IP. Kubernetes takes a free address from the cluster's service range when the Service is created, and the Service keeps it until it is deleted.

The service range is a private IPv4 range set by the platform operator; pods get their addresses from a second range of the same kind (see [ADR 0024](/adr/0024-internal-network-addressing)). Both ranges exist only inside the cluster. They are not routed outside it and clusters reuse the same ranges, so a cluster IP means nothing outside its own cluster, not even in another cluster of the same organization.

Inside the cluster, use the DNS name of the Service rather than its address: `<service>.<namespace>.svc.cluster.local`, where the namespace is [the name on the cluster](./namespaces.md#the-name-on-the-cluster).

## Public addresses

To make a workload reachable from outside the cluster, give its Service the type `LoadBalancer`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: tnt-payments--staging
spec:
  type: LoadBalancer
  selector:
    app: web
  ports:
    - port: 443
      targetPort: 8443
```

The platform then does three things:

1. It reserves a free address from the installation's public range for this Service.
2. The nodes of the cluster announce that address to the datacenter network, which from then on routes traffic for it straight to those nodes. There is no separate load balancer machine in between: the network spreads connections over the nodes, and each node forwards them to the pods of the Service.
3. The firewall in front of the cluster opens the TCP and UDP ports of the Service on that address. Everything else stays closed.

The address shows up in the `EXTERNAL-IP` column, usually within a few seconds, and answers from outside a few seconds after that. Until the address shows up, the column says `<pending>`:

```
$ kubectl get service web -n tnt-payments--staging
NAME   TYPE           CLUSTER-IP      EXTERNAL-IP    PORT(S)         AGE
web    LoadBalancer   10.248.17.203   203.0.113.27   443:31612/TCP   40s
```

The platform also writes the address into the `spec.loadBalancerIP` field of the Service.

Creating a Service takes the project admin role on the project that owns the namespace, or the organization admin role; see [What the kubeconfig grants](./clusters.md#what-the-kubeconfig-grants).

### Use few addresses

Every `LoadBalancer` Service gets an address of its own. Public addresses come from one range that all clusters of the installation share. They do not count against an organization [quota](./clusters.md#quotas), but the range is finite. When it has no free address left, a new Service stays at `<pending>`, and the events of the Service say why (`kubectl describe service`).

For HTTP and HTTPS, do not give every application its own `LoadBalancer` Service. Install a Gateway API [plugin](./plugins.md) and put a Gateway in front instead. A Gateway gets its public address in the same way, because the plugin creates a `LoadBalancer` Service for it; `kubectl get gateway` shows the address. Any number of hostnames and applications can then share that one address through routes, while their own Services stay plain `ClusterIP`.

### How long you keep an address

The address belongs to the Service, not to the pods or nodes behind it. It stays the same when pods restart, when you roll out a new version, and when node pools change or nodes are replaced.

The address is released when the Service is deleted. That includes the indirect ways of deleting a Service:

- deleting its namespace;
- uninstalling the Helm release it is part of;
- changing its type to something other than `LoadBalancer`;
- deleting the Gateway it was created for;
- deleting the cluster.

A released address goes back into the shared range and can be handed to anyone. Creating the Service again gives you a new address, which is almost never the old one. So when a DNS record points at a Service, change that Service in place rather than deleting and recreating it. A [lost cluster](./clusters.md#lost-clusters) that is created again also gets new addresses.

### Keeping an address across deletes

The platform has no self-service for reserving an address yet. If a name has to keep its address even when the Service behind it is replaced, contact a Fundament operator, who can reserve an address for you. Set that address in `spec.loadBalancerIP` and the Service uses it instead of a freshly assigned one:

```yaml
spec:
  type: LoadBalancer
  loadBalancerIP: 203.0.113.40
```

A reserved address is not released when the Service is deleted, so you can attach it to another Service later. Only an address that was reserved for you can be used this way.

### Restricting who can connect

By default the firewall accepts connections to the ports of the Service from any source address. To accept only certain sources, list them in `spec.loadBalancerSourceRanges`:

```yaml
spec:
  type: LoadBalancer
  loadBalancerSourceRanges:
    - 198.51.100.0/24
```

The firewall then drops connections from all other sources before they reach the cluster.

### Seeing the address of the client

`spec.externalTrafficPolicy` decides which nodes receive the traffic and whether your pods see the real address of the client:

| Value | Nodes that receive traffic | Source address the pod sees |
| --- | --- | --- |
| `Cluster` (the default) | All nodes. A node may forward a connection to a pod on another node. | The address of a node. |
| `Local` | Only nodes that run a ready pod of the Service. | The address of the client. |

Use `Local` when the application logs, rate limits or filters on the address of the client. Run more than one replica, spread over nodes, so the address stays reachable when a node goes away.

### DNS

The platform does not create DNS records for your Services. Point an `A` record at the public address with your own DNS provider.

## IPv6

Addresses inside the cluster are IPv4 only, as decided in [ADR 0024](/adr/0024-internal-network-addressing). IPv6 for public addresses is a separate decision that has not been made yet, so public addresses are IPv4 as well.

## Outbound traffic

The public address of a Service is for incoming connections only. Connections that your pods open to the internet leave through the firewall of the cluster and arrive from the address of that firewall, whichever Service the pod belongs to. That address is not reserved either: it can change when the platform replaces the firewall. Some installations translate it once more on the way out, to an address that all their clusters share. Ask the operator of your installation which address others will see before it goes on an allow list.
