package shootverify

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/fundament-oss/fundament/common/gardener"
)

const (
	testAudience = "fundament-authn-api"
	testUsername = "system:serviceaccount:fundament-system:plugin-controller"
	testCluster  = "019b4000-2000-7000-8000-000000000001"
	testOrg      = "019b4000-0000-7000-8000-000000000001"
)

// fakeReviewer returns a fake clientset whose TokenReview create answers with
// the given status (or error). It records the spec it received.
func fakeReviewer(status *authenticationv1.TokenReviewStatus, createErr error) (*fake.Clientset, *authenticationv1.TokenReviewSpec) {
	cs := fake.NewClientset()
	var seen authenticationv1.TokenReviewSpec
	cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authenticationv1.TokenReview)
		seen = review.Spec
		if createErr != nil {
			return true, nil, createErr
		}
		return true, &authenticationv1.TokenReview{Spec: review.Spec, Status: *status}, nil
	})
	return cs, &seen
}

func authenticatedStatus() *authenticationv1.TokenReviewStatus {
	return &authenticationv1.TokenReviewStatus{
		Authenticated: true,
		Audiences:     []string{testAudience},
		User: authenticationv1.UserInfo{
			Username: testUsername,
			UID:      "sa-uid",
			Extra:    map[string]authenticationv1.ExtraValue{CredentialIDExtraKey: {"JTI=abc"}},
		},
	}
}

func TestReviewWithClientset_Authenticated(t *testing.T) {
	cs, seen := fakeReviewer(authenticatedStatus(), nil)

	subject, err := ReviewWithClientset(context.Background(), cs, "tok", testAudience)
	require.NoError(t, err)
	assert.Equal(t, testUsername, subject.Username)
	assert.Equal(t, "sa-uid", subject.UID)
	assert.Equal(t, "JTI=abc", subject.CredentialID)
	assert.Equal(t, "tok", seen.Token)
	assert.Equal(t, []string{testAudience}, seen.Audiences, "the review must carry the audience")
}

func TestReviewWithClientset_Denies(t *testing.T) {
	tests := []struct {
		name   string
		status *authenticationv1.TokenReviewStatus
		err    error
		want   error
	}{
		{"not authenticated", &authenticationv1.TokenReviewStatus{Authenticated: false}, nil, ErrUnauthenticated},
		{"status error", &authenticationv1.TokenReviewStatus{Error: "[invalid bearer token]"}, nil, ErrUnauthenticated},
		{"audience not echoed", func() *authenticationv1.TokenReviewStatus {
			s := authenticatedStatus()
			s.Audiences = []string{"https://kubernetes.default.svc"}
			return s
		}(), nil, ErrUnauthenticated},
		{"no audiences", func() *authenticationv1.TokenReviewStatus {
			s := authenticatedStatus()
			s.Audiences = nil
			return s
		}(), nil, ErrUnauthenticated},
		{"api error", &authenticationv1.TokenReviewStatus{}, errors.New("connection refused"), ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, _ := fakeReviewer(tt.status, tt.err)
			_, err := ReviewWithClientset(context.Background(), cs, "tok", testAudience)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestSingleClusterVerifier(t *testing.T) {
	cs, _ := fakeReviewer(authenticatedStatus(), nil)
	v := NewSingleClusterVerifier(cs, testCluster, testOrg)

	subject, err := v.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err)
	assert.Equal(t, testOrg, subject.OrganizationID)
	assert.Equal(t, testUsername, subject.Username)

	_, err = v.Verify(context.Background(), "019b4000-2000-7000-8000-000000000002", "tok", testAudience)
	require.ErrorIs(t, err, ErrUnknownCluster)
}

type fakeAccess struct {
	access *gardener.ShootAccess
	err    error
	calls  []string
}

func (f *fakeAccess) AccessFor(_ context.Context, clusterID string) (*gardener.ShootAccess, error) {
	f.calls = append(f.calls, clusterID)
	return f.access, f.err
}

func TestGardenerVerifier_OrganizationFromShootLabel(t *testing.T) {
	cs, _ := fakeReviewer(authenticatedStatus(), nil)
	src := &fakeAccess{access: &gardener.ShootAccess{RESTConfig: &rest.Config{Host: "https://shoot"}, OrganizationID: testOrg}}
	v := newGardenerVerifier(src, func(*rest.Config) (kubernetes.Interface, error) { return cs, nil })

	subject, err := v.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err)
	assert.Equal(t, testOrg, subject.OrganizationID)
	assert.Equal(t, []string{testCluster}, src.calls)
}

func TestGardenerVerifier_NoShootIsUnavailable(t *testing.T) {
	src := &fakeAccess{err: errors.New("no shoot found for cluster")}
	v := newGardenerVerifier(src, func(*rest.Config) (kubernetes.Interface, error) {
		t.Fatal("clientset must not be built without access")
		return nil, nil
	})

	_, err := v.Verify(context.Background(), testCluster, "tok", testAudience)
	require.ErrorIs(t, err, ErrUnavailable)
}

func mockToken(t *testing.T, secret []byte, aud []string, exp time.Duration) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   testUsername,
		Audience:  aud,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(exp)),
		ID:        "cred-1",
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	require.NoError(t, err)
	return s
}

