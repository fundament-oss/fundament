# functl

CLI for managing Fundament platform resources using an API key.

## Installation

### Download a prebuilt binary

Every push to master publishes binaries for Linux (amd64, arm64), macOS (Apple Silicon) and Windows (amd64) to the rolling [`functl-latest`](https://github.com/fundament-oss/fundament/releases/tag/functl-latest) release:

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

Checksums are published alongside the archives as `SHA256SUMS`. Check the installed build with `functl version`.

### Build from source

```bash
go build -o functl ./functl/cmd/functl
```

### Using Just

```bash
just functl --help
```

Or run commands directly:

```bash
just functl auth status
```

## Configuration

Configuration files are stored in `~/.config/fundament/` by default, following the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir/latest/):

| File | Description |
|------|-------------|
| `config.yaml` | API endpoints and default settings |
| `credentials` | Stored API key (created after login) |

The config directory is resolved in this order:

1. `FUNCTL_CONFIG_DIR` environment variable (explicit override)
2. `XDG_CONFIG_HOME/fundament` (XDG spec)
3. `%APPDATA%/fundament` (Windows default)
4. `~/.config/fundament` (Linux/macOS fallback)

Use `functl config dir` to see the resolved directory, or `functl config path` for the config file path.

### Configuration file

functl works without a config file: the built-in defaults point at the
deployed environment (`https://organization-api.fundament-poc.nl` and
`https://authn.fundament-poc.nl`). Create `~/.config/fundament/config.yaml`
only to point at another environment or change other settings:

```yaml
api_endpoint: https://organization-api.my-own-fundament.example
authn_url: https://authn.my-own-fundament.example
output: table
```

For local development the repo's `mise.toml` sets `FUNCTL_API_ENDPOINT` and
`FUNCTL_AUTHN_URL` to the local skaffold endpoints, which override both the
defaults and the config file.

### Environment variables

| Variable | Description |
|----------|-------------|
| `FUNCTL_CONFIG_DIR` | Override the configuration directory path (must be absolute) |
| `FUNCTL_API_ENDPOINT` | Override the organization API endpoint (takes precedence over config file) |
| `FUNCTL_AUTHN_URL` | Override the authn API endpoint (takes precedence over config file) |
| `FUNCTL_DEBUG` | Enable debug logging (same as `--debug`) |
| `FUNDAMENT_API_KEY` | API key for authentication (takes precedence over credentials file) |

## Authentication

Before using most commands, you need to authenticate with an API key.

### Login

```bash
# Interactive prompt for API key
functl auth login

# Or provide the API key directly
functl auth login <API_KEY>
```

## Commands

### Global flags

| Flag | Short | Description |
|------|-------|-------------|
| `--debug` | `-d` | Enable debug logging |
| `--output` | `-o` | Output format: `table` (default) or `json` |
| `--help` | `-h` | Show help |

### `functl plugin create`

Scaffolds a new, standalone Fundament plugin project. It writes files and nothing
else -- no network, no API key, no organization -- so it works before you have an
account.

```shell
functl plugin create my-plugin
```

Prompts for anything not passed as a flag when stdin is a terminal; otherwise
takes the defaults, so it is safe to script.

| Flag | Description | Default |
|------|-------------|---------|
| `<name>` | Plugin name: a lowercase DNS label, at most 56 characters | required |
| `--display-name` | Human-readable name shown in the console | title-cased name |
| `--description` | One-line description | `A Fundament plugin.` |
| `--author` | Plugin author | `git config user.name` |
| `--license` | SPDX license identifier | `Apache-2.0` |
| `--module` | Go module path | derived from the git remote |
| `--template` | `minimal` or `helm` | `minimal` |
| `--console` | `none`, `vanilla` or `vite` | `none` |
| `--crd` | Custom resource as `<plural>.<group>` | `<name>s.example.com` |
| `--kind` | Kind of that custom resource | UpperCamelCase name |
| `--dir` | Where to write the project | `./<name>` |
| `--sdk-version` | fundament version to pin for the plugin SDK | the version baked into functl |
| `--[no-]git` | Run `git init` | on |
| `--[no-]tidy` | Run `go mod tidy` | on |
| `--force` | Write into a non-empty directory | off |
| `--yes`, `-y` | Accept all defaults without prompting | off |

The 56-character limit is not arbitrary: the plugin controller prefixes `plugin-`
to derive the plugin's namespace and other resource names, and those must fit
Kubernetes' 63-character DNS-label limit.

## Output formats

### Table (default)

Human-readable tabular format:

```bash
functl project list
```

```
ID                                      NAME            CREATED
019424a8-1234-7000-8000-000000000001    my-project      2024-01-15 10:30:00
019424a8-5678-7000-8000-000000000002    another-proj    2024-01-16 14:22:00
```

### JSON

Machine-readable JSON format for scripting:

```bash
functl project list -o json
```

```json
[
  {
    "id": "019424a8-1234-7000-8000-000000000001",
    "name": "my-project",
    "created": "2024-01-15T10:30:00Z"
  }
]
```
