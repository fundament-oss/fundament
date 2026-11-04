package denial

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authzv1 "github.com/fundament-oss/fundament/authz-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/common/authz/actions"
)

type sentenceCase struct {
	Action   string `json:"action"`
	Role     string `json:"role"`
	Sentence string `json:"sentence"`
}

func TestSentencesFileCoversEveryAction(t *testing.T) {
	raw, err := os.ReadFile("testdata/sentences.json")
	require.NoError(t, err)
	var cases []sentenceCase
	require.NoError(t, json.Unmarshal(raw, &cases))

	all := actions.All()
	require.Len(t, cases, len(all))

	seen := map[string]bool{}
	for _, c := range cases {
		a, ok := authzv1.Action_value[c.Action]
		require.True(t, ok, "unknown action %q", c.Action)
		r, ok := authzv1.RequiredRole_value[c.Role]
		require.True(t, ok, "unknown role %q", c.Role)
		assert.False(t, seen[c.Action], "duplicate %s", c.Action)
		seen[c.Action] = true
		assert.Equal(t, actions.Role(authzv1.Action(a)), authzv1.RequiredRole(r), c.Action)

		d := authzv1.PermissionDeniedDetail_builder{Action: authzv1.Action(a), RequiredRole: authzv1.RequiredRole(r)}.Build()
		assert.Equal(t, c.Sentence, Sentence(d), c.Action)
	}
	for _, a := range all {
		assert.True(t, seen[a.String()], "missing %s", a)
	}
}

func TestSentenceFallbacks(t *testing.T) {
	assert.Equal(t, "You do not have permission to do this.", Sentence(nil))

	d := authzv1.PermissionDeniedDetail_builder{
		Action:       authzv1.Action_ACTION_UNSPECIFIED,
		RequiredRole: authzv1.RequiredRole_REQUIRED_ROLE_PROJECT_ADMIN,
	}.Build()
	assert.Equal(t, "Only project admins can do this", Sentence(d))
}

func TestSentenceUnknownRole(t *testing.T) {
	d := authzv1.PermissionDeniedDetail_builder{
		Action:       authzv1.Action_ACTION_UNSPECIFIED,
		RequiredRole: authzv1.RequiredRole(999),
	}.Build()
	assert.Equal(t, "Only users with the required role can do this", Sentence(d))
}

func TestInterceptorRewritesOnlyDenials(t *testing.T) {
	detail := authzv1.PermissionDeniedDetail_builder{
		Action:       authzv1.Action_ACTION_DELETE_CLUSTER,
		RequiredRole: authzv1.RequiredRole_REQUIRED_ROLE_ORGANIZATION_ADMIN,
	}.Build()
	denied := connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	ed, err := connect.NewErrorDetail(detail)
	require.NoError(t, err)
	denied.AddDetail(ed)

	call := func(in error) error {
		next := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, in }
		_, err := Interceptor()(next)(context.Background(), nil)
		return err
	}

	err = call(denied)
	require.Error(t, err)
	assert.Equal(t, "Only organization admins can delete clusters", err.Error())
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	bare := call(connect.NewError(connect.CodePermissionDenied, errors.New("permission denied")))
	assert.Equal(t, "You do not have permission to do this.", bare.Error())
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(bare))

	notFound := connect.NewError(connect.CodeNotFound, errors.New("missing"))
	assert.Same(t, notFound, call(notFound))
}