func TestMockVerifier(t *testing.T) {
	secret := []byte("mock-shoot-secret")
	v := NewMockVerifier(secret, testCluster, testOrg)

	subject, err := v.Verify(context.Background(), testCluster,
		mockToken(t, secret, []string{testAudience}, time.Hour), testAudience)
	require.NoError(t, err)
	assert.Equal(t, testUsername, subject.Username)
	assert.Equal(t, testOrg, subject.OrganizationID)
	assert.Equal(t, "cred-1", subject.CredentialID)

	_, err = v.Verify(context.Background(), testCluster,
		mockToken(t, secret, []string{"https://kubernetes.default.svc"}, time.Hour), testAudience)
	require.ErrorIs(t, err, ErrUnauthenticated, "wrong audience")

	_, err = v.Verify(context.Background(), testCluster,
		mockToken(t, []byte("other"), []string{testAudience}, time.Hour), testAudience)
	require.ErrorIs(t, err, ErrUnauthenticated, "wrong secret")

	_, err = v.Verify(context.Background(), testCluster,
		mockToken(t, secret, []string{testAudience}, -time.Minute), testAudience)
	require.ErrorIs(t, err, ErrUnauthenticated, "expired")

	_, err = v.Verify(context.Background(), "019b4000-2000-7000-8000-000000000002",
		mockToken(t, secret, []string{testAudience}, time.Hour), testAudience)
	require.ErrorIs(t, err, ErrUnknownCluster)
}

type scriptedVerifier struct {
	results []error
	calls   int
	block   bool
}

func (s *scriptedVerifier) Verify(ctx context.Context, _, _, _ string) (*Subject, error) {
	s.calls++
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err() //nolint:wrapcheck // test double
	}
	err := s.results[min(s.calls-1, len(s.results)-1)]
	if err != nil {
		return nil, err
	}
	return &Subject{Username: testUsername}, nil
}

func TestGuarded_TimeoutIsUnavailable(t *testing.T) {
	inner := &scriptedVerifier{block: true}
	g := NewGuarded(inner)
	g.timeout = 10 * time.Millisecond

	_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.ErrorIs(t, err, ErrUnavailable)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestGuarded_OpensAfterConsecutiveUnavailable(t *testing.T) {
	inner := &scriptedVerifier{results: []error{ErrUnavailable}}
	g := NewGuarded(inner)
	now := time.Now()
	g.now = func() time.Time { return now }

	for range DefaultFailureThreshold {
		_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
		require.ErrorIs(t, err, ErrUnavailable)
	}
	assert.Equal(t, DefaultFailureThreshold, inner.calls)

	_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Equal(t, DefaultFailureThreshold, inner.calls, "open circuit must not call the shoot")
	assert.Contains(t, err.Error(), "circuit open")

	// Another cluster is unaffected.
	inner.results = []error{nil}
	_, err = g.Verify(context.Background(), "019b4000-2000-7000-8000-000000000002", "tok", testAudience)
	require.NoError(t, err)

	// After the cooldown one probe goes through; success closes the circuit.
	now = now.Add(DefaultCooldown + time.Second)
	_, err = g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err)
	_, err = g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err)
}

func TestGuarded_DenyDoesNotTrip(t *testing.T) {
	inner := &scriptedVerifier{results: []error{ErrUnauthenticated}}
	g := NewGuarded(inner)

	for range DefaultFailureThreshold + 2 {
		_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
		require.ErrorIs(t, err, ErrUnauthenticated)
	}
	assert.Equal(t, DefaultFailureThreshold+2, inner.calls, "a healthy cluster saying no never opens the circuit")
}

// deadlineVerifier reports whether the review still had time left after a
// short delay: a review that inherited a 1 ms caller deadline would not.
type deadlineVerifier struct {
	calls int
}

func (v *deadlineVerifier) Verify(ctx context.Context, _, _, _ string) (*Subject, error) {
	v.calls++
	time.Sleep(20 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &Subject{Username: testUsername}, nil
}

// A caller's deadline or cancellation never reaches the review, so it can
// neither fail a healthy cluster's answer nor open its circuit.
func TestGuarded_CallerDeadlineDoesNotReachReview(t *testing.T) {
	inner := &deadlineVerifier{}
	g := NewGuarded(inner)

	for range DefaultFailureThreshold + 2 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_, err := g.Verify(ctx, testCluster, "tok", testAudience)
		cancel()
		require.NoError(t, err, "the review ran to completion under the guard's own timeout")
	}
	assert.Equal(t, DefaultFailureThreshold+2, inner.calls)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.Verify(ctx, testCluster, "tok", testAudience)
	require.NoError(t, err, "an already-cancelled caller does not cut the review short")
}

