package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/authn-api/pkg/shootverify"
	"github.com/fundament-oss/fundament/common/auth"
)

type fakeClusters struct {
	rows  map[uuid.UUID]uuid.UUID // cluster -> organization
	err   error
	calls int
	// block makes the lookup wait for the request context, like a slow query.
	block bool
}

func (f *fakeClusters) ClusterGetByID(ctx context.Context, arg db.ClusterGetByIDParams) (db.ClusterGetByIDRow, error) {
	f.calls++
	if f.block {
		<-ctx.Done()
		return db.ClusterGetByIDRow{}, fmt.Errorf("query: %w", ctx.Err())
	}
	if f.err != nil {
		return db.ClusterGetByIDRow{}, f.err
	}
	org, ok := f.rows[arg.ID]
	if !ok {
		return db.ClusterGetByIDRow{}, pgx.ErrNoRows
	}
	return db.ClusterGetByIDRow{ID: arg.ID, OrganizationID: org}, nil
}

// fakeShootVerifier records the last call and answers with a fixed subject
// or error.
type fakeShootVerifier struct {
	subject *shootverify.Subject
	err     error
	// delay stands in for a slow review.
	delay   time.Duration
	calls   int
	lastID  string
	lastAud string
}

func (f *fakeShootVerifier) Verify(_ context.Context, clusterID, _, audience string) (*shootverify.Subject, error) {
	f.calls++
	f.lastID = clusterID
	f.lastAud = audience
	time.Sleep(f.delay)
	if f.err != nil {
		return nil, f.err
	}
	return f.subject, nil
}

type exchangeHarness struct {
	server    *AuthnServer
	client    authnv1connect.TokenServiceClient
	secret    []byte
	clusters  *fakeClusters
	verifier  *fakeShootVerifier
	clusterID uuid.UUID
	orgID     uuid.UUID
	url       string
}

func newExchangeHarness(t *testing.T) *exchangeHarness {
	t.Helper()
	secret := []byte(testJWTSecret)
	clusterID := uuid.New()
	orgID := uuid.New()
	clusters := &fakeClusters{rows: map[uuid.UUID]uuid.UUID{clusterID: orgID}}
	verifier := &fakeShootVerifier{subject: &shootverify.Subject{
		Username:       ShootPluginControllerSubject,
		UID:            "sa-uid",
		CredentialID:   "JTI=abc",
		OrganizationID: orgID.String(),
	}}
	server := &AuthnServer{
		config:           &Config{JWTSecret: secret, TokenExpiry: 15 * time.Minute},
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		clusters:         clusters,
		shootVerifier:    verifier,
		workloadSubjects: DefaultWorkloadSubjects(),
	}

	mux := http.NewServeMux()
	path, handler := authnv1connect.NewTokenServiceHandler(server, connect.WithInterceptors(validate.NewInterceptor()))
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &exchangeHarness{
		server:    server,
		client:    authnv1connect.NewTokenServiceClient(srv.Client(), srv.URL),
		secret:    secret,
		clusters:  clusters,
		verifier:  verifier,
		clusterID: clusterID,
		orgID:     orgID,
		url:       srv.URL,
	}
}

// credential mints something shaped like a projected ServiceAccount token.
// The signature is irrelevant to authn-api (only the cluster verifies it),
// so any key will do.
func credential(t *testing.T, aud []string, exp time.Duration) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Issuer:    "https://kubernetes.default.svc.cluster.local",
		Subject:   ShootPluginControllerSubject,
		Audience:  aud,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(exp)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("shoot-signing-key"))
	require.NoError(t, err)
	return s
}

func (h *exchangeHarness) exchange(t *testing.T, authHeader, clusterID string) (*authnv1.ExchangeWorkloadTokenResponse, error) {
	t.Helper()
	ctx, callInfo := connect.NewClientContext(context.Background())
	if authHeader != "" {
		callInfo.RequestHeader().Set("Authorization", authHeader)
	}
	return h.client.ExchangeWorkloadToken(ctx, authnv1.ExchangeWorkloadTokenRequest_builder{ //nolint:wrapcheck // tests assert on the raw connect error
		ClusterId: clusterID,
	}.Build())
}

