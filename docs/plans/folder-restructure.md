# Folder Restructure Plan (Minimal Code Changes)

This document outlines a **minimal-change** approach to folder restructuring. The goal is to achieve standard Go project layout without rewriting service internals.

## Approach

**What we DO:**
- Move files to standard Go locations
- Update import paths (find/replace)
- Consolidate proto definitions
- Consolidate build artifacts

**What we DON'T do (can be a future phase):**
- Refactor to hexagonal architecture (domain/port/app/adapter)
- Restructure internal service code
- Change how services work internally

## Goals (Minimal Scope)

1. **Standard Go layout** — `cmd/`, `internal/`, `pkg/` structure
2. **Shared proto definitions** — Single `proto/` directory
3. **Consolidated build artifacts** — `build/docker/`, `build/hotreload/`
4. **Cleaner frontend location** — `web/` directory

## Current Structure

```
fundament/
├── authn-api/
│   ├── cmd/fun-authn-api/main.go
│   ├── pkg/authn/              # auth.go, handler.go, session.go
│   ├── pkg/authnhttp/          # OpenAPI generated
│   ├── pkg/db/                 # sqlc queries + gen/
│   ├── pkg/proto/              # buf config + gen/
│   ├── hotreload/
│   └── Dockerfile
├── organization-api/
│   ├── cmd/fun-organization-api/main.go
│   ├── pkg/organization/       # auth.go, handler.go, oganization.go
│   ├── pkg/db/                 # sqlc queries + gen/
│   ├── pkg/proto/              # buf config + gen/
│   ├── hotreload/
│   └── Dockerfile
├── appstore-api/
│   ├── cmd/fun-appstore-api/main.go
│   ├── hotreload/
│   └── Dockerfile
├── common/psqldb/
├── login-frontend/
├── console-frontend/
├── db/
├── charts/
└── deploy/
```

## Target Structure (Minimal Change)

```
fundament/
├── cmd/                              # Entry points only
│   ├── authn-api/main.go
│   ├── organization-api/main.go
│   └── appstore-api/main.go
│
├── internal/                         # Service implementations (moved as-is)
│   ├── authn/                        # All authn-api/pkg/* content
│   │   ├── authn/                    # auth.go, handler.go, session.go
│   │   ├── http/                     # authnhttp → http (OpenAPI generated)
│   │   └── db/                       # queries.sql, sqlc.yaml, gen/
│   │
│   ├── organization/                 # All organization-api/pkg/* content
│   │   ├── organization/             # auth.go, handler.go, oganization.go
│   │   └── db/                       # queries.sql, sqlc.yaml, gen/
│   │
│   └── appstore/                     # Future expansion
│
├── pkg/                              # Public shared code
│   └── psqldb/                       # common/psqldb → pkg/psqldb
│
├── proto/                            # Consolidated proto definitions
│   ├── buf.yaml
│   ├── buf.gen.yaml
│   ├── authn/v1/authn.proto
│   └── organization/v1/organization.proto
│
├── gen/                              # All generated code
│   └── proto/
│       ├── authn/v1/
│       │   ├── authn.pb.go
│       │   └── authnv1connect/
│       └── organization/v1/
│           ├── organization.pb.go
│           └── organizationv1connect/
│
├── build/                            # Build artifacts
│   ├── docker/
│   │   ├── authn-api.Dockerfile
│   │   ├── organization-api.Dockerfile
│   │   └── appstore-api.Dockerfile
│   └── hotreload/
│       ├── authn-api/
│       ├── organization-api/
│       └── appstore-api/
│
├── deployments/                      # Deployment configs
│   ├── charts/fundament/
│   └── k3d/
│
├── web/                              # Frontend applications
│   ├── login/
│   └── console/
│
├── db/                               # Database (unchanged)
├── docs/
├── go.mod
├── skaffold.yaml
├── Justfile
└── mise.toml
```

## Migration Steps

### Phase 1: Create directory structure & move non-Go files

```bash
# Create new directories
mkdir -p cmd/{authn-api,organization-api,appstore-api}
mkdir -p internal/{authn,organization,appstore}
mkdir -p internal/authn/{authn,http,db}
mkdir -p internal/organization/{organization,db}
mkdir -p pkg/psqldb
mkdir -p proto/{authn,organization}/v1
mkdir -p gen/proto
mkdir -p build/{docker,hotreload}
mkdir -p deployments
mkdir -p web
```

### Phase 2: Move proto definitions (consolidate)

```bash
# Move proto source files
mv authn-api/pkg/proto/authn/v1/authn.proto proto/authn/v1/
mv organization-api/pkg/proto/organization/v1/organization.proto proto/organization/v1/

# Create unified buf.yaml (merge from existing)
# Create unified buf.gen.yaml with output to gen/proto/
```

**buf.gen.yaml changes:**
```yaml
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/fundament-oss/fundament/gen/proto
plugins:
  - remote: buf.build/protocolbuffers/go
    out: ../gen/proto
    opt: paths=source_relative
  - remote: buf.build/connectrpc/go
    out: ../gen/proto
    opt: paths=source_relative
```

### Phase 3: Move Go source files

