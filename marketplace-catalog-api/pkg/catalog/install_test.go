package catalog_test

// install.v1 tests live here rather than in pkg/install because they need the
// embedded Postgres this package's TestMain owns; a second postmaster over the
// same PGDATA would stop the first (see main_test.go).

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/auth"
	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/install"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/catalog/v1"
	"github.com/fundament-oss/fundament/marketplace-catalog-api/pkg/proto/gen/install/v1/installv1connect"
)

var installTestSecret = []byte("install-test-secret")

func installDSN(dbName string) string {
	return fmt.Sprintf("postgres://fun_marketplace_install_api@localhost:%d/%s?sslmode=disable", testDBPort, dbName)
}

func newInstallServer(t *testing.T, env *testEnv) *install.Server {
	t.Helper()

	database, err := install.NewDB(context.Background(), slog.Default(), psqldb.Config{URL: installDSN(env.dbName)})
	require.NoError(t, err)
	t.Cleanup(database.Close)

	return install.New(slog.Default(), install.Config{JWTSecret: installTestSecret}, database)
}

func asOrganization(orgID uuid.UUID) context.Context {
	return install.WithOrganizationID(context.Background(), orgID)
}

// seedAllowedOrganization allow-lists orgID for a RESTRICTED plugin.
func seedAllowedOrganization(t *testing.T, env *testEnv, pluginID, orgID uuid.UUID) {
	t.Helper()
	_, err := env.adminPool.Exec(context.Background(),
		`INSERT INTO appstore.plugin_allowed_organizations (plugin_id, organization_id) VALUES ($1, $2)`, pluginID, orgID)
	require.NoError(t, err)
}

func softDeleteDefinition(t *testing.T, env *testEnv, definitionID uuid.UUID) {
	t.Helper()
	_, err := env.adminPool.Exec(context.Background(),
		`UPDATE appstore.plugin_definitions SET deleted = now() WHERE id = $1`, definitionID)
	require.NoError(t, err)
}

func listAsInstaller(t *testing.T, srv *install.Server, ctx context.Context) []uuid.UUID {
	t.Helper()
	resp, err := srv.Service.ListPlugins(ctx, &catalogv1.ListPluginsRequest{})
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(resp.GetPlugins()))
	for _, p := range resp.GetPlugins() {
		ids = append(ids, uuid.MustParse(p.GetId()))
	}
	return ids
}

func definitionAsInstaller(srv *install.Server, ctx context.Context, pluginID uuid.UUID, version string) error {
	_, err := srv.Service.GetPluginDefinition(ctx, catalogv1.GetPluginDefinitionRequest_builder{
		PluginId: new(pluginID.String()),
		Version:  version,
	}.Build())
	return err //nolint:wrapcheck // tests assert on the raw connect code
}

func TestInstallRLS_PublicListingVisibleToEveryone(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	id := seedPlugin(t, env, seedOptions{Name: "install-public", Visibility: "public", Published: true})
	stranger := seedOrganization(t, env, "install-stranger")

	assert.Contains(t, listAsInstaller(t, srv, asOrganization(stranger)), id)
	assert.Contains(t, listAsInstaller(t, srv, context.Background()), id, "no organization in the session still sees PUBLIC")
	require.NoError(t, definitionAsInstaller(srv, asOrganization(stranger), id, "1.0.0"))
}

func TestInstallRLS_RestrictedListingOnlyForAllowListedOrOwner(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	allowed := seedOrganization(t, env, "install-allowed")
	other := seedOrganization(t, env, "install-other")
	id := seedPlugin(t, env, seedOptions{Name: "install-restricted", Visibility: "restricted", Published: true})
	seedAllowedOrganization(t, env, id, allowed)

	assert.Contains(t, listAsInstaller(t, srv, asOrganization(allowed)), id, "allow-listed organization")
	assert.Contains(t, listAsInstaller(t, srv, asOrganization(seededOrganizationID)), id, "owning organization")
	assert.NotContains(t, listAsInstaller(t, srv, asOrganization(other)), id, "any other organization")
	assert.NotContains(t, listAsInstaller(t, srv, context.Background()), id, "no organization in the session")
	assert.NotContains(t, listAsCatalog(t, env), id, "the storefront never sees RESTRICTED")

	require.NoError(t, definitionAsInstaller(srv, asOrganization(allowed), id, "1.0.0"))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(definitionAsInstaller(srv, asOrganization(other), id, "1.0.0")))

	versions, err := srv.Service.ListPluginVersions(asOrganization(allowed), catalogv1.ListPluginVersionsRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)
	assert.Len(t, versions.GetVersions(), 1)
	versions, err = srv.Service.ListPluginVersions(asOrganization(other), catalogv1.ListPluginVersionsRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)
	assert.Empty(t, versions.GetVersions())

	_, err = srv.Service.GetPlugin(asOrganization(other), catalogv1.GetPluginRequest_builder{PluginId: id.String()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// D4: an organization may install its own unpublished versions (the sideload
// loop); everyone else's drafts stay hidden even on a PUBLIC listing.
func TestInstallRLS_OwnDraftInstallableForeignDraftHidden(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	other := seedOrganization(t, env, "install-draft-other")
	id := seedPlugin(t, env, seedOptions{Name: "install-draft", Visibility: "public", Published: true})
	seedVersion(t, env, id, "2.0.0-draft", false)

	require.NoError(t, definitionAsInstaller(srv, asOrganization(seededOrganizationID), id, "2.0.0-draft"), "owner reads its draft")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(definitionAsInstaller(srv, asOrganization(other), id, "2.0.0-draft")), "foreign draft")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(definitionAsInstaller(srv, context.Background(), id, "2.0.0-draft")), "no organization")
	require.NoError(t, definitionAsInstaller(srv, asOrganization(other), id, "1.0.0"), "the published version stays readable")

	// Drafts never appear in the version history, even for the owner: the
	// console pins installs to published versions only.
	versions, err := srv.Service.ListPluginVersions(asOrganization(seededOrganizationID), catalogv1.ListPluginVersionsRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)
	assert.Len(t, versions.GetVersions(), 1)
}

