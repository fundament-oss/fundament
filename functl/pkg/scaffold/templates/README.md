# Plugin project templates

Rendered by `functl plugin create` (see `../scaffold.go`).

## Rules

- **Every file ends in `.tmpl`**, which is stripped when written. No exceptions:
  a file literally named `go.mod` would exclude this directory from the functl
  module, and a `*.go` file would be compiled and vetted as part of functl
  itself.
- Paths are rendered through `text/template` too, so a filename can depend on the
  resource being scaffolded (`console/{{.ResourcePlural}}-list.html.tmpl`).
- Templates render with `missingkey=error`. The available fields are the
  `data` struct in `../scaffold.go`.
- To emit a literal `{{`, write `{{"{{"}}`.
- `text/template`, never `html/template`: these are source files, and HTML
  escaping would mangle the JS and YAML.

## Sets

| Directory         | When                        |
| ----------------- | --------------------------- |
| `base/`           | always                      |
| `plugin-minimal/` | `--template=minimal`        |
| `plugin-helm/`    | `--template=helm`           |
| `console-vanilla/`| `--console=vanilla`         |
| `console-vite/`   | `--console=vite`            |

## Upstream

These were ported by hand from the first-party plugins, and nothing keeps them in
sync. When the SDK's JS API or the NLDD loading convention changes, update both:

| Template            | Ported from                                    |
| ------------------- | ---------------------------------------------- |
| `plugin-helm/`      | `plugins/cert-manager/plugin.go`                |
| `base/definition.yaml.tmpl` (helm RBAC) | `plugins/cert-manager/definition.yaml` |
| `console-vanilla/`  | `plugins/cert-manager/console/`                 |
| `console-vite/`     | `plugins/openfsc/console-ui/`                   |
