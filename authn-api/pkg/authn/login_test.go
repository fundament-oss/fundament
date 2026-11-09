package authn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/fundament-oss/fundament/authn-api/pkg/authnhttp"
	"github.com/fundament-oss/fundament/common/psqldb"
)

const testProviderAuthURL = "https://login.test-fundamentsso.nl/realms/fundamentsso/protocol/openid-connect/auth"

// loginServer builds a server that can run the login handlers without a
// database: an allowlist, a session store and the provider's auth endpoint.
func loginServer() *AuthnServer {
	server := allowlistServer()
	server.sessionStore = NewSessionStore([]byte("test-secret"))
	server.oauth2Config = &oauth2.Config{
		ClientID:    "fundament-poc",
		RedirectURL: "https://authn.fundament.localhost:8443/callback",
		Endpoint:    oauth2.Endpoint{AuthURL: testProviderAuthURL},
	}
	return server
}

func TestHandleLogin_RedirectsToProviderWithPKCE(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", http.NoBody)
	loginServer().HandleLogin(recorder, request, authnhttp.HandleLoginParams{})

	require.Equal(t, http.StatusTemporaryRedirect, recorder.Code)
	location, err := url.Parse(recorder.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, testProviderAuthURL, location.Scheme+"://"+location.Host+location.Path)
	assert.Equal(t, "S256", location.Query().Get("code_challenge_method"))
	assert.NotEmpty(t, location.Query().Get("code_challenge"))

	// The redirect back to /callback comes from the identity provider's site,
	// so a Strict cookie would be withheld from it.
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionName, cookies[0].Name)
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
}

func TestHandleLoginMethods(t *testing.T) {
	tests := map[string]struct {
		password     bool
		displayName  string
		wantPassword bool
		wantProvider *string
	}{
		"Dex only":       {password: true, wantPassword: true},
		"external only":  {password: false, displayName: "Pleio", wantProvider: new("Pleio")},
		"Dex and button": {password: true, displayName: "Dex", wantPassword: true, wantProvider: new("Dex")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server := loginServer()
			server.config.PasswordLoginEnabled = tt.password
			server.config.OIDCDisplayName = tt.displayName

			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login/methods", http.NoBody)
			server.HandleLoginMethods(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var body authnhttp.LoginMethodsResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			assert.Equal(t, tt.wantPassword, body.Password)
			assert.Equal(t, tt.wantProvider, body.ProviderName)
		})
	}
}

// With an external identity provider there is no password grant to send the
// form's credentials to, so the endpoint refuses before trying one.
func TestHandlePasswordLogin_DisabledIs404(t *testing.T) {
	server := loginServer()
	server.config.PasswordLoginEnabled = false

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login/password",
		strings.NewReader(`{"email":"alice@acme-corp.com","password":"password"}`))
	server.HandlePasswordLogin(recorder, request)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

// seedInvite registers an email the way `funops user create` does: a user
// row nobody has signed in with yet (no external_ref).
func seedInvite(t *testing.T, env *refreshEnv, email string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := env.superPool.QueryRow(context.Background(),
		`INSERT INTO tenant.users (name, email) VALUES ($1, $1) RETURNING id`, email).Scan(&id)
	require.NoError(t, err)
	return id
}

func userRow(t *testing.T, env *refreshEnv, id uuid.UUID) (externalRef, email pgtype.Text) {
	t.Helper()

	err := env.superPool.QueryRow(context.Background(),
		`SELECT external_ref, email FROM tenant.users WHERE id = $1`, id).Scan(&externalRef, &email)
	require.NoError(t, err)
	return externalRef, email
}

func newLoginEnv(t *testing.T) *refreshEnv {
	t.Helper()

	env := newRefreshEnv(t)
	env.server.db = &psqldb.DB{Pool: env.servicePool}
	return env
}

// An address the identity provider has not verified must not claim the
// invite registered at it: whoever controls an unverified address at the
// provider is not necessarily the person who was invited. Nor may the sign-in
// go ahead without the address: that account could never take it once it is
// verified, because the invite holds it. Once verified, the invite is claimed.
func TestProcessOIDCLogin_UnverifiedEmailDoesNotClaimInvite(t *testing.T) {
	env := newLoginEnv(t)
	inviteID := seedInvite(t, env, "participant@example.com")
	claims := oidcClaims{Sub: "keycloak-sub-1", Name: "Participant", Email: "participant@example.com"}

	unverified := claims
	_, _, err := env.server.processOIDCLogin(context.Background(), &unverified, "oidc")
	require.ErrorIs(t, err, errEmailNotVerified)

	inviteRef, _ := userRow(t, env, inviteID)
	assert.False(t, inviteRef.Valid, "the invite is no longer pending")
	var accounts int
	require.NoError(t, env.superPool.QueryRow(context.Background(),
		`SELECT count(*) FROM tenant.users WHERE external_ref = $1`, claims.Sub).Scan(&accounts))
	assert.Zero(t, accounts, "the refused sign-in created an account")

	verified := claims
	verified.EmailVerified = true
	u, _, err := env.server.processOIDCLogin(context.Background(), &verified, "oidc")
	require.NoError(t, err)
	assert.Equal(t, inviteID, u.ID)
}

// Without an invite at it, an unverified address is refused the same way:
// otherwise the refusal would tell whoever signs in which addresses were
// invited.
func TestProcessOIDCLogin_UnverifiedEmailRefusedWithoutInvite(t *testing.T) {
	env := newLoginEnv(t)

	_, _, err := env.server.processOIDCLogin(context.Background(), &oidcClaims{
		Sub: "keycloak-sub-3", Name: "Someone", Email: "someone@example.com", EmailVerified: false,
	}, "oidc")
	require.ErrorIs(t, err, errEmailNotVerified)
}

func TestLoginErrorStatus(t *testing.T) {
	assert.Equal(t, http.StatusForbidden, loginErrorStatus(errEmailNotVerified))
	assert.Equal(t, http.StatusForbidden, loginErrorStatus(errEmailInUse))
	assert.Equal(t, http.StatusInternalServerError, loginErrorStatus(errors.New("database down")))
}

// An existing account keeps the address it has when the provider later
// reports one it has not verified.
func TestProcessOIDCLogin_UnverifiedEmailKeepsExistingAddress(t *testing.T) {
	env := newLoginEnv(t)

	u, _, err := env.server.processOIDCLogin(context.Background(), &oidcClaims{
		Sub: "keycloak-sub-4", Name: "Someone", Email: "someone@example.com", EmailVerified: true,
	}, "oidc")
	require.NoError(t, err)

	_, _, err = env.server.processOIDCLogin(context.Background(), &oidcClaims{
		Sub: "keycloak-sub-4", Name: "Someone", Email: "changed@example.com", EmailVerified: false,
	}, "oidc")
	require.NoError(t, err)

	_, email := userRow(t, env, u.ID)
	assert.Equal(t, "someone@example.com", email.String)
}

func TestProcessOIDCLogin_VerifiedEmailClaimsInvite(t *testing.T) {
	env := newLoginEnv(t)
	inviteID := seedInvite(t, env, "participant@example.com")

	u, _, err := env.server.processOIDCLogin(context.Background(), &oidcClaims{
		Sub: "keycloak-sub-2", Name: "Participant", Email: "participant@example.com", EmailVerified: true,
	}, "oidc")
	require.NoError(t, err)

	assert.Equal(t, inviteID, u.ID)
	inviteRef, _ := userRow(t, env, inviteID)
	assert.Equal(t, "keycloak-sub-2", inviteRef.String)
}
