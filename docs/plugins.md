# Fundament Plugin System

The plugin system allows extending Fundament with installable plugins that integrate into the platform's console UI, RBAC, and lifecycle management.

## System Overview

```
  ┌──────────────────────────────────────────────────────────────────────────┐
  │                         Fundament Cluster                               │
  │                                                                         │
  │  fundament namespace                                                    │
  │  ┌────────────────────────────────────────────────────────────────────┐  │
  │  │  PluginInstallation CRs          Plugin Controller                │  │
  │  │  ┌──────────────────────┐       ┌──────────────────────────────┐  │  │
  │  │  │ cert-manager-test    │──────►│ Watches CRs                  │  │  │
  │  │  │ another-plugin       │       │ Creates plugin namespaces    │  │  │
  │  │  └──────────────────────┘       │ Manages RBAC + deployments   │  │  │
  │  │                                 │ Polls plugin status          │  │  │
  │  │                                 └──────┬───────────────────────┘  │  │
  │  └────────────────────────────────────────┼──────────────────────────┘  │
  │                                           │ creates                     │
  │                     ┌─────────────────────┼─────────────────────┐       │
  │                     ▼                     ▼                     ▼       │
  │  plugin-cert-manager-test    plugin-another-plugin     plugin-...       │
  │  ┌───────────────────────┐  ┌───────────────────────┐                  │
  │  │ SA + RoleBinding      │  │ SA + RoleBinding      │                  │
  │  │ Deployment + Service  │  │ Deployment + Service  │                  │
  │  │ (+ ClusterRoleBinding │  │                       │                  │
  │  │  if requested)        │  │                       │                  │
  │  └───────────────────────┘  └───────────────────────┘                  │
  └──────────────────────────────────────────────────────────────────────────┘
```

## Three Components

