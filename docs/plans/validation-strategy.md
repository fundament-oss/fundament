# Validation Strategy

This document outlines the research and proposed strategy for handling validation across the Fundament stack, addressing the drift problem identified in [PR #12](https://github.com/fundament-oss/fundament/pull/12#issuecomment-3701925545).

## Problem Statement

Validation rules are currently defined in three separate places:

| Layer | How | Example |
|-------|-----|---------|
| Proto | Field types only | `string name = 1;` |
| Go models | Struct tags | `validate:"required,max=255"` |
| Database | Constraints | `NOT NULL`, `CHECK`, `UNIQUE` |

These can **drift apart** over time, leading to:
- Inconsistent error messages across layers
- Silent data corruption if DB is more permissive than app
- Duplicated validation logic when building a CLI

### Example of Drift

| Rule | Proto | Go Model | Database |
|------|-------|----------|----------|
| Name required | (implicit) | `validate:"required"` | `NOT NULL` |
| Name max length | none | `max=255` | `text` (unlimited) |
| Name uniqueness | none | none | `UNIQUE` constraint |

## Proposed Solution: Protovalidate

Use [protovalidate](https://github.com/bufbuild/protovalidate) to make **Proto the single source of truth** for validation rules.

### What is Protovalidate?

Protovalidate is Buf's semantic validation library for Protocol Buffers. It allows defining validation rules directly in `.proto` files using annotations, which are then enforced at runtime.

Key features:
- Standard annotations for common rules (required, min/max length, regex patterns, UUIDs, emails)
- [CEL (Common Expression Language)](https://cel.dev/) for complex custom rules
- Runtime validation (no code generation needed)
- Multi-language support (Go, Java, Python, TypeScript, C++)

### Example Proto with Validation

```protobuf
syntax = "proto3";

import "buf/validate/validate.proto";

message CreateClusterRequest {
  string name = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.min_len = 1,
    (buf.validate.field).string.max_len = 255,
    (buf.validate.field).string.pattern = "^[a-z][a-z0-9-]*$"
  ];

  string region = 2 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.in = "NL1,NL2,NL3"
  ];

  string kubernetes_version = 3 [
    (buf.validate.field).required = true,
    (buf.validate.field).string.pattern = "^1\\.(2[5-9]|3[0-9])\\.[0-9]+$"
  ];
}
```

### Setup

1. Add protovalidate dependency to `buf.yaml`:

```yaml
version: v2
modules:
  - path: .
deps:
  - buf.build/bufbuild/protovalidate
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

2. Run `buf mod update` to fetch the dependency

3. Install Go packages:

```bash
go get buf.build/go/protovalidate
go get connectrpc.com/validate
```

## Architecture

### Shared Validation Across Entry Points

```
┌─────────────────────────────────────────────────────────┐
│              Proto Files (Single Source)                │
│   *.proto files with buf.validate annotations           │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
            ┌─────────────────────┐
            │  Generated Go Code  │
            │  (shared module)    │
            └──────────┬──────────┘
                       │
         ┌─────────────┼─────────────┐
         ▼                           ▼
   ┌───────────┐               ┌───────────┐
   │    CLI    │               │    API    │
   │ validates │               │ validates │
   │ locally   │               │ on server │
   └─────┬─────┘               └─────┬─────┘
         │                           │
         └───────────┬───────────────┘
                     ▼
              ┌───────────┐
              │ Database  │
              │ enforces  │
              │ (last     │
              │  resort)  │
              └───────────┘
```

### API Server Integration

Use Connect's validate interceptor for automatic validation:

```go
import (
    "connectrpc.com/connect"
    "connectrpc.com/validate"
    "buf.build/go/protovalidate"
)

func main() {
    validator, err := protovalidate.New()
    if err != nil {
        log.Fatal(err)
    }

    interceptor := validate.NewInterceptor(validator)

    mux := http.NewServeMux()
    mux.Handle(organizationv1connect.NewClusterServiceHandler(
        &clusterServer{},
        connect.WithInterceptors(interceptor),
    ))
}
```

This removes the need for manual validation calls in handlers.

### CLI Integration

The CLI imports the same proto-generated code and validates locally:

```go
import (
    "buf.build/go/protovalidate"
    organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func createClusterCmd(name, region, k8sVersion string) error {
    // Build the proto message (same type as API uses)
    req := &organizationv1.CreateClusterRequest{
        Name:              name,
        Region:            region,
        KubernetesVersion: k8sVersion,
    }

    // Validate locally before calling API
    validator, _ := protovalidate.New()
    if err := validator.Validate(req); err != nil {
        // Instant feedback, no network round-trip
        return fmt.Errorf("invalid input: %w", err)
    }

    // Call API (which validates again - defense in depth)
    client := organizationv1connect.NewClusterServiceClient(httpClient, baseURL)
    resp, err := client.CreateCluster(ctx, connect.NewRequest(req))
    return err
}
```

Benefits:
- Instant local validation feedback
- No network round-trip for invalid input
- Consistent error messages with API
- Rules defined once, used everywhere

## Database Constraints

Protovalidate doesn't automatically sync with database constraints. We need a two-pronged approach:

### 1. Add CHECK Constraints for Critical Rules

```sql
-- Add constraints that mirror proto validation
ALTER TABLE organization.clusters
ADD CONSTRAINT clusters_name_length
CHECK (length(name) <= 255);

ALTER TABLE organization.clusters
ADD CONSTRAINT clusters_name_pattern
CHECK (name ~ '^[a-z][a-z0-9-]*$');
```

### 2. Integration Tests to Detect Drift

Use [Atlas](https://atlasgo.io/) for programmatic schema introspection:

```go
// validation_sync_test.go
package organization_test

import (
    "context"
    "database/sql"
    "os"
    "testing"

    "ariga.io/atlas/sql/postgres"
    "ariga.io/atlas/sql/schema"
)

func TestValidationRulesMatchDatabase(t *testing.T) {
    ctx := context.Background()

    // Connect and inspect DB schema
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    drv, err := postgres.Open(db)
    if err != nil {
        t.Fatal(err)
    }

    sch, err := drv.InspectSchema(ctx, "organization", nil)
    if err != nil {
        t.Fatal(err)
    }

    // Get clusters table
    tbl, ok := sch.Table("clusters")
    if !ok {
        t.Fatal("clusters table not found")
    }

    // Assert constraints match proto validation rules
    nameCol, ok := tbl.Column("name")
    if !ok {
        t.Fatal("name column not found")
    }

    // Check NOT NULL matches proto required
    if nameCol.Type.Null {
        t.Error("DB allows NULL for 'name' but proto marks it required")
    }

    // Check for length constraint
    hasLengthCheck := false
    for _, check := range tbl.Attrs {
        if c, ok := check.(*schema.Check); ok {
            if strings.Contains(c.Expr, "length(name)") {
                hasLengthCheck = true
            }
        }
    }
    if !hasLengthCheck {
        t.Error("Missing CHECK constraint for name length (proto max_len = 255)")
    }
}
```

Run this test in CI to catch drift between proto rules and DB constraints.

## Migration Plan

### Phase 1: Add Protovalidate

1. Update `buf.yaml` with protovalidate dependency
2. Add validation annotations to existing proto messages
3. Add Connect validate interceptor to API servers
4. Remove `pkg/models/*.go` validation structs
5. Remove `common/validate/` package

### Phase 2: Database Constraints

1. Add CHECK constraints to match critical proto rules
2. Create migration files for constraint changes
3. Update schema documentation

### Phase 3: Integration Tests

1. Add Atlas-based schema introspection tests
2. Add tests comparing proto rules to DB constraints
3. Run tests in CI pipeline

### Phase 4: CLI (Future)

1. CLI imports shared proto module
2. Use protovalidate for local validation
3. Consistent UX with API error messages

## What Gets Removed

After implementing protovalidate:

| File/Package | Status |
|--------------|--------|
| `organization-api/pkg/models/cluster.go` | Delete |
| `organization-api/pkg/models/tenant.go` | Delete |
| `common/validate/validate.go` | Delete |
| Manual `validator.Validate()` calls in handlers | Remove |

## Comparison

| Aspect | Current (struct tags) | With Protovalidate |
|--------|----------------------|-------------------|
| Source of truth | 3 places | 1 place (proto) |
| CLI validation | Must duplicate | Share proto |
| Feedback speed | Requires API call | Instant, local |
| Error messages | Inconsistent | Consistent |
| Multi-language | Reimplement | Native support |
| Boilerplate | High (models pkg) | Low (interceptor) |

## References

- [protovalidate](https://github.com/bufbuild/protovalidate) - Main repository
- [protovalidate-go](https://github.com/bufbuild/protovalidate-go) - Go implementation
- [protovalidate.com](https://protovalidate.com/) - Documentation
- [Connect validate interceptor](https://connectrpc.com/docs/go/getting-started/) - Connect integration
- [Atlas](https://atlasgo.io/) - Database schema inspection
- [CEL](https://cel.dev/) - Custom expression language for complex rules
