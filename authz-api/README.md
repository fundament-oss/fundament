Authz API
---

Tells the console which actions the caller may take. It depends only on OpenFGA and has no database.

`AuthzService.CheckPermissions` takes up to 500 `(action, object_id)` checks and returns one `allowed` per check, in request order. The caller is always the user in the validated JWT; the request carries no identity and no organization. An object the caller has no relation to is reported as not allowed.

The action to relation and object type mapping is `common/authz/actions`.

# Configuration

| Variable | Default | |
| --- | --- | --- |
| `OPENFGA_API_URL` | required | OpenFGA endpoint |
| `OPENFGA_STORE_NAME` | `fundament` | Store resolved by name |
| `JWT_SECRET` | required | Verifies user tokens |
| `LISTEN_ADDR` | `:8080` | |
| `LOG_LEVEL` | `info` | |
| `CORS_ALLOWED_ORIGINS` | | Comma-separated |
| `DEPLOYMENT_VERSION` | `unknown` | Served on `/version` |

Health endpoints: `/livez`, `/readyz` (OpenFGA store reachable), `/version`.

# Tests

The tests run against an in-process OpenFGA (`common/authz/authztest`).

```shell
go test ./authz-api/... ./common/authz/...
```
