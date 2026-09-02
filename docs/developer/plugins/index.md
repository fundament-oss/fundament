---
title: Plugins
sidebar:
  label: Overview
  order: 2
---

The plugin system allows extending Fundament with installable plugins that integrate into the platform's console UI, RBAC, and lifecycle management.

To write one, start with the scaffolder rather than a blank directory:

```shell
functl plugin create my-plugin
```

It generates a buildable project with its manifest, Dockerfile and optionally a
console UI, and needs no Fundament account. See
[Writing a plugin](/docs/developer/plugins/writing-a-plugin).

## System overview

```
  ┌──────────────────────────────────────────────────────────────────────────┐
  │                         Fundament Cluster                                │
  │                                                                          │
  │  PluginInstallation CRs (cluster-scoped)                                 │
  │  ┌──────────────────────┐                                                │
  │  │ system--cert-manager │                                                │
  │  │ acme--another-plugin │                                                │
  │  └──────────┬───────────┘                                                │
  │             │                                                            │
  │  fundament namespace                                                     │
  │  ┌──────────┼────────────────────────────────────────────────────────┐   │
  │  │          ▼                                                        │   │
  │  │  Plugin Controller                                                │   │
  │  │  ┌──────────────────────────────┐                                 │   │
  │  │  │ Watches CRs                  │                                 │   │
  │  │  │ Creates plugin namespaces    │                                 │   │
  │  │  │ Manages RBAC + deployments   │                                 │   │
  │  │  │ Polls plugin status          │                                 │   │
  │  │  └──────┬───────────────────────┘                                 │   │
  │  └─────────┼─────────────────────────────────────────────────────────┘   │
  │                                           │ creates                      │
  │               ┌───────────────────────────┼─────────────────────┐        │
  │               ▼                           ▼                     ▼        │
  │  plugin-system--cert-manager  plugin-acme--another-plugin  plugin-...    │
  │  ┌───────────────────────┐  ┌───────────────────────┐                    │
  │  │ SA + RoleBinding      │  │ SA + RoleBinding      │                    │
  │  │ Deployment + Service  │  │ Deployment + Service  │                    │
  │  │ (+ ClusterRole/CRB    │  │                       │                    │
  │  │  from the definition) │  │                       │                    │
  │  └───────────────────────┘  └───────────────────────┘                    │
  └──────────────────────────────────────────────────────────────────────────┘
```

## Three components

