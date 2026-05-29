package dcim_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/common/psqldb"
	"github.com/fundament-oss/fundament/dcim-api/pkg/dcim"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1/dcimv1connect"
)

const (
	validUUID   = "550e8400-e29b-41d4-a716-446655440000"
	invalidUUID = "not-a-uuid"
)

const testJWTSecret = "test-secret"

type testEnv struct {
	server    *httptest.Server
	adminPool *pgxpool.Pool
	testToken string
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

func (e *testEnv) client() *http.Client {
	base := e.server.Client()
	return &http.Client{Transport: &authTransport{base: base.Transport, token: e.testToken}}
}

func signTestToken(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": uuid.New().String(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return s
}

type apiOptions struct {
	t testing.TB
}

type APIOption func(*apiOptions)

func newTestAPI(t *testing.T, options ...APIOption) *testEnv {
	t.Helper()

	opts := apiOptions{t: t}
	for _, option := range options {
		option(&opts)
	}

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	testDB, adminPool := createTestDB(t, testLogger)

	srv := dcim.New(testLogger, testDB, []byte(testJWTSecret))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &testEnv{
		server:    ts,
		adminPool: adminPool,
		testToken: signTestToken(t),
	}
}

func createTestDB(t *testing.T, logger *slog.Logger) (*psqldb.DB, *pgxpool.Pool) {
	t.Helper()

	name := testNameToDBName(t.Name())
	createTestDatabase(t, name)

	testDB, err := psqldb.New(t.Context(), logger, psqldb.Config{
		URL: fmt.Sprintf("postgres://postgres:postgres@localhost:%d/%s?sslmode=disable", testDBPort, name),
	})
	require.NoError(t, err)
	t.Cleanup(testDB.Close)

	adminPool, err := pgxpool.New(t.Context(), fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/%s?sslmode=disable",
		testDBPort, name,
	))
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	return testDB, adminPool
}

func createTestDatabase(t *testing.T, name string) {
	t.Helper()

	adminURL := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", testDBPort)

	adminPool, err := pgxpool.New(context.Background(), adminURL)
	require.NoError(t, err)
	defer adminPool.Close()

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)

	_, err = adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %q TEMPLATE fundament`, name))
	require.NoError(t, err)
}

func testNameToDBName(testName string) string {
	name := strings.ToLower(testName)
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")

	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, want, connect.CodeOf(err))
}

func createSite(t *testing.T, env *testEnv, name string) string {
	t.Helper()

	client := dcimv1connect.NewSiteServiceClient(env.client(), env.server.URL)

	resp, err := client.CreateSite(context.Background(), connect.NewRequest(
		(&dcimv1.CreateSiteRequest_builder{Name: name}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetSiteId())

	return resp.Msg.GetSiteId()
}

func createRoom(t *testing.T, env *testEnv, siteID, name string) string {
	t.Helper()

	client := dcimv1connect.NewRoomServiceClient(env.client(), env.server.URL)

	resp, err := client.CreateRoom(context.Background(), connect.NewRequest(
		(&dcimv1.CreateRoomRequest_builder{SiteId: siteID, Name: name}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetRoomId())

	return resp.Msg.GetRoomId()
}

func createRackRow(t *testing.T, env *testEnv, roomID, name string) string {
	t.Helper()

	client := dcimv1connect.NewRackRowServiceClient(env.client(), env.server.URL)

	resp, err := client.CreateRackRow(context.Background(), connect.NewRequest(
		(&dcimv1.CreateRackRowRequest_builder{RoomId: roomID, Name: name}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetRackRowId())

	return resp.Msg.GetRackRowId()
}

// createRackRowFixture bootstraps a site → room → rack row chain and returns
// the rack row id so tests targeting the RackService have a valid parent to
// attach racks to.
func createRackRowFixture(t *testing.T, env *testEnv, prefix string) string {
	t.Helper()

	siteID := createSite(t, env, prefix+" site")
	roomID := createRoom(t, env, siteID, prefix+" room")

	return createRackRow(t, env, roomID, prefix+" row")
}

func createRack(t *testing.T, env *testEnv, rowID, name string, totalUnits int32) string {
	t.Helper()

	client := dcimv1connect.NewRackServiceClient(env.client(), env.server.URL)

	resp, err := client.CreateRack(context.Background(), connect.NewRequest(
		(&dcimv1.CreateRackRequest_builder{
			RowId:      rowID,
			Name:       name,
			TotalUnits: totalUnits,
		}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetRackId())

	return resp.Msg.GetRackId()
}

func createCatalogEntry(t *testing.T, env *testEnv, model string) string {
	t.Helper()

	client := dcimv1connect.NewCatalogServiceClient(env.client(), env.server.URL)

	resp, err := client.CreateCatalogEntry(context.Background(), connect.NewRequest(
		(&dcimv1.CreateCatalogEntryRequest_builder{
			Manufacturer: "Test Mfr",
			Model:        model,
			PartNumber:   model + "-PN",
			Category:     dcimv1.AssetCategory_ASSET_CATEGORY_SERVER,
		}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetCatalogEntryId())

	return resp.Msg.GetCatalogEntryId()
}

func createAsset(t *testing.T, env *testEnv, catalogID string) string {
	t.Helper()

	client := dcimv1connect.NewAssetServiceClient(env.client(), env.server.URL)

	resp, err := client.CreateAsset(context.Background(), connect.NewRequest(
		(&dcimv1.CreateAssetRequest_builder{
			DeviceCatalogId: catalogID,
			Status:          dcimv1.AssetStatus_ASSET_STATUS_DEPLOYED,
		}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetAssetId())

	return resp.Msg.GetAssetId()
}

func placeAssetInRack(t *testing.T, env *testEnv, assetID, rackID string, unit int32) string {
	t.Helper()

	client := dcimv1connect.NewPlacementServiceClient(env.client(), env.server.URL)

	resp, err := client.CreatePlacement(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePlacementRequest_builder{
			AssetId: assetID,
			Rack: (&dcimv1.RackLocation_builder{
				RackId:        rackID,
				RackUnitStart: unit,
				RackSlotType:  dcimv1.RackSlotType_RACK_SLOT_TYPE_UNIT,
			}).Build(),
		}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetPlacementId())

	return resp.Msg.GetPlacementId()
}

func placeAssetInSubComponent(t *testing.T, env *testEnv, assetID, parentPlacementID, parentPortDefinitionID string) string {
	t.Helper()

	client := dcimv1connect.NewPlacementServiceClient(env.client(), env.server.URL)

	resp, err := client.CreatePlacement(context.Background(), connect.NewRequest(
		(&dcimv1.CreatePlacementRequest_builder{
			AssetId: assetID,
			SubComponent: (&dcimv1.SubComponentLocation_builder{
				ParentPlacementId:      parentPlacementID,
				ParentPortDefinitionId: parentPortDefinitionID,
			}).Build(),
		}).Build(),
	))
	require.NoError(t, err)

	require.NotEmpty(t, resp.Msg.GetPlacementId())

	return resp.Msg.GetPlacementId()
}
