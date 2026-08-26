# Fundament Plugin SDK

The Go framework a Fundament plugin embeds. It provides the HTTP server, health
probes, the plugin metadata RPC, console asset serving, the reconcile loop and
signal handling, so a plugin only implements its own behaviour.

```
go get github.com/fundament-oss/fundament/plugin-sdk
```

```go
package main

import "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"

func main() {
	pluginruntime.Run(NewMyPlugin())
}
```

Do not start from a blank directory — scaffold a complete, buildable project:

```
functl plugin create my-plugin
```

See [docs/developer/plugins/](../docs/developer/plugins/) for the architecture
and the plugin authoring guide.

## Interfaces

`Plugin` is required; the rest are optional and detected at runtime.

| Interface         | Purpose                                                        |
| ----------------- | -------------------------------------------------------------- |
| `Plugin`          | `Start` (blocks until ctx is cancelled) and `Shutdown`           |
| `Installer`       | `Install` / `Uninstall` / `Upgrade` lifecycle hooks              |
| `Reconciler`      | `Reconcile`, called every `FUNP_RECONCILE_INTERVAL` (default 5m) |
| `ConsoleProvider` | `ConsoleAssets()`, mounted at `/console/`                        |

## This is a separate Go module

`plugin-sdk` has its own `go.mod` so plugin authors depend on the SDK rather
than on the whole Fundament monorepo. Two consequences for contributors:

- `go build`, `go test`, `go fmt`, `go generate` and `golangci-lint` at the repo
  root stop at this directory. The `Justfile` recipes run a second pass in here;
  CI has a dedicated `plugin-sdk` job.
- The root module reaches this code through
  `replace github.com/fundament-oss/fundament/plugin-sdk => ./plugin-sdk`, so
  in-repo consumers always build against the working tree.

## Releasing

Go resolves a nested module from a `<subdir>/vX.Y.Z` tag, so a release is just a
tag. Run the **Release plugin-sdk** workflow with the version to publish; it
validates the version, builds and tests the module, then pushes
`plugin-sdk/vX.Y.Z`.

When you release, bump `DefaultSDKVersion` in
[`functl/pkg/scaffold/version.go`](../functl/pkg/scaffold/version.go) so newly
scaffolded plugins pin the new release.

Keep the `go` directive in `go.mod` as low as the dependencies allow: it is the
Go version floor for every third-party plugin author.

## License

Apache-2.0 ([LICENSE](LICENSE)), unlike the AGPL-licensed rest of this
repository. Writing a Fundament plugin does not make your plugin a derivative
work of the platform.
