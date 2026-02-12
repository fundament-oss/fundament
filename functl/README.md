# functl

CLI for managing Fundament platform resources using an API key.

## Installation

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

Configuration files are stored in `~/.fundament/`:

| File | Description |
|------|-------------|
| `config.yaml` | API endpoints and default settings |
| `credentials` | Stored API key (created after login) |

### Configuration file

Create `~/.fundament/config.yaml` to override defaults:

```yaml
api_endpoint: http://organization.fundament.localhost:8080
authn_url: http://authn.fundament.localhost:8080
output: table
```

### Environment variables

| Variable | Description |
|----------|-------------|
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