func TestInstallRLS_DefinitionByName(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	other := seedOrganization(t, env, "install-byname-other")
	seedPlugin(t, env, seedOptions{Name: "install-byname", Visibility: "public", Published: true})
	restricted := seedPlugin(t, env, seedOptions{Name: "install-byname-restricted", Visibility: "restricted", Published: true})
	seedAllowedOrganization(t, env, restricted, other)

	byName := func(ctx context.Context, plugin, version string) error {
		_, err := srv.Service.GetPluginDefinition(ctx, catalogv1.GetPluginDefinitionRequest_builder{
			Name:    catalogv1.PluginRef_builder{OrganizationName: seededOrganizationName, PluginName: plugin}.Build(),
			Version: version,
		}.Build())
		return err //nolint:wrapcheck // tests assert on the raw connect code
	}
	require.NoError(t, byName(asOrganization(other), "install-byname", "1.0.0"))
	require.NoError(t, byName(asOrganization(other), "install-byname-restricted", "1.0.0"), "allow-listed by name")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(byName(context.Background(), "install-byname-restricted", "1.0.0")))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(byName(asOrganization(other), "install-byname", "9.9.9")), "unknown version")
}

func TestInstallRLS_SoftDeletedPluginHiddenFromOwner(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	id := seedPlugin(t, env, seedOptions{Name: "install-deleted", Visibility: "public", Published: true, Deleted: true})

	owner := asOrganization(seededOrganizationID)
	assert.NotContains(t, listAsInstaller(t, srv, owner), id)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(definitionAsInstaller(srv, owner, id, "1.0.0")))
	_, err := srv.Service.GetPlugin(owner, catalogv1.GetPluginRequest_builder{PluginId: id.String()}.Build())
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestInstallRLS_SoftDeletedDefinitionHiddenFromOwner(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	id := seedPlugin(t, env, seedOptions{Name: "install-deleted-def", Visibility: "public", Published: true})
	gone := seedVersion(t, env, id, "1.1.0", true)
	softDeleteDefinition(t, env, gone)

	owner := asOrganization(seededOrganizationID)
	require.NoError(t, definitionAsInstaller(srv, owner, id, "1.0.0"))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(definitionAsInstaller(srv, owner, id, "1.1.0")))
	versions, err := srv.Service.ListPluginVersions(owner, catalogv1.ListPluginVersionsRequest_builder{PluginId: id.String()}.Build())
	require.NoError(t, err)
	assert.Len(t, versions.GetVersions(), 1)
}

