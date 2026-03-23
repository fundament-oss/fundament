---
title: Plugins
sidebar:
  order: 5
---

The plugin system allows extending Fundament with installable plugins that integrate into the platform's console UI, RBAC, and lifecycle management.

## System Overview

```
  ┌──────────────────────────────────────────────────────────────────────────┐
  │                         Fundament Cluster                                │
  │                                                                          │
  │  fundament namespace                                                     │
  │  ┌───────────────────────────────────────────────────────────────────┐   │
  │  │  PluginInstallation CRs          Plugin Controller                │   │
  │  │  ┌──────────────────────┐       ┌──────────────────────────────┐  │   │
  │  │  │ cert-manager-test    │──────►│ Watches CRs                  │  │   │
  │  │  │ another-plugin       │       │ Creates plugin namespaces    │  │   │
  │  │  └──────────────────────┘       │ Manages RBAC + deployments   │  │   │
  │  │                                 │ Polls plugin status          │  │   │
  │  │                                 └──────┬───────────────────────┘  │   │
  │  └────────────────────────────────────────┼──────────────────────────┘   │
  │                                           │ creates                      │
  │               ┌───────────────────────────┼─────────────────────┐        │
  │               ▼                           ▼                     ▼        │
  │   plugin-cert-manager-test    plugin-another-plugin     plugin-...       │
  │  ┌───────────────────────┐  ┌───────────────────────┐                    │
  │  │ SA + RoleBinding      │  │ SA + RoleBinding      │                    │
  │  │ Deployment + Service  │  │ Deployment + Service  │                    │
  │  │ (+ ClusterRoleBinding │  │                       │                    │
  │  │  if requested)        │  │                       │                    │
  │  └───────────────────────┘  └───────────────────────┘                    │
  └──────────────────────────────────────────────────────────────────────────┘
```

## Three Components

