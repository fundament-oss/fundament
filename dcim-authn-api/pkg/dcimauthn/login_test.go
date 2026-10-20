package dcimauthn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	db "github.com/fundament-oss/fundament/dcim-authn-api/pkg/db/gen"
)

const (
	idpIssuer   = "https://dex.test"
	idpClientID = "dcim-authn"
)

// idpSecret signs the fake dex's ID tokens; distinct from the server's
// JWTSecret so the two token domains cannot be confused.
var idpSecret = []byte("idp-secret-idp-secret-idp-secret")

// hs256KeySet lets the oidc verifier check the fake dex's HS256 ID tokens;
// production dex signs RS256, but the algorithm is not what's under test.
type hs256KeySet struct{}

func (hs256KeySet) VerifySignature(_ context.Context, rawJWT string) ([]byte, error) {
	if _, err := jwt.Parse(rawJWT, func(*jwt.Token) (any, error) { return idpSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})); err != nil {
		return nil, err
	}
	return base64.RawURLEncoding.DecodeString(strings.Split(rawJWT, ".")[1])
}

// loginStore records the upsert and answers with a fixed directory row whose
// id is unrelated to any IdP subject: the only way the minted token's sub can
// equal that id is by taking it from the upsert result.
type loginStore struct {
	stubStore
	row      db.UserUpsertRow
	upserted db.UserUpsertParams
}

func (s *loginStore) UserUpsert(_ context.Context, arg db.UserUpsertParams) (db.UserUpsertRow, error) {
	s.upserted = arg
	return s.row, nil
}

func signIDToken(t *testing.T, sub string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":   idpIssuer,
		"aud":   idpClientID,
		"sub":   sub,
		"name":  "Alice",
		"email": "alice@example.com",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(idpSecret)
	require.NoError(t, err)
	return signed
}

// loginTestServer wires the refresh test server to a fake dex token endpoint
// that answers the password grant with the given ID token.
func loginTestServer(t *testing.T, store Store, idToken string) *Server {
	t.Helper()

	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
			"access_token": "dex-access-token",
			"token_type":   "bearer",
			"id_token":     idToken,
		}))
	}))
	t.Cleanup(dex.Close)

	s := refreshTestServer()
	s.queries = store
	s.oauth2Config = &oauth2.Config{
		ClientID: idpClientID,
		Endpoint: oauth2.Endpoint{TokenURL: dex.URL + "/token"},
	}
	s.oidcVerifier = oidc.NewVerifier(idpIssuer, hs256KeySet{}, &oidc.Config{
		ClientID:             idpClientID,
		SupportedSigningAlgs: []string{jwt.SigningMethodHS256.Alg()},
	})
	return s
}

func postLogin(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login",
		strings.NewReader(`{"email":"alice@example.com","password":"pw"}`))
	rec := httptest.NewRecorder()
	s.HandlePasswordLogin(rec, req)
	return rec
}

// The core contract of the login flow: the minted token's subject is the
// dcim.users.id the upsert returned — not the IdP sub, and not anything
// derived from it — and the upsert received the IdP claims verbatim.
func TestHandlePasswordLogin_MintsInternalSubject(t *testing.T) {
	store := &loginStore{row: db.UserUpsertRow{ID: uuid.New(), Name: "Alice Renamed"}}
	s := loginTestServer(t, store, signIDToken(t, "CiQxMjM0IdP-opaque-sub"))

	rec := postLogin(t, s)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, db.UserUpsertParams{
		Name:        "Alice",
		ExternalRef: "CiQxMjM0IdP-opaque-sub",
		Email:       "alice@example.com",
	}, store.upserted)

	cookies := rec.Result().Cookies()
	require.NotEmpty(t, cookies, "expected an auth cookie to be set")
	claims := decodeToken(t, s.config.JWTSecret, cookies[0].Value)
	assert.Equal(t, store.row.ID.String(), claims.Subject)
	assert.Equal(t, store.row.Name, claims.Name)
}

// An ID token without a subject must be refused before it reaches the
// directory: upserting it would merge every such login onto one
// empty-external_ref row. The stubStore panics on UserUpsert, so this also
// proves the refusal happens before any provisioning.
func TestHandlePasswordLogin_EmptySubjectIsRefused(t *testing.T) {
	s := loginTestServer(t, stubStore{}, signIDToken(t, ""))

	rec := postLogin(t, s)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}
