# Plugin Controller

The plugin controller manages the lifecycle of Fundament plugins. It watches `PluginInstallation` custom resources and creates the necessary Kubernetes resources to run each plugin in its own isolated namespace.

## Architecture Overview

```
                                    Fundament Namespace
                                   ┌─────────────────────────────────────────┐
                                   │                                         │
  kubectl apply                    │  PluginInstallation CR                  │
  install.yaml ──────────────────► │  (name: cert-manager-test)              │
                                   │                                         │
                                   │  Plugin Controller                      │
                                   │  ┌───────────────────────────────────┐  │
                                   │  │ watches PluginInstallation CRs    │  │
                                   │  │ creates child resources           │  │
                                   │  │ polls plugin status via metadata  │  │
                                   │  │ API and updates CR status         │  │
                                   │  └──────────────┬────────────────────┘  │
                                   └─────────────────┼───────────────────────┘
                                                     │ creates
                                                     ▼
                                    Plugin Namespace (plugin-cert-manager-test)
                                   ┌─────────────────────────────────────────┐
                                   │  Namespace                              │
                                   │  ServiceAccount                         │
                                   │  RoleBinding ──► ClusterRole/admin      │
                                   │  Deployment (plugin image)              │
                                   │  Service (:8080)                        │
                                   └─────────────────────────────────────────┘
```

## Namespace Isolation

Each plugin runs in its own namespace named `plugin-{pluginName}`. This provides:

- **Resource isolation** -- plugins cannot interfere with each other
- **Simple cleanup** -- deleting the namespace cascades to all resources within it
- **Default RBAC scoping** -- plugins get `admin` within their own namespace by default

## RBAC Model

```
                    ┌─────────────────────────────────────────────┐
                    │            Default (always created)          │
                    │                                             │
                    │  RoleBinding (in plugin namespace)          │
                    │  ├─ binds: ServiceAccount/plugin-{name}     │
                    │  └─ to:    ClusterRole/admin                │
                    │                                             │
                    │  Grants: full admin within the plugin's     │
                    │  own namespace (pods, services, secrets,    │
                    │  configmaps, deployments, etc.)             │
                    └─────────────────────────────────────────────┘

                    ┌─────────────────────────────────────────────┐
                    │         Optional (via spec.clusterRoles)     │
                    │                                             │
                    │  ClusterRoleBinding                         │
                    │  ├─ binds: ServiceAccount/plugin-{name}     │
                    │  └─ to:    ClusterRole/{requested-role}     │
                    │                                             │
                    │  Grants: cluster-wide permissions.          │
                    │  Used by plugins that need cross-namespace  │
                    │  access (e.g. installing Helm charts that   │
                    │  create CRDs, webhooks, or resources in     │
                    │  other namespaces).                         │
                    └─────────────────────────────────────────────┘
```

A plugin that only needs to manage resources within its own namespace requires no extra configuration. Plugins that need broader access (like cert-manager, which installs CRDs, webhooks, and resources across namespaces) request specific ClusterRoles:

```yaml
spec:
  clusterRoles:
    - cluster-admin
```

## Reconciliation Flow

```
  PluginInstallation created/updated
              │
              ▼
  ┌─ Ensure finalizer on CR
  │
  ├─ Create/update Namespace (plugin-{name})
  │
  ├─ Create/update ServiceAccount
  │
  ├─ Create/update RoleBinding ──► admin
  │
  ├─ Create/update ClusterRoleBindings (if spec.clusterRoles set)
  │
  ├─ Create/update Deployment (plugin image + env vars)
  │
  ├─ Create/update Service (:8080)
  │
  ├─ Poll plugin metadata API for status
  │     GET http://plugin-{name}.plugin-{name}.svc.cluster.local:8080
  │     └─ ConnectRPC: PluginMetadataService.GetStatus
  │
  └─ Update CR .status (phase, ready, message, pluginVersion)
              │
              ▼
  RequeueAfter (status poll interval)
```

## Deletion Flow

```
  PluginInstallation deleted
              │
              ▼
  ┌─ Finalizer triggers cleanup
  │
  ├─ Delete ClusterRoleBindings (if any)
  │
  ├─ Delete Namespace
  │     └─ cascades to: SA, RoleBinding, Deployment, Service
  │
  └─ Remove finalizer from CR
              │
              ▼
        CR is garbage collected
```

## PluginInstallation CRD

```yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: cert-manager-test
  namespace: fundament
spec:
  # Required
  image: ghcr.io/fundament-oss/fundament/cert-manager-plugin:latest
  pluginName: cert-manager-test
  version: v1.17.2

  # Optional: ClusterRoles to bind at cluster scope
  clusterRoles:
    - cluster-admin

  # Optional: extra environment variables for the plugin container
  config:
    LOG_LEVEL: debug
```

### Status

The controller polls the plugin's metadata API and writes status back to the CR:

| Field              | Description                               |
|--------------------|-------------------------------------------|
| `phase`            | `Pending`, `Deploying`, `Running`, `Degraded`, `Failed`, `Terminating` |
| `ready`            | `true` when phase is `Running`            |
| `message`          | Human-readable status message from plugin |
| `pluginVersion`    | Version reported by the plugin            |
| `observedGeneration` | Last spec generation processed          |

## Plugin SDK Integration

The plugin controller works with the [plugin-sdk](../plugin-sdk/). Each plugin binary:

1. Implements the `pluginsdk.Plugin` interface
2. Calls `pluginsdk.Run(plugin)` which starts an HTTP server on `:8080` with:
   - `GET /healthz` -- liveness probe
   - `GET /readyz` -- readiness probe (flips when `host.ReportReady()` is called)
   - ConnectRPC `PluginMetadataService` -- serves status and definition
   - `GET /console/` -- optional console UI assets
3. Reports lifecycle status via `host.ReportStatus()`

```
  Plugin Container (:8080)
  ┌──────────────────────────────────────────────────┐
  │                                                  │
  │  /healthz ◄─── Kubernetes liveness probe         │
  │  /readyz  ◄─── Kubernetes readiness probe        │
  │                                                  │
  │  /pluginmetadata.v1.PluginMetadataService/       │
  │    ├─ GetStatus    ◄─── plugin controller polls  │
  │    └─ GetDefinition ◄── console fetches metadata │
  │                                                  │
  │  /console/ ◄─── console UI iframe (optional)     │
  │                                                  │
  └──────────────────────────────────────────────────┘
```

## Controller RBAC

The plugin controller itself needs cluster-wide permissions to manage plugin resources:

| Resource              | Verbs                                    | Why                                      |
|-----------------------|------------------------------------------|------------------------------------------|
| `plugininstallations` | get, list, watch, update, patch          | Watch and reconcile CRs                  |
| `namespaces`          | get, list, watch, create, update, patch, delete | Create/delete plugin namespaces    |
| `serviceaccounts`     | get, list, watch, create, update, patch, delete | Manage plugin SAs                  |
| `services`            | get, list, watch, create, update, patch, delete | Expose plugin HTTP servers         |
| `deployments`         | get, list, watch, create, update, patch, delete | Run plugin containers              |
| `rolebindings`        | get, list, watch, create, update, patch, delete, bind | Bind admin role in plugin NS |
| `clusterroles`        | get, list, watch, bind                   | Bind existing ClusterRoles to plugin SAs |
| `clusterrolebindings` | get, list, watch, create, update, patch, delete | Manage optional cluster-wide bindings |