| Component | Purpose |
|-----------|---------|
| [**Plugin Runtime**](#plugin-runtime) | Go framework that plugins implement. Handles HTTP, health probes, metadata API, logging, and lifecycle. |
| [**Plugin Controller**](#plugin-controller) | Kubernetes controller that watches `PluginInstallation` CRs and manages plugin namespaces, RBAC, and deployments. |
| [**Plugin** (e.g. cert-manager)](#writing-a-plugin) | A container image that uses the SDK. Implements business logic (install software, manage CRDs, serve console UI). |

## Plugin Runtime

The runtime provides all the boilerplate so plugin authors only implement business logic.

### Core Interface

```go
type Plugin interface {
    Definition() PluginDefinition   // Static metadata (from definition.yaml)
    Start(ctx context.Context, host Host) error  // Main logic, block until ctx cancelled
    Shutdown(ctx context.Context) error          // Graceful cleanup
}
```

> **Idempotency requirement**: `Start` is called every time the container starts, including restarts. Implementations must be idempotent — for example, using `helm upgrade --install` rather than `helm install`, and checking existing state before performing setup.

### Optional Interfaces

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

### What `pluginruntime.Run()` Does

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
        │   ├─ GET /healthz ──────── Liveness probe (always 200)
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

### Host Interface

The `Host` is passed to `Start()` and `Reconcile()`:

```go
type Host interface {
    Logger() *slog.Logger                  // Structured logger
    Telemetry() TelemetryService           // Tracing + metrics
    ReportStatus(status PluginStatus)      // Update status (visible to controller)
    ReportReady()                          // Flip readiness probe to healthy
}
```

### Plugin Phases

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

### Error Types

The SDK provides error classification to drive retry behavior:

```go
// Transient: retryable, plugin stays "degraded"
return pluginerrors.NewTransient(fmt.Errorf("CRDs not yet ready: %w", err))

// Permanent: non-retryable, plugin goes to "failed"
return pluginerrors.NewPermanent(fmt.Errorf("invalid configuration: %w", err))
```

### SDK Helpers

The SDK provides optional helpers that plugins can use. Plugins are free to choose their own installation and management approach — these helpers are conveniences, not requirements.

| Helper | Purpose |
|--------|---------|
| `helpers/helm` | Wrapper around `helm upgrade --install`, `helm status`, and `helm uninstall` |
| `helpers/crd` | Verify that required CRDs exist in the cluster |
| `helpers/controllerruntime` | Scaffold a controller-runtime manager |
| `console` | Convert embedded FS to `http.FileSystem` for console assets |
| `auth` | JWT validation middleware for Connect RPC interceptors |

## Plugin Controller

The controller runs in the `fundament` namespace and watches `PluginInstallation` CRs.

### PluginInstallation CRD

```yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: cert-manager-test
  namespace: fundament
spec:
  image: ghcr.io/fundament-oss/fundament/cert-manager-plugin:v1.0.0
  pluginName: cert-manager-test
  version: v1.17.2
  clusterRoles:          # Optional: bind SA to these ClusterRoles
    - cluster-admin
  config:                # Optional: extra env vars (injected with FUNP_ prefix)
    LOG_LEVEL: debug     # → becomes FUNP_LOG_LEVEL in the container
```

### What the Controller Creates

For each `PluginInstallation`, the controller creates:

```
  plugin-{pluginName} namespace
  ├─ ServiceAccount/plugin-{pluginName}
  ├─ RoleBinding ──► ClusterRole/admin (always, namespace-scoped)
  ├─ Deployment (runs the plugin image)
  └─ Service (:8080)

  ClusterRoleBinding (only if spec.clusterRoles is set)
  └─ Binds SA to requested ClusterRoles at cluster scope
```

### RBAC Model

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
  │  OPTIONAL (spec.clusterRoles)       │
  │                                     │
  │  ClusterRoleBinding                 │
  │  → ClusterRole/{requested}          │
  │                                     │
  │  For plugins that need cluster-wide │
  │  access (CRDs, webhooks, resources  │
  │  in other namespaces).              │
  └─────────────────────────────────────┘
```

### Reconciliation Loop

```
  PluginInstallation CR event
           │
           ▼
  Add finalizer ──► Create Namespace ──► Create SA
           │
           ├──► Create RoleBinding (→ admin)
           ├──► Create ClusterRoleBindings (if spec.clusterRoles)
           ├──► Create Deployment
           ├──► Create Service
           │
           ▼
  Poll plugin metadata API
  GET http://plugin-{name}.plugin-{name}.svc.cluster.local:8080
       └─ PluginMetadataService.GetStatus()
           │
           ▼
  Update CR .status
  (phase, ready, message, pluginVersion)
           │
           ▼
  RequeueAfter (poll interval)
```

### Deletion

```
  CR deleted ──► Finalizer triggers:
                 ├─ Delete ClusterRoleBindings (if any)
                 ├─ Delete Namespace (cascades to all resources)
                 └─ Remove finalizer → CR garbage collected
```

## Writing a Plugin

### 1. Create `definition.yaml`

```yaml
apiVersion: fundament.io/v1
kind: PluginDefinition
spec:
  metadata:
    name: my-plugin
    displayName: My Plugin
    version: v1.0.0
    description: Does something useful
    author: My Team
    license: Apache-2.0
    icon: puzzle
    tags:
      - example

  permissions:
    capabilities:
      - internet_access
    rbac:
      - apiGroups: ["my-api.io"]
        resources: ["myresources"]
        verbs: ["get", "list", "watch"]

  menu:
    project:
      - crd: myresources.my-api.io
        list: true
        detail: true
        create: true
        icon: box

  uiHints:
    myresources.my-api.io:
      statusMapping:
        jsonPath: ".status.phase"
        values:
          "Ready":
            badge: success
            label: Ready
          "Failed":
            badge: danger
            label: Failed
```

### 2. Implement the Plugin

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

type MyPlugin struct {
    def pluginruntime.PluginDefinition
}

func (p *MyPlugin) Definition() pluginruntime.PluginDefinition {
    return p.def
}

func (p *MyPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
    host.ReportStatus(pluginruntime.PluginStatus{
        Phase:   pluginruntime.PhaseInstalling,
        Message: "setting up",
    })

    // Do setup work...

    host.ReportReady()
    host.ReportStatus(pluginruntime.PluginStatus{
        Phase:   pluginruntime.PhaseRunning,
        Message: "operational",
    })

    <-ctx.Done()
    return nil
}

func (p *MyPlugin) Shutdown(_ context.Context) error {
    return nil
}

func main() {
    def, err := pluginruntime.LoadDefinition("definition.yaml")
    if err != nil {
        log.Fatal(err)
    }
    pluginruntime.Run(&MyPlugin{def: def})
}
```

### 3. Build a Container Image

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/my-plugin ./plugins/my-plugin

FROM alpine:3.21
# Add any CLI tools your plugin needs (e.g. helm)
COPY --from=builder /bin/my-plugin /my-plugin
COPY plugins/my-plugin/definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["/my-plugin"]
```

### 4. Create a PluginInstallation

```yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: my-plugin
  namespace: fundament
spec:
  image: registry.example.com/my-plugin:v1.0.0
  pluginName: my-plugin
  version: v1.0.0
  # Only if your plugin needs cluster-wide access:
  # clusterRoles:
  #   - cluster-admin
```

## Example: cert-manager Plugin

The cert-manager plugin is a reference implementation that installs and manages cert-manager.

### What It Does

1. **Start**: Checks if cert-manager is already installed, then runs `helm upgrade --install cert-manager` from the Jetstack Helm repo
2. **Verify**: Checks that all cert-manager CRDs exist (`certificates`, `issuers`, `clusterissuers`, `certificaterequests`)
3. **Reconcile**: Periodically re-checks CRD availability, reports degraded if missing
4. **Console**: Serves a placeholder console UI at `/console/`

### File Structure

```
plugins/cert-manager/
├── main.go             # Entry point: load definition, call pluginruntime.Run()
├── plugin.go           # Plugin implementation (Start, Install, Reconcile, etc.)
├── console.go          # Embeds console/ directory as http.FileSystem
├── definition.yaml     # Plugin metadata, permissions, menu entries, UI hints
├── console/
│   └── placeholder.html
├── plugin_test.go      # Unit tests
└── Dockerfile          # Multi-stage build (Go build + alpine with helm)
```

### Why It Needs `cluster-admin`

cert-manager installs cluster-scoped resources that require broad permissions:
- CRDs (`certificates.cert-manager.io`, etc.)
- ClusterRoles and ClusterRoleBindings
- ValidatingWebhookConfigurations / MutatingWebhookConfigurations
- Resources across multiple namespaces

The default namespace-admin RoleBinding only covers the plugin's own namespace. The `clusterRoles: [cluster-admin]` field in the PluginInstallation grants the additional access.

```yaml
# plugins/cert-manager/install.yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: cert-manager-test
  namespace: fundament
spec:
  image: localhost:5111/cert-manager-plugin:latest
  pluginName: cert-manager-test
  version: v1.17.2
  clusterRoles:
    - cluster-admin
```

### Plugin Lifecycle

```
  Container starts
       │
       ▼
  pluginruntime.Run()
       │
       ├─ HTTP server on :8080
       │
       ▼
  Start()
       │
       ├─ Check if cert-manager is already installed
       ├─ ReportStatus("installing", "checking/installing cert-manager")
       ├─ helm upgrade --install cert-manager jetstack/cert-manager
       ├─ Create k8s client
       ├─ crd.VerifyAll([certificates, certificaterequests, issuers, clusterissuers])
       ├─ ReportReady()
       ├─ ReportStatus("running", "cert-manager is running")
       └─ Block until SIGTERM
              │
              ▼
  Reconcile() (every 5 minutes)
       │
       ├─ crd.VerifyAll(...)
       ├─ If OK:  ReportStatus("running")
       └─ If not: ReportStatus("degraded")
```

## Metadata API

Every plugin exposes a ConnectRPC service that the controller and console consume:

```protobuf
service PluginMetadataService {
  rpc GetStatus(GetStatusRequest) returns (GetStatusResponse);
  rpc GetDefinition(GetDefinitionRequest) returns (GetDefinitionResponse);
}
```

| Consumer | Method | Purpose |
|----------|--------|---------|
| Plugin Controller | `GetStatus` | Poll phase, message, version → write to CR `.status` |
| Console Frontend | `GetDefinition` | Fetch menu entries, UI hints, CRDs → render plugin UI |

## Plugin Sandbox

A self-contained development environment for plugin development. See [`plugins/README.md`](../plugins/README.md) for setup instructions and available commands.
