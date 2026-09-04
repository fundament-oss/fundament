# Demo

A demo plugin.

A [Fundament](https://github.com/fundament-oss/fundament) plugin, scaffolded with
`functl plugin create`.

## Layout

| Path                 | What it is                                                     |
| -------------------- | -------------------------------------------------------------- |
| `definition.yaml`    | The plugin manifest: metadata, RBAC, console menu and components |
| `plugin.go`          | The plugin itself                                                |
| `main.go`            | Hands the plugin to the SDK runtime                              |
| `Justfile`           | Build, test, lint and image recipes                              |
| `console.go`         | Embeds the console UI assets                                     |
| `console/`           | Console UI pages                       |

## Develop

Recipes are in the `Justfile`; run `just --list` to see them all. Install
[just](https://just.systems) if you do not have it (`brew install just`,
`cargo install just`, or your package manager).

```shell
go mod tidy      # needs network access, or a warm module cache
just build
just test
```

```shell
just docker
```

## Fill in

- `definition.yaml`: `spec.permissions.rbac` decides what the plugin may do in
  the cluster. It is materialised verbatim as a ClusterRole, so grant only what
  the plugin uses.
- `spec.menu`, `spec.customComponents`, `spec.allowedResources` and the files
  under `console/` are one contract. `definition_test.go` checks that the files
  referenced actually exist; nothing checks the rest, so change them together.
- Every `TODO` marker in this project.

`metadata.license` says `Apache-2.0`. Add the licence text as a `LICENSE` file.

## Publish

Publishing a standalone plugin is **not supported yet**. Today a plugin
definition is published from inside the Fundament monorepo with
`just plugin-publish`, which requires the plugin to live under `plugins/` and to
have a catalog entry in the appstore. Until `functl plugin publish` exists, you
can build and run this plugin, but not install it into a Fundament cluster.

## Docs

<https://github.com/fundament-oss/fundament/tree/master/docs/developer/plugins>