// End to end through the Connect handler and the interceptor: a WorkloadToken
// for an allow-listed organization reads the RESTRICTED listing; a UserToken
// needs the organization header; a foreign organization gets nothing.
func TestInstallHTTP_WorkloadAndUserCredentials(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	allowed := seedOrganization(t, env, "install-http-allowed")
	other := seedOrganization(t, env, "install-http-other")
	id := seedPlugin(t, env, seedOptions{Name: "install-http", Visibility: "restricted", Published: true})
	seedAllowedOrganization(t, env, id, allowed)

	mux := http.NewServeMux()
	mux.Handle(srv.Path(), srv.Handler())
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	workloadToken := func(orgID uuid.UUID) string {
		claims := auth.WorkloadClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: auth.ConsoleIssuer, Subject: uuid.NewString(), Audience: jwt.ClaimStrings{auth.TokenTypeWorkload},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now()), ID: uuid.NewString(),
			},
			OrganizationID: orgID.String(),
			Workload:       auth.WorkloadPluginController,
		}
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(installTestSecret)
		require.NoError(t, err)
		return s
	}
	userToken := func(orgIDs ...uuid.UUID) string {
		claims := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{
			Issuer: auth.ConsoleIssuer, Subject: uuid.NewString(), Audience: jwt.ClaimStrings{auth.TokenTypeUser},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		}, OrganizationIDs: orgIDs}
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(installTestSecret)
		require.NoError(t, err)
		return s
	}
	call := func(headers map[string]string) ([]string, error) {
		client := installv1connect.NewInstallServiceClient(ts.Client(), ts.URL)
		ctx, callInfo := connect.NewClientContext(context.Background())
		for k, v := range headers {
			callInfo.RequestHeader().Set(k, v)
		}
		resp, err := client.ListPlugins(ctx, &catalogv1.ListPluginsRequest{})
		if err != nil {
			return nil, err //nolint:wrapcheck // tests assert on the raw connect error
		}
		ids := make([]string, 0, len(resp.GetPlugins()))
		for _, p := range resp.GetPlugins() {
			ids = append(ids, p.GetId())
		}
		return ids, nil
	}

	ids, err := call(map[string]string{"Authorization": "Bearer " + workloadToken(allowed)})
	require.NoError(t, err)
	assert.Contains(t, ids, id.String(), "plugin-controller of an allow-listed cluster")

	ids, err = call(map[string]string{"Authorization": "Bearer " + workloadToken(other)})
	require.NoError(t, err)
	assert.NotContains(t, ids, id.String(), "plugin-controller of another organization")

	ids, err = call(map[string]string{"Authorization": "Bearer " + userToken(allowed), install.OrganizationHeader: allowed.String()})
	require.NoError(t, err)
	assert.Contains(t, ids, id.String(), "console user acting for the allow-listed organization")

	_, err = call(map[string]string{"Authorization": "Bearer " + userToken(allowed)})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "user without organization header")

	_, err = call(map[string]string{"Authorization": "Bearer " + userToken(other), install.OrganizationHeader: allowed.String()})
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "user not a member of the named organization")

	_, err = call(nil)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))

	// Two requests on the same pool with different organizations: the
	// AfterRelease reset must keep the first caller's organization from
	// leaking into the second.
	for range 3 {
		ids, err = call(map[string]string{"Authorization": "Bearer " + workloadToken(allowed)})
		require.NoError(t, err)
		assert.Contains(t, ids, id.String())
		ids, err = call(map[string]string{"Authorization": "Bearer " + workloadToken(other)})
		require.NoError(t, err)
		assert.NotContains(t, ids, id.String())
	}
}

func publishersAsInstaller(t *testing.T, srv *install.Server, ctx context.Context) []uuid.UUID {
	t.Helper()
	resp, err := srv.Service.ListPublishers(ctx, &catalogv1.ListPublishersRequest{})
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(resp.GetPublishers()))
	for _, p := range resp.GetPublishers() {
		ids = append(ids, uuid.MustParse(p.GetId()))
	}
	return ids
}

// Installs are addressed by organization name, so every listing install.v1
// returns must resolve to a publisher: a RESTRICTED publisher the storefront
// never lists, and an organization whose only listing is a draft.
func TestInstallRLS_PublishersCoverEveryInstallableListing(t *testing.T) {
	env := newTestEnv(t)
	srv := newInstallServer(t, env)
	restrictedPublisher := seedOrganization(t, env, "install-restricted-publisher")
	draftPublisher := seedOrganization(t, env, "install-draft-publisher")
	allowed := seedOrganization(t, env, "install-allowed")
	other := seedOrganization(t, env, "install-other")

	restricted := seedPlugin(t, env, seedOptions{Name: "install-restricted", Visibility: "restricted", Published: true, OrganizationID: &restrictedPublisher})
	seedAllowedOrganization(t, env, restricted, allowed)
	seedPlugin(t, env, seedOptions{Name: "install-draft", Visibility: "public", Published: false, OrganizationID: &draftPublisher})

	assert.Contains(t, publishersAsInstaller(t, srv, asOrganization(allowed)), restrictedPublisher)
	assert.NotContains(t, publishersAsInstaller(t, srv, asOrganization(other)), restrictedPublisher)
	assert.Contains(t, publishersAsInstaller(t, srv, asOrganization(draftPublisher)), draftPublisher, "an organization's own drafts")

	for _, org := range []uuid.UUID{allowed, other, draftPublisher, restrictedPublisher} {
		ctx := asOrganization(org)
		plugins, err := srv.Service.ListPlugins(ctx, &catalogv1.ListPluginsRequest{})
		require.NoError(t, err)
		publishers := publishersAsInstaller(t, srv, ctx)
		for _, p := range plugins.GetPlugins() {
			assert.Contains(t, publishers, uuid.MustParse(p.GetOrganizationId()), "publisher of %s as %s", p.GetName(), org)
		}
	}
}