// gatedVerifier blocks every call until release is closed.
type gatedVerifier struct {
	entered chan struct{}
	release chan struct{}
}

func (v *gatedVerifier) Verify(context.Context, string, string, string) (*Subject, error) {
	v.entered <- struct{}{}
	<-v.release
	return &Subject{Username: testUsername}, nil
}

func TestGuarded_HalfOpenAdmitsOneProbe(t *testing.T) {
	g := NewGuarded(&scriptedVerifier{results: []error{ErrUnavailable}})
	now := time.Now()
	g.now = func() time.Time { return now }
	for range DefaultFailureThreshold {
		_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
		require.ErrorIs(t, err, ErrUnavailable)
	}

	gate := &gatedVerifier{entered: make(chan struct{}), release: make(chan struct{})}
	g.inner = gate
	now = now.Add(DefaultCooldown + time.Second)

	probeDone := make(chan error)
	go func() {
		_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
		probeDone <- err
	}()
	<-gate.entered

	_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.ErrorIs(t, err, ErrUnavailable, "a second caller must not probe while the first is in flight")
	assert.Contains(t, err.Error(), "probe in flight")

	close(gate.release)
	require.NoError(t, <-probeDone)

	go func() { <-gate.entered }()
	_, err = g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err, "a successful probe closes the circuit")
}

func TestGuarded_ProbeOutlivesItsCaller(t *testing.T) {
	inner := &scriptedVerifier{results: []error{ErrUnavailable}}
	g := NewGuarded(inner)
	now := time.Now()
	g.now = func() time.Time { return now }
	for range DefaultFailureThreshold {
		_, _ = g.Verify(context.Background(), testCluster, "tok", testAudience)
	}
	now = now.Add(DefaultCooldown + time.Second)

	inner.results = []error{nil}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.Verify(ctx, testCluster, "tok", testAudience)
	require.NoError(t, err, "the probe completes although its caller left")

	_, err = g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err, "and its success closed the circuit")
}

func TestNewReviewClientset_NoClientSideLimiter(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.invalid", QPS: 7}
	cs, err := newReviewClientset(cfg)
	require.NoError(t, err)
	assert.Nil(t, cs.AuthenticationV1().RESTClient().GetRateLimiter(), "client-go must not throttle reviews")
	assert.InDelta(t, 7, cfg.QPS, 0, "the caller's config is left alone")
}

// panickingVerifier stands in for a review that unwinds instead of returning.
type panickingVerifier struct{}

func (panickingVerifier) Verify(context.Context, string, string, string) (*Subject, error) {
	panic("review blew up")
}

// A probe that panics (caught upstream by the recovery interceptor) must not
// leave the half-open slot taken forever.
func TestGuarded_PanickingProbeReleasesHalfOpen(t *testing.T) {
	g := NewGuarded(&scriptedVerifier{results: []error{ErrUnavailable}})
	now := time.Now()
	g.now = func() time.Time { return now }
	for range DefaultFailureThreshold {
		_, _ = g.Verify(context.Background(), testCluster, "tok", testAudience)
	}
	now = now.Add(DefaultCooldown + time.Second)

	g.inner = panickingVerifier{}
	require.Panics(t, func() { _, _ = g.Verify(context.Background(), testCluster, "tok", testAudience) })

	g.inner = &scriptedVerifier{results: []error{nil}}
	_, err := g.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err, "the next caller probes instead of being refused as 'probe in flight'")
}

// The clientset is built once per ShootAccess and rebuilt when the cache hands
// out new credentials.
func TestGardenerVerifier_ReusesClientsetUntilCredentialsRotate(t *testing.T) {
	cs, _ := fakeReviewer(authenticatedStatus(), nil)
	src := &fakeAccess{access: &gardener.ShootAccess{RESTConfig: &rest.Config{Host: "https://shoot"}, OrganizationID: testOrg}}
	builds := 0
	v := newGardenerVerifier(src, func(*rest.Config) (kubernetes.Interface, error) {
		builds++
		return cs, nil
	})

	for range 3 {
		_, err := v.Verify(context.Background(), testCluster, "tok", testAudience)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, builds)

	src.access = &gardener.ShootAccess{RESTConfig: &rest.Config{Host: "https://shoot"}, OrganizationID: testOrg}
	_, err := v.Verify(context.Background(), testCluster, "tok", testAudience)
	require.NoError(t, err)
	assert.Equal(t, 2, builds, "rotated credentials get a new clientset")
}
