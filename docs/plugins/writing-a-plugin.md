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
  icon: puzzle
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

### 3. Build a container image

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
spec:
  image: registry.example.com/my-plugin:v1.0.0
  pluginName: my-plugin
  version: v1.0.0
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
