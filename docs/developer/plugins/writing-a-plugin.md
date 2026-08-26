---
title: Writing a plugin
sidebar:
  order: 3
---

## Scaffold the project

`functl plugin create` generates a complete, buildable plugin project. It works
offline and does not require a Fundament account.

```shell
functl plugin create my-plugin
```

It prompts for the details it needs, then writes the project to `./my-plugin`,
runs `git init` and `go mod tidy`. Every prompt has a matching flag, so it is
equally usable unattended:

```shell
functl plugin create my-plugin \
  --template=helm \
  --console=vanilla \
  --crd myresources.my-api.io \
  --kind MyResource \
  --module github.com/my-org/my-plugin \
  --yes
```

| Flag                    | Values                    | What it changes                                                                 |
| ----------------------- | ------------------------- | ------------------------------------------------------------------------------- |
| `--template`            | `minimal`, `helm`         | `minimal` is a bare `Start`/`Shutdown`. `helm` adds install/upgrade/uninstall of an upstream chart, plus a reconcile loop that verifies its CRDs. |
| `--console`             | `none`, `vanilla`, `vite` | `vanilla` is plain HTML + ES modules. `vite` is a TypeScript app with the NLDD design system, built with bun. |
| `--crd` / `--kind`      | `<plural>.<group>` / Kind | The custom resource the generated console pages and CRD checks refer to.          |
| `--module`              | Go module path            | The generated `go.mod` module path.                                              |
| `--dir`                 | path                      | Where to write the project (default `./<name>`).                                 |
| `--no-git` / `--no-tidy`|                           | Skip `git init` / `go mod tidy`.                                                 |
| `--yes`                 |                           | Accept every default without prompting.                                          |

The name must be a lowercase DNS label of at most 56 characters: the controller
derives the plugin's namespace and other resource names from it by prefixing
`plugin-`, and those have to fit in Kubernetes' 63-character limit.

```
my-plugin/
├── definition.yaml       the plugin manifest
├── plugin.go             the plugin itself
├── main.go               hands the plugin to the SDK runtime
├── console.go            embeds the console UI          (--console != none)
├── console/              console pages or build output  (--console != none)
├── console-ui/           console UI source              (--console=vite)
├── Dockerfile
├── Justfile
├── go.mod
└── README.md
```

## Fill in `definition.yaml`

The manifest describes the plugin to the platform. The scaffolder writes a valid
one with `TODO` markers; these are the parts worth understanding.

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

`spec.permissions.rbac` is materialised verbatim as a ClusterRole bound to the
plugin's ServiceAccount, so grant only what the plugin actually uses. The `helm`
template starts you off with the rules Helm itself needs to create and wait on a
release; trim anything your chart does not touch.

`customComponents` maps a CRD kind to the HTML files your plugin ships under
`console/`. It is optional: any menu entry without a custom component renders the
console's generated read-only list and detail views from the CRD schema, so add
`customComponents` only for kinds that need write actions or a bespoke layout.
`allowedResources` is the allowlist the console host enforces on every
`fundament.k8s.list` / `.get` call the plugin makes — see
[Custom UI](custom-ui) and [Console integration](console-integration) for the
full story.

`menu`, `customComponents`, `allowedResources` and the files under `console/`
are one contract with nothing to enforce it at build time. The generated
`definition_test.go` checks that every file named in `customComponents` is
actually embedded; the rest is on you, so change them together.

There is deliberately **no `spec.image`**. Publishing builds the image, resolves
the pushed manifest digest and injects it, so a published definition always pins
immutable code and its hash binds that exact code. A manifest that names a
mutable tag is rejected.

## Implement the plugin

`Plugin` is the only required interface. `Installer`, `Reconciler` and
`ConsoleProvider` are optional and detected at runtime.

```go
package main

import (
	"context"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

type MyPlugin struct{}

func NewMyPlugin() *MyPlugin { return &MyPlugin{} }

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
	pluginruntime.Run(NewMyPlugin())
}
```

`Start` blocks until the context is cancelled, and **must be idempotent**: the
container is restarted on upgrades, evictions and crashes, so it runs again from
scratch every time. Check what already exists before creating it. `Shutdown`
runs on every restart too, so it must not uninstall anything.

Classify failures so the platform knows whether to retry:
`pluginerrors.NewTransient(err)` reports the plugin as degraded and retries;
`pluginerrors.NewPermanent(err)` reports it as failed and does not.

## Ship the console UI

Plugins serve their own list and detail HTML from an embedded filesystem
mounted at `/console/` by the runtime. Implement `ConsoleProvider`:

```go
package main

import (
	"embed"
	"net/http"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/console"
)

//go:embed console/*
var consoleFiles embed.FS

func (p *MyPlugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console")
}
```

Put one HTML file per `customComponents` entry under `console/` (e.g.
`console/myresources-list.html`, `console/myresources-detail.html`).
See [Custom UI](custom-ui) for what those pages need to do and
[Example: cert-manager](example-cert-manager) for a worked layout.

If `console/` is *build output* rather than committed source — which it is with
`--console=vite` — pass `console.RequireHTML()` as well, so a binary built
without running the UI build fails at startup instead of serving a blank iframe.

## Build a container image

The scaffolder writes a self-contained Dockerfile that builds with the project
directory as its context:

```dockerfile
FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /my-plugin .

FROM alpine:3.21
# Add any CLI tools your plugin needs; the helm template installs helm here.
COPY --from=build /my-plugin /usr/local/bin/my-plugin
COPY definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["my-plugin"]
```

The first-party plugins under `plugins/` differ here: they build with the
monorepo root as the context, because they live inside it.

## Install it

A plugin is installed by creating a `PluginInstallation` that pins a *published*
definition by name, version and hash:

```yaml
apiVersion: plugins.fundament.io/v1
kind: PluginInstallation
metadata:
  name: my-plugin
spec:
  definitionRef:
    pluginName: my-plugin
    pluginVersion: v1.0.0
    definitionHash: sha256:...
  config:
    SOME_SETTING: value
```

The image is **not** part of the CR: the controller fetches the definition from
organization-api, checks its hash against `definitionHash`, and takes the image
from there. `spec.config` entries are injected into the plugin as `FUNP_*`
environment variables.

Publishing a **standalone** plugin is not supported yet. Today definitions are
published from inside the monorepo with `just plugin-publish <dir>`, which
requires the plugin to live under `plugins/` and to have a catalog entry in the
appstore.

## Metadata API

Every plugin exposes a ConnectRPC service that the controller consumes:

```protobuf
service PluginMetadataService {
  rpc GetStatus(GetStatusRequest) returns (GetStatusResponse);
  rpc RequestUninstall(RequestUninstallRequest) returns (RequestUninstallResponse);
}
```

| Consumer          | Method             | Purpose                                                    |
| ----------------- | ------------------ | ---------------------------------------------------------- |
| Plugin Controller | `GetStatus`        | Poll phase, message, version → write to CR `.status`        |
| Plugin            | `RequestUninstall` | Ask the platform to tear the plugin down                    |

Plugin definitions are served to the console by organization-api, not by the
plugin pod.

## Plugin sandbox

A self-contained development environment for plugin development. See [`plugins/README.md`](https://github.com/fundament-oss/fundament/blob/master/plugins/README.md) for setup instructions and available commands.
