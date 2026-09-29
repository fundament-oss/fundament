---
title: functl CLI
sidebar:
  order: 10
---

`functl` is the command-line client for the Fundament platform API. It does what
the console does (organizations, projects, namespaces, clusters, members and
API keys) from a terminal or a CI pipeline.

`functl <command> --help` is the reference for every command's arguments and
flags; this page covers setup and the things `--help` does not tell you. The
[functl README](https://github.com/fundament-oss/fundament/blob/master/functl/README.md)
has the full option list of `functl plugin create`.

## Install

Prebuilt binaries for Linux (amd64, arm64), macOS (Apple Silicon) and Windows
(amd64) are published to the rolling `functl-latest` release:

```bash
# Pick your platform: linux_amd64, linux_arm64, or darwin_arm64
curl -fsSL https://github.com/fundament-oss/fundament/releases/download/functl-latest/functl_linux_amd64.tar.gz \
  | tar -xzf - functl
sudo mv functl /usr/local/bin/
```

On Windows, download the `.zip` instead:

```powershell
Invoke-WebRequest -Uri https://github.com/fundament-oss/fundament/releases/download/functl-latest/functl_windows_amd64.zip -OutFile functl.zip
Expand-Archive functl.zip -DestinationPath .
```

Checksums are published alongside the archives as `SHA256SUMS`. Verify the
install with `functl version`.

## Point functl at the sandbox

Without a config file, `functl` talks to the `fundament-poc.nl` installation.
An API key only works on the installation it was created on, so to use a key
from the [sandbox console](https://console.fundament.projects.digilab.network/api-keys),
create `~/.config/fundament/config.yaml` first:

```yaml
api_endpoint: https://organization.fundament.projects.digilab.network
authn_url: https://authn.fundament.projects.digilab.network
registry_url: https://marketplace-registry-api.fundament.projects.digilab.network
```

Without it, `functl auth login` sends the key to `fundament-poc.nl`, which
rejects it. See
[Configuration](#configuration) for the other settings.

## Authenticate

`functl` authenticates with an [API key](./api-keys.md):

```bash
functl auth login            # prompts for the key
functl auth login <API_KEY>  # or pass it directly
functl auth status           # show who you are
functl auth logout           # remove stored credentials
```

The key is stored in `~/.config/fundament/credentials`. Setting
`FUNDAMENT_API_KEY` in the environment takes precedence over the stored key,
which is what you want in CI.

## Select an organization

Most commands work within one organization. Select it by its ID, which
`functl org list` shows:

```bash
functl org list
functl org set <ORG_ID>   # remembered for subsequent commands
functl org unset
```

To use another organization for a single command, pass `--org=<ORG_ID>`
instead, for example in CI.

## Commands

| Group | Subcommands |
| --- | --- |
| `functl auth` | `login`, `status`, `logout` |
| `functl org` | `list`, `set`, `unset`, `member list\|invite\|update-permission\|remove` |
| `functl project` | `list`, `get`, `create`, `update`, `member list\|add\|update-role\|remove` |
| `functl namespace` | `list`, `create`, `delete` |
| `functl cluster` | `list`, `get`, `kubeconfig`, `token` |
| `functl apikey` | `list`, `create`, `revoke`, `delete` |
| `functl config` | `dir`, `path` |
| `functl plugin` | `create`, `publish` |
| `functl version` | none |

Things to know before you run them:

- Commands take IDs, not names: organization, cluster, project, namespace, API
  key and user IDs. The `list` commands show them.
- Projects belong to a cluster: `functl project list <CLUSTER_ID>` and
  `functl project create <CLUSTER_ID> <NAME>`.
- `functl namespace list` needs `--cluster=<CLUSTER_ID>` or
  `--project=<PROJECT_ID>`.
- Member commands identify the member with `--user-id=<USER_ID>`, from
  `functl org member list` or `functl project member list <PROJECT_ID>`.
  Project members also take `--role`, organization members `--permission`.
- `org member remove` and `project member remove` ask for confirmation; pass
  `--yes` to skip it in scripts.

### Plugin development

`functl plugin create <name>` scaffolds a new Fundament plugin project: a
buildable Go project with its manifest, Dockerfile and optionally a console UI.
It runs entirely locally, so unlike every other command it needs no API key and
no organization.

```shell
functl plugin create my-plugin
```

It prompts for the details it needs; pass `--yes` to accept the defaults, or set
each one with a flag (`--template=minimal|helm`, `--console=none|vanilla|vite`,
`--module`, `--crd`, `--kind`, ...) for unattended use. See
[Writing a plugin](../developer/plugins/writing-a-plugin.md).

`functl plugin publish <definition.yaml> --image=<IMAGE>` pushes a plugin
version to the marketplace registry, for the active organization:

- It needs the registry's address in `registry_url` or `FUNCTL_REGISTRY_URL`,
  and refuses to run without it.
- `--image` must be a digest reference (`registry/repo@sha256:...`); a tag such
  as `:1.0` is refused.
- `--create` reserves the listing first if it does not exist yet, and
  `--submit` opens a review round for the pushed version.

### Cluster credentials

`functl cluster kubeconfig` writes a kubeconfig for a cluster, the usual way to
point `kubectl` at a Fundament cluster. `functl cluster token` is the exec
credential plugin that kubeconfig calls: it exchanges your API key for a
short-lived platform token and prints it as an `ExecCredential`. You rarely run
it yourself, but `functl` has to stay installed and logged in for the kubeconfig
to keep working. See [Cluster access](./clusters.md#cluster-access).

## Configuration

`functl` works without a config file, and then uses the `fundament-poc.nl`
installation (`https://organization-api.fundament-poc.nl` and
`https://authn.fundament-poc.nl`). Create `~/.config/fundament/config.yaml` to
target another installation, such as [the sandbox](#point-functl-at-the-sandbox):

```yaml
api_endpoint: https://organization-api.my-own-fundament.example
authn_url: https://authn.my-own-fundament.example
registry_url: https://marketplace-registry-api.my-own-fundament.example
output: table
```

`registry_url` is only needed for `functl plugin publish`; there is no default.

Environment variables override the config file:

| Variable | Description |
| --- | --- |
| `FUNDAMENT_API_KEY` | API key, takes precedence over the credentials file |
| `FUNCTL_API_ENDPOINT` | Organization API endpoint |
| `FUNCTL_AUTHN_URL` | Authentication API endpoint |
| `FUNCTL_REGISTRY_URL` | Marketplace registry API endpoint, for `plugin publish` |
| `FUNCTL_CONFIG_DIR` | Configuration directory (must be absolute) |
| `FUNCTL_DEBUG` | Enable debug logging (same as `--debug`) |

These flags work on every command:

| Flag | Description |
| --- | --- |
| `--org=<ORG_ID>` | Use this organization instead of the one from `functl org set` |
| `-o`, `--output` | `table` (default) or `json` |
| `-d`, `--debug` | Enable debug logging |

Use `functl config dir` and `functl config path` to see what actually resolved.

## Output formats

The default is a human-readable table. For scripting, ask for JSON:

```bash
functl project list <CLUSTER_ID> -o json
```

```json
[
  {
    "id": "019424a8-1234-7000-8000-000000000001",
    "cluster_id": "019424a8-0000-7000-8000-000000000001",
    "name": "my-project",
    "alias": "My project",
    "created": "2026-01-15T10:30:00Z",
    "namespace_count": 2,
    "member_count": 1
  }
]
```

Field names follow the API, in snake_case, and every field is present even when
it is empty.

## See also

- [API keys](./api-keys.md): creating and rotating the key `functl` uses.
- [OpenTofu provider](./opentofu-provider.md): for declarative management
  instead of imperative commands.
