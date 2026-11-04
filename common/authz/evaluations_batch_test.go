package authz_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/authz/authztest"
)

func newBatchClient(t *testing.T, tuples ...authztest.Tuple) (*authz.Client, *authztest.Server) {
	t.Helper()

	srv := authztest.Serve(t, tuples...)
	store, err := authz.NewStore(authz.Config{APIURL: srv.URL, StoreName: authztest.StoreName})
	require.NoError(t, err)

	return authz.NewClient(store), srv
}

func TestEvaluationsUsesBatchCheckOrder(t *testing.T) {
	user := uuid.New()
	allowedOrg, deniedOrg := uuid.New(), uuid.New()

	client, srv := newBatchClient(t, authztest.Tuple{
		User: authz.User(user).String(), Relation: "admin", Object: authz.Organization(allowedOrg).String(),
	})

	// Alternating allow/deny, enough to need several batch-check requests.
	const n = 120
	evals := make([]authz.EvaluationRequest, 0, n)
	for i := range n {
		org := deniedOrg
		if i%2 == 0 {
			org = allowedOrg
		}
		evals = append(evals, authz.EvaluationRequest{
			Subject: authz.User(user), Resource: authz.Organization(org), Action: authz.CanEdit(),
		})
	}

	resp, err := client.Evaluations(t.Context(), authz.EvaluationsRequest{Evaluations: evals})
	require.NoError(t, err)
	require.Len(t, resp.Evaluations, n)

	for i, d := range resp.Evaluations {
		assert.Equal(t, i%2 == 0, d.Decision, fmt.Sprintf("evaluation %d", i))
	}

	assert.Greater(t, srv.BatchCheckCalls(), 1, "a batch larger than one request is split")
}

func TestEvaluationsSemantics(t *testing.T) {
	user := uuid.New()
	allowedOrg, deniedOrg := uuid.New(), uuid.New()

	client, _ := newBatchClient(t, authztest.Tuple{
		User: authz.User(user).String(), Relation: "admin", Object: authz.Organization(allowedOrg).String(),
	})

	eval := func(org uuid.UUID) authz.EvaluationRequest {
		return authz.EvaluationRequest{Subject: authz.User(user), Resource: authz.Organization(org), Action: authz.CanEdit()}
	}
	evals := []authz.EvaluationRequest{eval(allowedOrg), eval(deniedOrg), eval(allowedOrg)}

	t.Run("deny on first deny stops after the denial", func(t *testing.T) {
		resp, err := client.Evaluations(t.Context(), authz.EvaluationsRequest{
			Evaluations: evals, Options: &authz.EvaluationsOptions{Semantic: authz.DenyOnFirstDeny},
		})
		require.NoError(t, err)
		require.Len(t, resp.Evaluations, 2)
		assert.True(t, resp.Evaluations[0].Decision)
		assert.False(t, resp.Evaluations[1].Decision)
	})

	t.Run("permit on first permit stops after the permit", func(t *testing.T) {
		resp, err := client.Evaluations(t.Context(), authz.EvaluationsRequest{
			Evaluations: []authz.EvaluationRequest{eval(deniedOrg), eval(allowedOrg), eval(deniedOrg)},
			Options:     &authz.EvaluationsOptions{Semantic: authz.PermitOnFirstPermit},
		})
		require.NoError(t, err)
		require.Len(t, resp.Evaluations, 2)
		assert.True(t, resp.Evaluations[1].Decision)
	})

	t.Run("empty request", func(t *testing.T) {
		resp, err := client.Evaluations(t.Context(), authz.EvaluationsRequest{})
		require.NoError(t, err)
		assert.Empty(t, resp.Evaluations)
	})
}