func TestExchangeWorkloadToken_Success(t *testing.T) {
	h := newExchangeHarness(t)

	resp, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour), h.clusterID.String())
	require.NoError(t, err)

	assert.Equal(t, "Bearer", resp.GetTokenType())
	assert.InDelta(t, int64(WorkloadTokenExpiry.Seconds()), resp.GetExpiresIn(), 2)
	assert.Equal(t, h.clusterID.String(), h.verifier.lastID, "verified against the requested cluster")
	assert.Equal(t, auth.WorkloadCredentialAudience, h.verifier.lastAud)

	claims, err := auth.ParseWorkloadToken(resp.GetAccessToken(), h.secret)
	require.NoError(t, err)
	assert.Equal(t, h.clusterID, claims.ClusterID())
	assert.Equal(t, h.orgID.String(), claims.OrganizationID)
	assert.Equal(t, auth.WorkloadPluginController, claims.Workload)
	assert.NotEmpty(t, claims.ID, "jti")
	assert.NotNil(t, claims.IssuedAt)
}

// TestExchangeWorkloadToken_ExpCappedByCredential pins the replay bound: the
// WorkloadToken never outlives the projected token that produced it.
func TestExchangeWorkloadToken_ExpCappedByCredential(t *testing.T) {
	h := newExchangeHarness(t)

	resp, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, 4*time.Minute), h.clusterID.String())
	require.NoError(t, err)
	assert.InDelta(t, int64((4 * time.Minute).Seconds()), resp.GetExpiresIn(), 2)

	claims, err := auth.ParseWorkloadToken(resp.GetAccessToken(), h.secret)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(4*time.Minute), claims.ExpiresAt.Time, 2*time.Second)
}

func TestExchangeWorkloadToken_Denied(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(h *exchangeHarness)
		authHeader    func(t *testing.T, h *exchangeHarness) string
		clusterID     func(h *exchangeHarness) string
		verifierCalls int
		clusterCalls  int
	}{
		{
			name:       "no authorization header",
			authHeader: func(*testing.T, *exchangeHarness) string { return "" },
		},
		{
			name: "not a JWT",
			authHeader: func(*testing.T, *exchangeHarness) string {
				return "Bearer fun_notajwt"
			},
		},
		{
			name: "credential for another audience",
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{"https://kubernetes.default.svc.cluster.local"}, time.Hour)
			},
		},
		{
			name: "credential expired",
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, -time.Minute)
			},
		},
		{
			name: "credential exp beyond projected lifetime",
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, 24*time.Hour)
			},
		},
		{
			name: "fundament UserToken presented as credential",
			authHeader: func(t *testing.T, h *exchangeHarness) string {
				claims := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
					Issuer:    auth.ConsoleIssuer,
					Subject:   uuid.NewString(),
					Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				}}
				s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(h.secret)
				require.NoError(t, err)
				return "Bearer " + s
			},
		},
		{
			name: "unknown cluster",
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour)
			},
			clusterID:    func(*exchangeHarness) string { return uuid.NewString() },
			clusterCalls: 1,
		},
		{
			name:  "cluster rejects the token",
			setup: func(h *exchangeHarness) { h.verifier.err = shootverify.ErrUnauthenticated },
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour)
			},
			clusterCalls: 1, verifierCalls: 1,
		},
		{
			name:  "verifier unavailable",
			setup: func(h *exchangeHarness) { h.verifier.err = shootverify.ErrUnavailable },
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour)
			},
			clusterCalls: 1, verifierCalls: 1,
		},
		{
			name:  "subject not allow-listed",
			setup: func(h *exchangeHarness) { h.verifier.subject.Username = "system:serviceaccount:default:default" },
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour)
			},
			clusterCalls: 1, verifierCalls: 1,
		},
		{
			name:  "organization mismatch",
			setup: func(h *exchangeHarness) { h.verifier.subject.OrganizationID = uuid.NewString() },
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour)
			},
			clusterCalls: 1, verifierCalls: 1,
		},
		{
			name:  "verifier reports no organization",
			setup: func(h *exchangeHarness) { h.verifier.subject.OrganizationID = "" },
			authHeader: func(t *testing.T, _ *exchangeHarness) string {
				return "Bearer " + credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour)
			},
			clusterCalls: 1, verifierCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newExchangeHarness(t)
			if tt.setup != nil {
				tt.setup(h)
			}
			clusterID := h.clusterID.String()
			if tt.clusterID != nil {
				clusterID = tt.clusterID(h)
			}
			_, err := h.exchange(t, tt.authHeader(t, h), clusterID)
			assertConnectCode(t, err, connect.CodeUnauthenticated)
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, errWorkloadCredential.Error(), ce.Message(), "one generic message for every pre-signing failure")
			assert.Equal(t, tt.clusterCalls, h.clusters.calls)
			assert.Equal(t, tt.verifierCalls, h.verifier.calls)
		})
	}
}