| Component | Purpose |
|-----------|---------|
| [**Plugin SDK**](#plugin-sdk) | Go framework that plugins implement. Handles HTTP, health probes, metadata API, logging, and lifecycle. |
| [**Plugin Controller**](#plugin-controller) | Kubernetes controller that watches `PluginInstallation` CRs and manages plugin namespaces, RBAC, and deployments. |
| [**Plugin** (e.g. cert-manager)](#writing-a-plugin) | A container image that uses the SDK. Implements business logic (install software, manage CRDs, serve console UI). |

---

## Plugin SDK

The SDK provides all the boilerplate so plugin authors only implement business logic.

### Core Interface

```go
type Plugin interface {
    Definition() PluginDefinition   // Static metadata (from definition.yaml)
    Start(ctx context.Context, host Host) error  // Main logic, block until ctx cancelled
    Shutdown(ctx context.Context) error          // Graceful cleanup
}
```

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

### What `pluginsdk.Run()` Does

When a plugin binary calls `pluginsdk.Run(plugin)`, the SDK:

```
  pluginsdk.Run(plugin)
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
  Installing ──► Running ◄──► Degraded
      │              │
      ▼              ▼
   Failed        Uninstalling
```

| Phase | Meaning |
|-------|---------|
| `installing` | Plugin is setting up (e.g. running Helm install) |
| `running` | Plugin is healthy and operational |
| `degraded` | Plugin is running but something is wrong (transient error) |
| `failed` | Unrecoverable error (permanent error) |
| `uninstalling` | Plugin is cleaning up before shutdown |

### Error Types

The SDK provides error classification to drive retry behavior:

```go
// Transient: retryable, plugin stays "degraded"
return pluginerrors.NewTransient(fmt.Errorf("CRDs not yet ready: %w", err))

// Permanent: non-retryable, plugin goes to "failed"
return pluginerrors.NewPermanent(fmt.Errorf("invalid configuration: %w", err))
```

### SDK Helpers

| Helper | Purpose |
|--------|---------|
| `helpers/helm` | Wrapper around `helm upgrade --install` and `helm uninstall` |
| `helpers/crd` | Verify that required CRDs exist in the cluster |
| `helpers/controllerruntime` | Scaffold a controller-runtime manager |
| `console` | Convert embedded FS to `http.FileSystem` for console assets |
| `auth` | JWT validation middleware for Connect RPC interceptors |

---

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
  config:                # Optional: extra env vars for the container
    LOG_LEVEL: debug
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
  │  → ClusterRole/admin               │
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

---

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

    pluginsdk "github.com/fundament-oss/fundament/plugin-sdk"
)

type MyPlugin struct {
    def pluginsdk.PluginDefinition
}

func (p *MyPlugin) Definition() pluginsdk.PluginDefinition {
    return p.def
}

func (p *MyPlugin) Start(ctx context.Context, host pluginsdk.Host) error {
    host.ReportStatus(pluginsdk.PluginStatus{
        Phase:   pluginsdk.PhaseInstalling,
        Message: "setting up",
    })

    // Do setup work...

    host.ReportReady()
    host.ReportStatus(pluginsdk.PluginStatus{
        Phase:   pluginsdk.PhaseRunning,
        Message: "operational",
    })

    <-ctx.Done()
    return nil
}

func (p *MyPlugin) Shutdown(_ context.Context) error {
    return nil
}

func main() {
    def, err := pluginsdk.LoadDefinition("definition.yaml")
    if err != nil {
        log.Fatal(err)
    }
    pluginsdk.Run(&MyPlugin{def: def})
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

---

## Example: cert-manager Plugin

The cert-manager plugin is a reference implementation that installs and manages cert-manager.

### What It Does

1. **Start**: Runs `helm upgrade --install cert-manager` from the Jetstack Helm repo
2. **Verify**: Checks that all cert-manager CRDs exist (`certificates`, `issuers`, `clusterissuers`, `certificaterequests`)
3. **Reconcile**: Periodically re-checks CRD availability, reports degraded if missing
4. **Console**: Serves a placeholder console UI at `/console/`

### File Structure

```
plugins/cert-manager/
├── main.go             # Entry point: load definition, call pluginsdk.Run()
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
  pluginsdk.Run()
       │
       ├─ HTTP server on :8080
       │
       ▼
  Start()
       │
       ├─ ReportStatus("installing", "installing cert-manager")
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

---

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

---

## Plugin Sandbox

A self-contained development environment lives in `plugins/sandbox/`. It creates an isolated K3D cluster with only the plugin controller -- no database, auth services, or other Fundament components needed. The sandbox cluster (`fundament-plugin`) uses a separate registry on port `5112`, so it can coexist with the main Fundament cluster without conflicts.

### Quick Start

```bash
cd plugins
just cluster-create   # Create K3D cluster + registry (~10s)
just dev              # Build + deploy plugin-controller with file watching

# In another terminal:
cd plugins
just plugin-install cert-manager   # Build plugin, push to registry, apply CR
just plugin-status                 # Check PluginInstallation status
just logs                          # Watch controller logs

# Verify cert-manager actually works:
just cert-manager test             # Creates a self-signed ClusterIssuer + Certificate
just cert-manager test-cleanup     # Remove test resources

# Cleanup:
just plugin-uninstall cert-manager
just cluster-delete
```

### Available Commands

| Command | Description |
|---------|-------------|
| `just cluster-create` | Create a K3D cluster for plugin development |
| `just cluster-start` | Start the cluster (creates if it doesn't exist) |
| `just cluster-stop` | Stop the cluster without deleting it |
| `just cluster-delete` | Delete the cluster and registry |
| `just dev` | Deploy plugin-controller with file watching (auto-rebuild) |
| `just deploy` | Deploy plugin-controller (one-time) |
| `just undeploy` | Remove the deployment |
| `just plugin-install <plugin>` | Build plugin image, push to registry, apply CR |
| `just plugin-uninstall <plugin>` | Delete PluginInstallation CR |
| `just plugin-logs <plugin>` | Stream a specific plugin's logs |
| `just plugin-status` | Show all PluginInstallation CRs |
| `just logs` | Stream plugin-controller logs |
| `just cert-manager test` | Verify cert-manager with a self-signed certificate |
| `just cert-manager test-cleanup` | Remove cert-manager test resources |