| Component | Purpose |
|-----------|---------|
| [**Plugin Runtime**](#plugin-runtime) | Go framework that plugins implement. Handles HTTP, health probes, metadata API, logging, and lifecycle. |
| [**Plugin Controller**](#plugin-controller) | Kubernetes controller that watches `PluginInstallation` CRs and manages plugin namespaces, RBAC, and deployments. |
| [**Plugin**](/docs/developer/plugins/writing-a-plugin) | A container image that uses the SDK. Implements business logic (install software, manage CRDs, serve console UI). |

## Plugin Runtime

The runtime provides all the boilerplate so plugin authors only implement business logic.

### Core interface

```go
type Plugin interface {
    Definition() PluginDefinition   // Static metadata (from definition.yaml)
    Start(ctx context.Context, host Host) error  // Main logic, block until ctx cancelled
    Shutdown(ctx context.Context) error          // Graceful cleanup
}
```

> **Idempotency requirement**: `Start` is called every time the container starts, including restarts. Implementations must be idempotent — for example, using `helm upgrade --install` rather than `helm install`, and checking existing state before performing setup.

### Optional interfaces

```go
type Reconciler interface {        // Periodic health checks (default: every 5m)
    Reconcile(ctx context.Context, host Host) error
}

type Installer interface {         // Structured install/uninstall/upgrade
    Install(ctx context.Context, host Host) error
    Uninstall(ctx context.Context, host Host) error
    Upgrade(ctx context.Context, host Host) error
}

type ConsoleProvider interface {   // Serve UI assets at /console/
    ConsoleAssets() http.FileSystem
}
```

### What `pluginruntime.Run()` does

When a plugin binary calls `pluginruntime.Run(plugin)`, the SDK:

```
  pluginruntime.Run(plugin)
        │
        ├─ Parse environment config (cluster ID, org ID, log level, etc.)
        ├─ Initialize structured JSON logger
        ├─ Initialize OpenTelemetry (tracing + metrics)
        ├─ Create Host (provides logger, telemetry, status reporting)
        │
        ├─ Start HTTP server on :8080
        │   ├─ GET /livez ───────── Liveness probe (always 200)
        │   ├─ GET /readyz ───────── Readiness probe (200 after ReportReady())
        │   ├─ ConnectRPC ─────────── PluginMetadataService (status + definition)
        │   └─ GET /console/ ──────── Static UI assets (if ConsoleProvider)
        │
        ├─ Call plugin.Start(ctx, host)
        │   └─ Plugin does its work, calls host.ReportReady() when ready
        │
        ├─ Start reconciliation loop (if Reconciler interface implemented)
        │
        ├─ Wait for SIGTERM/SIGINT
        │
        └─ Call plugin.Shutdown(ctx) with deadline
```

### Host interface

The `Host` is passed to `Start()` and `Reconcile()`:

```go
type Host interface {
    Logger() *slog.Logger                  // Structured logger
    Telemetry() TelemetryService           // Tracing + metrics
    ReportStatus(status PluginStatus)      // Update status (visible to controller)
    ReportReady()                          // Flip readiness probe to healthy
}
```

### Plugin phases

```
                 ┌──────────────┐
                 │              │
  Installing ──►─┤  Running ◄──►── Degraded
      │    │     │              │      │
      │    │     └──────────────┘      │
      │    │            │              │
      │    ▼            ▼              │
      │  Degraded ─► Failed ◄──────────┘
      │              │  ▲
      │              ▼  │
      └───────► Uninstalling
```

| Phase | Meaning |
|-------|---------|
| `installing` | Plugin is setting up (e.g. running Helm install) |
| `running` | Plugin is healthy and operational |
| `degraded` | Transient error — plugin will retry (e.g. failed install, missing CRD) |
| `failed` | Permanent error requiring human intervention (e.g. invalid configuration) |
| `uninstalling` | Plugin is cleaning up before removal |

Transitions:
- **installing → running**: Setup completed successfully.
- **installing → degraded**: A recoverable error during setup (e.g. image pull backoff, transient helm failure). The container will restart and retry.
- **installing → failed**: A permanent error during setup (e.g. invalid configuration).
- **running → degraded**: A transient error is detected during reconciliation (e.g. missing CRD).
- **degraded → running**: The transient error is resolved.
- **degraded → failed**: A permanent error occurs while degraded.
- **running/degraded/installing → uninstalling**: The plugin CR is deleted.
- **failed → uninstalling**: The plugin CR is deleted; cleanup still runs.
- **uninstalling → failed**: A permanent error during cleanup (e.g. cannot delete resources).

### Error types

The SDK provides error classification to drive retry behavior:

```go
// Transient: retryable, plugin stays "degraded"
return pluginerrors.NewTransient(fmt.Errorf("CRDs not yet ready: %w", err))

// Permanent: non-retryable, plugin goes to "failed"
return pluginerrors.NewPermanent(fmt.Errorf("invalid configuration: %w", err))
```

### SDK helpers

The SDK provides optional helpers that plugins can use. Plugins are free to choose their own installation and management approach — these helpers are conveniences, not requirements.

| Helper | Purpose |
|--------|---------|
| `helpers/helm` | Wrapper around `helm upgrade --install`, `helm status`, and `helm uninstall` |
| `helpers/crd` | Verify that required CRDs exist in the cluster |
| `helpers/controllerruntime` | Scaffold a controller-runtime manager |
| `console` | Convert embedded FS to `http.FileSystem` for console assets |
| `auth` | JWT validation middleware for Connect RPC interceptors |

## Plugin Controller

The controller watches `PluginInstallation` CRs.

### PluginInstallation CRD

```yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: system--cert-manager
spec:
  definitionRef:
    organizationName: system
    pluginName: cert-manager
    pluginVersion: v1.17.2
    definitionHash: sha256:...
  config:                # Optional: extra env vars (injected with FUNP_ prefix)
    LOG_LEVEL: debug     # → becomes FUNP_LOG_LEVEL in the container
```

A plugin's identity is the pair `(organizationName, pluginName)`, and
`metadata.name` must equal `<organizationName>--<pluginName>` — see
[FUN-17](/funs/fun-17#plugin-identity-and-naming) for why.

### What the controller creates

For each `PluginInstallation`, the controller creates:

```
  plugin-<installationName> namespace   (hashed if the derived name would exceed 63 chars)
  ├─ ServiceAccount/plugin
  ├─ RoleBinding ──► ClusterRole/admin (namespace-scoped)
  ├─ Deployment (runs the plugin's image, container named "plugin")
  └─ Service/plugin (:8080)

  ClusterRole "plugin-<installationName>-scope" (cluster-scoped)
  └─ Materialised from the pinned PluginDefinition's permissions.rbac,
     bound to the plugin ServiceAccount via a ClusterRoleBinding
```

Every namespace-scoped child is named `plugin` — not `plugin-<installationName>`
— because the namespace already disambiguates installations; see
[FUN-17](/funs/fun-17#plugin-identity-and-naming).

### RBAC model

```
  ┌─────────────────────────────────────┐
  │  DEFAULT (always)                   │
  │                                     │
  │  RoleBinding in plugin namespace    │
  │  → ClusterRole/admin                │
  │                                     │
  │  Plugin can manage all resources    │
  │  within its own namespace.          │
  └─────────────────────────────────────┘
                  +
  ┌─────────────────────────────────────┐
  │  ALWAYS (from the pinned definition)│
  │                                     │
  │  ClusterRole materialised from      │
  │  spec.permissions.rbac in the       │
  │  PluginDefinition, bound via a      │
  │  ClusterRoleBinding                 │
  │                                     │
  │  For plugins that need cluster-wide │
  │  access (CRDs, webhooks, resources  │
  │  in other namespaces).              │
  └─────────────────────────────────────┘
```

### Reconciliation loop

```
  PluginInstallation CR event
           │
           ▼
  Add finalizer ──► Create Namespace ──► Create SA
           │
           ├──► Create RoleBinding (→ admin)
           ├──► Create ClusterRole/ClusterRoleBinding (from the pinned definition)
           ├──► Create Deployment
           ├──► Create Service
           │
           ▼
  Poll plugin metadata API
  GET http://plugin.<namespace>.svc.cluster.local:8080
       └─ PluginMetadataService.GetStatus()
           │
           ▼
  Update CR .status
  (phase, ready, message, namespace)
           │
           ▼
  RequeueAfter (poll interval)
```

### Deletion

```
  CR deleted ──► Finalizer triggers:
                 ├─ Delete ClusterRole/ClusterRoleBinding
                 ├─ Delete Namespace (cascades to all namespace-scoped resources)
                 └─ Remove finalizer → CR garbage collected
```
