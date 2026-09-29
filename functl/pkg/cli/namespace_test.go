package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/functl/pkg/client"
	"github.com/fundament-oss/fundament/functl/pkg/config"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// The organization-api scopes namespaces to the organization in the
// Fun-Organization header and refuses a call without it. The namespace
// commands once built their client without the active organization, so every
// one of them failed.

type fakeTokenService struct {
	authnv1connect.UnimplementedTokenServiceHandler
}

func (fakeTokenService) ExchangeToken(context.Context, *authnv1.ExchangeTokenRequest) (*authnv1.ExchangeTokenResponse, error) {
	return authnv1.ExchangeTokenResponse_builder{AccessToken: "jwt", TokenType: "Bearer", ExpiresIn: 3600}.Build(), nil
}

type fakeNamespaceService struct {
	organizationv1connect.UnimplementedNamespaceServiceHandler
}

func (fakeNamespaceService) ListClusterNamespaces(context.Context, *organizationv1.ListClusterNamespacesRequest) (*organizationv1.ListClusterNamespacesResponse, error) {
	return organizationv1.ListClusterNamespacesResponse_builder{}.Build(), nil
}

func (fakeNamespaceService) ListProjectNamespaces(context.Context, *organizationv1.ListProjectNamespacesRequest) (*organizationv1.ListProjectNamespacesResponse, error) {
	return organizationv1.ListProjectNamespacesResponse_builder{}.Build(), nil
}

func (fakeNamespaceService) CreateNamespace(context.Context, *organizationv1.CreateNamespaceRequest) (*organizationv1.CreateNamespaceResponse, error) {
	return organizationv1.CreateNamespaceResponse_builder{NamespaceId: "ns-1"}.Build(), nil
}

func (fakeNamespaceService) DeleteNamespace(context.Context, *organizationv1.DeleteNamespaceRequest) (*organizationv1.DeleteNamespaceResponse, error) {
	return organizationv1.DeleteNamespaceResponse_builder{}.Build(), nil
}

// newFakeAPI serves the token exchange and the namespace service, and records
// the organization header each namespace call arrived with.
func newFakeAPI(t *testing.T) (orgHeaders func() []string) {
	t.Helper()

	var mu sync.Mutex
	var seen []string

	mux := http.NewServeMux()
	mux.Handle(authnv1connect.NewTokenServiceHandler(fakeTokenService{}))
	path, handler := organizationv1connect.NewNamespaceServiceHandler(fakeNamespaceService{})
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get(client.OrganizationHeader))
		mu.Unlock()
		handler.ServeHTTP(w, r)
	}))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	t.Setenv("FUNCTL_CONFIG_DIR", t.TempDir())
	t.Setenv(config.EnvAPIEndpoint, server.URL)
	t.Setenv(config.EnvAuthnURL, server.URL)
	t.Setenv("FUNDAMENT_API_KEY", "fun_test")

	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
}

func TestNamespaceCommandsSendTheActiveOrganization(t *testing.T) {
	const orgID = "019b4000-0000-7000-8000-000000000001"

	commands := map[string]interface{ Run(*Context) error }{
		"list --cluster": &NamespaceListCmd{Cluster: "cluster-1"},
		"list --project": &NamespaceListCmd{Project: "project-1"},
		"create":         &NamespaceCreateCmd{Name: "ns", Project: "project-1"},
		"delete":         &NamespaceDeleteCmd{NamespaceID: "ns-1"},
	}

	for name, cmd := range commands {
		t.Run(name, func(t *testing.T) {
			orgHeaders := newFakeAPI(t)

			ctx := &Context{Output: OutputJSON, Config: &config.Config{Organization: orgID}}
			require.NoError(t, cmd.Run(ctx))

			assert.Equal(t, []string{orgID}, orgHeaders())
		})
	}
}

func TestNamespaceCommandsRequireAnOrganization(t *testing.T) {
	newFakeAPI(t)

	err := (&NamespaceListCmd{Cluster: "cluster-1"}).Run(&Context{Config: &config.Config{}})
	require.ErrorIs(t, err, ErrNoActiveOrganization)
}
