---
title: functl CLI
sidebar:
  order: 10
---

`functl` is the command-line client for the Fundament platform API. It does what
the console does — organizations, projects, namespaces, clusters, members and
API keys — from a terminal or a CI pipeline.

The [functl README](https://github.com/fundament-oss/fundament/blob/master/functl/README.md)
is the canonical reference for flags and options; this page is the short version.

## Install

Prebuilt binaries for Linux (amd64, arm64) and macOS (Apple Silicon) are
published to the rolling `functl-latest` release:

```bash
# Pick your platform: linux_amd64, linux_arm64, or darwin_arm64
curl -fsSL https://github.com/fundament-oss/fundament/releases/download/functl-latest/functl_linux_amd64.tar.gz \
  | tar -xzf - functl
sudo mv functl /usr/local/bin/
```

Checksums are published alongside the archives as `SHA256SUMS`. Verify the
install with `functl version`.

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

Most commands work within one organization:

```bash
functl org list
functl org set <ORG>     # remembered for subsequent commands
functl org unset
```

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
| `functl version` | — |

Run `functl <group> --help` for the flags of any individual command.

### Cluster credentials

`functl cluster kubeconfig` writes a kubeconfig for a cluster — the usual way to
point `kubectl` at a Fundament cluster. `functl cluster token` is the exec
credential plugin that kubeconfig calls: it exchanges your API key for a
short-lived platform token and prints it as an `ExecCredential`. You rarely run
it yourself, but `functl` has to stay installed and logged in for the kubeconfig
to keep working. See [Cluster access](./clusters.md#cluster-access).

## Configuration

`functl` works without a config file; the built-in defaults point at the
deployed environment. Create `~/.config/fundament/config.yaml` only to target a
different installation:

```yaml
api_endpoint: https://organization-api.my-own-fundament.example
authn_url: https://authn.my-own-fundament.example
output: table
```

Environment variables override the config file:

| Variable | Description |
| --- | --- |
| `FUNDAMENT_API_KEY` | API key, takes precedence over the credentials file |
| `FUNCTL_API_ENDPOINT` | Organization API endpoint |
| `FUNCTL_AUTHN_URL` | Authentication API endpoint |
| `FUNCTL_CONFIG_DIR` | Configuration directory (must be absolute) |
| `FUNCTL_DEBUG` | Enable debug logging (same as `--debug`) |

Use `functl config dir` and `functl config path` to see what actually resolved.

## Output formats

The default is a human-readable table. For scripting, ask for JSON:

```bash
functl project list -o json
```

## See also

- [API keys](./api-keys.md) — creating and rotating the key `functl` uses.
- [OpenTofu provider](./opentofu-provider.md) — for declarative management
  instead of imperative commands.
