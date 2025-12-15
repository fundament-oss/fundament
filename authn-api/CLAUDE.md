# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the service locally (from repo root)
just authn-api

# Or run directly
cd authn-api && JWT_SECRET=abc LISTEN_ADDR=:10100 go run .

# Lint Go code
cd authn-api && golangci-lint run ./...

# Generate protobuf code
cd authn-api/proto && buf generate

# Generate sqlc database code
cd authn-api/sqlc && sqlc generate

# Generate all code (proto + sqlc) from repo root
just generate
```

## Required Services

- **PostgreSQL**: `just db` (runs on port 5432)
- **Dex OIDC provider**: `just dex` (runs on port 5556)

## Architecture

This is a Go authentication service using:
- **HTTP endpoints** for browser-facing auth flow (login, callback, refresh, logout)
- **Connect RPC** for service-to-service API (`GetUserInfo`)
- **OIDC** via coreos/go-oidc for external identity provider integration
- **JWT** for internal session tokens (HS256 signed)
- **Gorilla Sessions** for OAuth state management (CSRF protection)
- **PostgreSQL** with pgx/v5 driver and sqlc for type-safe queries

### Key Files

- `main.go` - Server setup, OIDC provider initialization, HTTP routing
- `handler.go` - HTTP handlers for auth flow + RPC handler for GetUserInfo
- `auth.go` - JWT generation/validation, cookie signing with HMAC-SHA256
- `session.go` - Gorilla Sessions wrapper for OAuth state management
- `storage.go` - PostgreSQL connection pool management
- `config/config.go` - Environment variable configuration

### Authentication Flow

1. User visits `/login` → state stored in session cookie → redirect to OIDC provider
2. OIDC provider authenticates user → redirects to `/callback` with code and state
3. `/callback` verifies state, exchanges code for tokens, creates/updates user in DB
4. JWT set in `fundament_auth` cookie → user redirected to frontend
5. Frontend can call `GetUserInfo` RPC (via cookie or Bearer token) to get user data

### Endpoints

**HTTP (browser auth):**
- `GET /login` - Initiates OIDC login flow
- `GET /callback` - Handles OIDC redirect
- `POST /refresh` - Refreshes JWT token
- `POST /logout` - Clears auth cookie

**RPC (Connect):**
- `GetUserInfo` - Returns user info from valid JWT

### Proto and Database Code Generation

- Proto files: `proto/authn/v1/authn.proto` → generates to `proto/gen/`
- SQL queries: `sqlc/queries.sql` → generates to `sqlc/db/`
- Schema source: `../db/fundament.sql`

## Environment Variables

Required: `JWT_SECRET`

Optional with defaults:
- `OIDC_ISSUER` (http://localhost:5556)
- `OIDC_CLIENT_ID` (authn-api)
- `OIDC_REDIRECT_URL` (http://localhost:10100/callback)
- `FRONTEND_URL` (http://localhost:5173)
- `DATABASE_URL` (postgres://authn_api:password@localhost:5432/fundament)
- `LISTEN_ADDR` (:8080)
- `LOG_LEVEL` (info)
- `CORS_ALLOWED_ORIGINS` (http://localhost:5173)
- `COOKIE_DOMAIN` (localhost)
- `COOKIE_SECURE` (false)
