# Fundament Plugin SDK

The Go framework a Fundament plugin embeds. It provides the HTTP server, health
probes, the plugin metadata RPC, console asset serving, the reconcile loop and
signal handling, so a plugin only implements its own behaviour.

```
go get github.com/fundament-oss/fundament
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
