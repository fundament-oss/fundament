---
title: Pod security
sidebar:
  order: 12
---

Fundament runs some workloads on each cluster. This page lists the namespaces
of these workloads and the
[Pod Security Standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
level that each namespace meets. Use this list when you enforce a level with
your own admission policies, for example with Kyverno or Gatekeeper.

## Namespaces

| Namespace | Managed by | Level | Reason |
|---|---|---|---|
| `fundament-system` | Fundament | `restricted` | |
| `plugin-<organization>--<plugin>` | Fundament (plugin-controller) | `restricted` | |
| `firewall` | Infrastructure | `restricted` | |
| `metallb-system` | Infrastructure | `privileged` | The MetalLB speaker needs the `NET_RAW` capability to announce load balancer IPs. |
| `csi-lvm` | Infrastructure | `privileged` | The `csi-lvm-reviver` DaemonSet is privileged and mounts `/dev` and LVM paths of the node to manage local volumes. |
| `kube-system` | Infrastructure | `privileged` | Calico, kube-proxy and the API server proxy use the host network and host paths. |

Fundament labels each plugin namespace with
`pod-security.kubernetes.io/enforce: restricted`. The API server then refuses a
plugin pod that does not meet this level.

## Namespaces of plugins

A plugin installs its components in its own namespaces, for example
`cert-manager` or `rook-ceph`. The level of these namespaces depends on the
plugin. For example, the Ceph Storage plugin needs `privileged` in `rook-ceph`,
because Ceph mounts host paths and devices of the node. Read the documentation
of the plugin before you install it on a cluster with strict policies.

## Your own admission policies

If your policies enforce `restricted`, exclude only the namespaces with the
level `privileged` in the table above. Also exclude the namespaces of plugins
that need more than `restricted`.

Do not exclude `fundament-system` or the plugin namespaces. Their workloads
meet `restricted`. An exclusion turns off all policies for a namespace, so you
lose these checks without a reason.

Do not change the labels of the infrastructure namespaces. The infrastructure
sets them again at the next reconcile.