func TestExchangeWorkloadToken_ClusterLookupError_Internal(t *testing.T) {
	h := newExchangeHarness(t)
	h.clusters.err = errors.New("connection refused")

	_, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour), h.clusterID.String())
	assertConnectCode(t, err, connect.CodeInternal)
	assert.Equal(t, 0, h.verifier.calls)
}

func TestExchangeWorkloadToken_MalformedClusterID_InvalidArgument(t *testing.T) {
	h := newExchangeHarness(t)

	_, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour), "not-a-uuid")
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Equal(t, 0, h.clusters.calls)
}

// TestExchangeWorkloadToken_Walls pins FUN-17's three-way wall for the new
// type: a WorkloadToken is neither a UserToken nor exchangeable again.
func TestExchangeWorkloadToken_Walls(t *testing.T) {
	h := newExchangeHarness(t)

	resp, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour), h.clusterID.String())
	require.NoError(t, err)

	userValidator := auth.NewValidatorForAudience(h.secret, auth.ConsoleAuthCookieName, auth.ConsoleIssuer, auth.TokenTypeUser, nil)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+resp.GetAccessToken())
	_, err = userValidator.Validate(header)
	require.Error(t, err, "user validator accepted a WorkloadToken")

	_, err = auth.ParsePluginToken(resp.GetAccessToken(), h.secret)
	require.Error(t, err, "ParsePluginToken accepted a WorkloadToken")

	h.verifier.calls = 0
	_, err = h.exchange(t, "Bearer "+resp.GetAccessToken(), h.clusterID.String())
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Equal(t, 0, h.verifier.calls, "a WorkloadToken fails the local pre-check, never reaching a cluster")
}

func TestExchangeWorkloadToken_LocalSubjectFromConfig(t *testing.T) {
	h := newExchangeHarness(t)
	local := "system:serviceaccount:fundament:fundament-plugin-controller"
	h.server.workloadSubjects = workloadSubjects(map[string]string{local: auth.WorkloadPluginController})
	h.verifier.subject.Username = local

	resp, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour), h.clusterID.String())
	require.NoError(t, err)
	claims, err := auth.ParseWorkloadToken(resp.GetAccessToken(), h.secret)
	require.NoError(t, err)
	assert.Equal(t, auth.WorkloadPluginController, claims.Workload)
	assert.Contains(t, h.server.workloadSubjects, ShootPluginControllerSubject, "the shoot subject is always allow-listed")
}

// A caller whose deadline expires during the cluster lookup gets the
// deadline's own code, not internal: nothing is wrong on the server.
func TestExchangeWorkloadToken_CallerDeadlineDuringLookup(t *testing.T) {
	h := newExchangeHarness(t)
	h.clusters.block = true

	body := `{"clusterId":"` + h.clusterID.String() + `"}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		h.url+authnv1connect.TokenServiceExchangeWorkloadTokenProcedure, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Timeout-Ms", "50")
	req.Header.Set("Authorization", "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, time.Hour))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var got struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, connect.CodeDeadlineExceeded.String(), got.Code)
	assert.Equal(t, 0, h.verifier.calls)
}

// A credential that expires while its review is in flight must not yield a
// token that is already dead (zero or negative expires_in).
func TestExchangeWorkloadToken_CredentialExpiringDuringReview(t *testing.T) {
	h := newExchangeHarness(t)
	h.verifier.delay = 2 * time.Second

	_, err := h.exchange(t, "Bearer "+credential(t, []string{auth.WorkloadCredentialAudience}, 1500*time.Millisecond), h.clusterID.String())
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Equal(t, 1, h.verifier.calls, "refused after the review, not at the pre-check")
}
