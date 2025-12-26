# authn-api

Authentication service for Fundament using OIDC and JWT.

## Endpoints

### HTTP Endpoints (Browser Auth Flow)

| Endpoint    | Method | Description                                      |
|-------------|--------|--------------------------------------------------|
| `/login`    | GET    | Redirects to OIDC provider for authentication    |
| `/callback` | GET    | Handles OIDC redirect, sets auth cookie          |
| `/refresh`  | POST   | Refreshes JWT token, returns new token as JSON   |
| `/logout`   | POST   | Clears auth cookie                               |

### RPC Endpoint (Connect)

| Method      | Description                              |
|-------------|------------------------------------------|
| GetUserInfo | Returns authenticated user info from JWT |

Proto definition: `proto/authn/v1/authn.proto`

## Usage

### Browser Flow

1. Redirect user to `/login`
2. User authenticates with OIDC provider
3. Callback sets `fundament_auth` cookie
4. User is redirected to frontend

### API Usage

```bash
# Initiate login (redirects to OIDC provider)
curl -v http://localhost:10100/login

# Get user info (with cookie)
curl -X POST http://localhost:10100/authn.v1.AuthnService/GetUserInfo \
  -H "Content-Type: application/json" \
  -b "fundament_auth=<cookie>" \
  -d '{}'

# Get user info (with Bearer token)
curl -X POST http://localhost:10100/authn.v1.AuthnService/GetUserInfo \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{}'

# Refresh token
curl -X POST http://localhost:10100/refresh \
  -H "Content-Type: application/json" \
  -b "fundament_auth=<cookie>"

# Logout
curl -X POST http://localhost:10100/logout \
  -b "fundament_auth=<cookie>"
```

## Architecture Decisions

### Backend-driven OAuth Flow

This service uses a **backend-driven OAuth flow** rather than a frontend-driven (SPA) flow:

- The backend handles all communication with the OIDC provider
- The frontend simply redirects users to `/login` and receives them back after authentication
- Tokens are stored in HTTP-only cookies, not in browser storage

**Why this approach:**
- `client_secret` stays on the server, never exposed to the browser
- Tokens in HTTP-only cookies are harder to steal via XSS than localStorage
- Simpler frontend implementation
- More secure for web apps where you control both frontend and backend

**Alternative (frontend-driven):** The frontend communicates directly with the OIDC provider, handles tokens, and sends them to the backend. This is better for decoupled architectures or multiple client types (mobile, CLI) but requires more frontend complexity and exposes tokens to browser-based attacks.

### PKCE (Proof Key for Code Exchange)

This service uses PKCE instead of a `client_secret`. PKCE protects the token exchange by requiring proof that the same client that initiated the auth request is completing it:

1. `/login` generates a random `code_verifier` and sends its SHA256 hash (`code_challenge`) to the OIDC provider
2. `/callback` sends the original `code_verifier` during token exchange
3. The OIDC provider verifies the hash matches before issuing tokens

This eliminates the need to manage and rotate a client secret while providing equivalent security against authorization code interception attacks.

### Token Refresh

The **frontend is responsible** for refreshing tokens before they expire by calling `POST /refresh`. The backend provides the endpoint but does not proactively refresh tokens. The default token expiry is 24 hours.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JWT_SECRET` | Yes | - | Secret key for signing JWT tokens (HS256). Must be set. |
| `OIDC_ISSUER` | No | `http://localhost:5556` | URL of the OIDC provider (e.g., Dex, Keycloak, Auth0). |
| `OIDC_CLIENT_ID` | No | `authn-api` | OAuth2 client ID registered with the OIDC provider. |
| `OIDC_REDIRECT_URL` | No | `http://localhost:10100/callback` | Callback URL registered with the OIDC provider. Must match exactly. |
| `FRONTEND_URL` | No | `http://localhost:5173` | Default URL to redirect users after successful login. |
| `DATABASE_URL` | No | `postgres://authn_api:password@localhost:5432/fundament` | PostgreSQL connection string. |
| `LISTEN_ADDR` | No | `:8080` | Address and port for the HTTP server to listen on. |
| `LOG_LEVEL` | No | `info` | Logging level: `debug`, `info`, `warn`, or `error`. |
| `CORS_ALLOWED_ORIGINS` | No | `http://localhost:5173` | Comma-separated list of allowed CORS origins. |
| `COOKIE_DOMAIN` | No | `localhost` | Domain for auth cookies. Leave as `localhost` for local dev. |
| `COOKIE_SECURE` | No | `false` | Set to `true` to require HTTPS for cookies (use in production). |

## Future Improvements

- **Session Store**: Currently using Gorilla Sessions with cookie store for OAuth state. For multi-instance deployments, replace with Redis-backed store (e.g., `github.com/boj/redistore`).