```bash
# Entry points
mv authn-api/cmd/fun-authn-api/main.go cmd/authn-api/main.go
mv organization-api/cmd/fun-organization-api/main.go cmd/organization-api/main.go
mv appstore-api/cmd/fun-appstore-api/main.go cmd/appstore-api/main.go

# Service code (as-is, no restructuring)
mv authn-api/pkg/authn/* internal/authn/authn/
mv authn-api/pkg/authnhttp/* internal/authn/http/
mv authn-api/pkg/db/* internal/authn/db/

mv organization-api/pkg/organization/* internal/organization/organization/
mv organization-api/pkg/db/* internal/organization/db/

# Shared code
mv common/psqldb/* pkg/psqldb/
```

### Phase 4: Move build artifacts

```bash
# Dockerfiles
mv authn-api/Dockerfile build/docker/authn-api.Dockerfile
mv organization-api/Dockerfile build/docker/organization-api.Dockerfile
mv appstore-api/Dockerfile build/docker/appstore-api.Dockerfile

# Hotreload configs
mv authn-api/hotreload build/hotreload/authn-api
mv organization-api/hotreload build/hotreload/organization-api
mv appstore-api/hotreload build/hotreload/appstore-api

# Deployments
mv charts deployments/charts
mv deploy/k3d deployments/k3d 2>/dev/null || true
```

### Phase 5: Move frontends

```bash
mv login-frontend web/login
mv console-frontend web/console
```

### Phase 6: Update import paths

This is the main "code change" - find and replace import paths.

| Old Import | New Import |
|------------|------------|
| `github.com/fundament-oss/fundament/authn-api/pkg/authn` | `github.com/fundament-oss/fundament/internal/authn/authn` |
| `github.com/fundament-oss/fundament/authn-api/pkg/authnhttp` | `github.com/fundament-oss/fundament/internal/authn/http` |
| `github.com/fundament-oss/fundament/authn-api/pkg/db/gen` | `github.com/fundament-oss/fundament/internal/authn/db/gen` |
| `github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1` | `github.com/fundament-oss/fundament/gen/proto/authn/v1` |
| `github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect` | `github.com/fundament-oss/fundament/gen/proto/authn/v1/authnv1connect` |
| `github.com/fundament-oss/fundament/organization-api/pkg/organization` | `github.com/fundament-oss/fundament/internal/organization/organization` |
| `github.com/fundament-oss/fundament/organization-api/pkg/db/gen` | `github.com/fundament-oss/fundament/internal/organization/db/gen` |
| `github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/organization/v1` | `github.com/fundament-oss/fundament/gen/proto/organization/v1` |
| `github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/organization/v1/organizationv1connect` | `github.com/fundament-oss/fundament/gen/proto/organization/v1/organizationv1connect` |
| `github.com/fundament-oss/fundament/common/psqldb` | `github.com/fundament-oss/fundament/pkg/psqldb` |

**Files to update:**
- `cmd/authn-api/main.go` (4 imports)
- `cmd/organization-api/main.go` (3 imports)
- `internal/authn/authn/*.go` (internal refs + proto)
- `internal/authn/http/*.go` (if any refs)
- `internal/organization/organization/*.go` (internal refs + proto)

### Phase 7: Update build configs

**Dockerfiles** - Update build paths:
```dockerfile
# build/docker/authn-api.Dockerfile
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /authn-api ./cmd/authn-api
```

**skaffold.yaml** - Update artifact paths:
```yaml
build:
  artifacts:
    - image: fundament/authn-api
      context: .
      docker:
        dockerfile: build/docker/authn-api.Dockerfile
```

**Hotreload configs** - Update watched paths and build commands.

**Helm chart** - May need path updates if referencing build context.

### Phase 8: Regenerate code

```bash
# Regenerate proto
cd proto && buf generate

# Regenerate sqlc (configs stay with service)
cd internal/authn/db && sqlc generate
cd internal/organization/db && sqlc generate
```

### Phase 9: Cleanup

```bash
# Remove old directories (after verification)
rm -rf authn-api organization-api appstore-api common
rm -rf login-frontend console-frontend
rm -rf charts deploy
```

## Verification

- [ ] `go build ./cmd/...` succeeds
- [ ] `go test ./internal/...` passes
- [ ] `buf generate` works in `proto/`
- [ ] `sqlc generate` works in each `internal/*/db/`
- [ ] `skaffold dev` deploys successfully
- [ ] All services respond correctly
- [ ] Frontends build and work

## Future Phase: Hexagonal Architecture

After the folder restructure is stable, a separate effort can refactor service internals:

```
internal/authn/
├── domain/          # Entities, errors
├── port/            # Interfaces
├── app/             # Use cases
└── adapter/         # HTTP, gRPC, postgres implementations
```

This would be a more invasive change and can be done incrementally per-service.

## Summary of Code Changes

| Change Type | Files Affected | Complexity |
|-------------|----------------|------------|
| Import path updates | ~10 Go files | Find/replace |
| Dockerfile paths | 3 files | Simple |
| skaffold.yaml | 1 file | Simple |
| buf.gen.yaml | 1 file (new consolidated) | Simple |
| sqlc.yaml | 2 files (output path) | Simple |
| Justfile | 1 file | Simple |

**Total estimated: ~20 files with simple find/replace changes, no logic changes.**
