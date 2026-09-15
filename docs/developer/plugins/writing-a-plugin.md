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
| `--license`             | SPDX id                   | `metadata.license`, and the `LICENSE` file. MIT, Apache-2.0, GPL-3.0-only and EUPL-1.2 are written in full; any other id gets a placeholder to fill in. |
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
├── mise.toml             pinned tool versions (Go, just, bun)
├── LICENSE               the text of --license
└── README.md
```

Tool versions are pinned in `mise.toml`, the same way this repository pins its
own: `mise install` in the new project gets the Go toolchain its `go.mod` asks
for, plus `just` and, with `--console=vite`, `bun`.

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
template starts from the least privilege that can work: the three rules Helm
itself cannot install a release without (namespaces, its release Secrets, and
Pods for `--wait`). Everything a chart might additionally create -- Deployments,
Services, ServiceAccounts, ConfigMaps, CRDs, RBAC -- is there as commented-out
rules: uncomment what your chart needs.

`customComponents` maps a CRD kind to the HTML files your plugin ships under
`console/`. It is optional: any menu entry without a custom component renders the
console's generated read-only list and detail views from the CRD schema, so add
`customComponents` only for kinds that need write actions or a bespoke layout.
`allowedResources` lists the resources the plugin's pages read. Page calls are authorized by the user's and the plugin's RBAC (`permissions.rbac`): [Custom UI](custom-ui#fetching-data).

`menu`, `customComponents`, `allowedResources` and the files under `console/`
are one contract with nothing to enforce it at build time. The generated
`definition_test.go` checks that every file named in `customComponents` is
actually embedded; the rest is on you, so change them together.

`definition.yaml` has **no `spec.image`**: `functl plugin publish --image=<repo>@sha256:<digest>` injects the pushed digest, so a published definition pins immutable code and its hash binds that code. A manifest that names a mutable tag is rejected.

## Implement the plugin

`Plugin` is the only required interface. `Installer`, `Reconciler` and
`ConsoleProvider` are optional and detected at runtime.

```go
package main

import (
	"context"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

type MyPluginPlugin struct{}

func NewMyPluginPlugin() *MyPluginPlugin { return &MyPluginPlugin{} }

func (p *MyPluginPlugin) Start(ctx context.Context, host pluginruntime.Host) error {
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

func (p *MyPluginPlugin) Shutdown(_ context.Context) error {
	return nil
}

func main() {
	pluginruntime.Run(NewMyPluginPlugin())
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

func (p *MyPluginPlugin) ConsoleAssets() http.FileSystem {
	return console.NewFileSystem(consoleFiles, "console")
}
```

Put one HTML file per `customComponents` entry under `console/` (e.g.
`console/myresources-list.html`, `console/myresources-detail.html`).
See [Custom UI](custom-ui) for what those pages need to do and [`plugins/cert-manager/console/`](https://github.com/fundament-oss/fundament/tree/master/plugins/cert-manager/console) for a working set.

With `--console=vite`, `console/` is build output: pass `console.RequireHTML()` as well, so a binary built without the UI build fails at startup.

## Build a container image

The scaffolder writes a self-contained Dockerfile that builds with the project
directory as its context:

```dockerfile
FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /my-plugin-plugin .

FROM alpine:3.21
# Add any CLI tools your plugin needs; the helm template installs helm here.
COPY --from=build /my-plugin-plugin /usr/local/bin/my-plugin-plugin
COPY definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["my-plugin-plugin"]
```

## Publish and install

Publish the image and definition, install the plugin and check it in the console: [Testing plugins locally](testing-plugins-locally).
