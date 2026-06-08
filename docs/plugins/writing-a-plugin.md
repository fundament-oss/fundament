---
title: Writing a plugin
sidebar:
  order: 2
---

Writing a plugin consists of several steps, each described below.

### 1. Create `definition.yaml`

```yaml
apiVersion: fundament.io/v1
kind: PluginDefinition
metadata:
  name: my-plugin
  displayName: My Plugin
  version: v1.0.0
  description: Does something useful
  author: My Team
  license: Apache-2.0
  icon: puzzle-piece
  tags:
    - example
spec:
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
        icon: pencil-on-square

  customComponents:
    MyResource:
      list: myresources-list.html
      detail: myresources-detail.html

  allowedResources:
    - group: my-api.io
      version: v1
      resource: myresources
      verbs: [get, list]

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

`customComponents` maps every CRD kind that appears in the menu to the
HTML files your plugin ships under `console/`. The console does not
generate fallback views from the CRD schema, so every menu entry needs
its own `list` and `detail` page. `allowedResources` is the allowlist the
console host enforces on every `fundament.k8s.list` / `.get` call the
plugin makes — see [Custom UI](custom-ui) and
[Console integration](console-integration) for the full story.

### 2. Implement the plugin

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

### 3. Ship the console UI

Plugins serve their own list and detail HTML from an embedded filesystem
mounted at `/console/` by the runtime. Implement `ConsoleProvider`:

```go
package main

import (
    "embed"
    "net/http"

    "github.com/fundament-oss/fundament/plugin-sdk/helpers/console"
)

//go:embed console
var consoleFS embed.FS

func (p *MyPlugin) ConsoleAssets() http.FileSystem {
    return console.MustNewFileSystem(consoleFS)
}
```

Put one HTML file per `customComponents` entry under `console/` (e.g.
`console/myresources-list.html`, `console/myresources-detail.html`).
See [Custom UI](custom-ui) for what those pages need to do and
[Example: cert-manager](example-cert-manager) for a worked layout.

### 4. Build a container image

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

### 5. Create a PluginInstallation

```yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: my-plugin
spec:
  image: registry.example.com/my-plugin:v1.0.0
  pluginName: my-plugin
  # Only if your plugin needs cluster-wide access:
  # clusterRoles:
  #   - cluster-admin
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

## Plugin sandbox

A self-contained development environment for plugin development. See [`plugins/README.md`](../../plugins/README.md) for setup instructions and available commands.
