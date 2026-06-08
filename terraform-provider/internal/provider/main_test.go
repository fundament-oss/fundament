package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	authnv1connect "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	organizationv1connect "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
)

const (
	testUserEmail    = "alice@acme-corp.com"
	testUserPassword = "password"
)

func TestMain(m *testing.M) {
	if os.Getenv("TF_ACC") != "1" {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	endpoint := os.Getenv("FUNDAMENT_ENDPOINT")
	if endpoint == "" {
		fmt.Fprintln(os.Stderr, "FUNDAMENT_ENDPOINT is required for ACC tests")
		os.Exit(1)
	}

	authnEndpoint := deriveAuthnEndpoint(endpoint)

	login, err := passwordLogin(authnEndpoint, testUserEmail, testUserPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: login failed: %v\n", err)
		os.Exit(1)
	}

	if len(login.organizationIDs) == 0 {
		fmt.Fprintln(os.Stderr, "TestMain: no organization IDs in login response")
		os.Exit(1)
	}

	orgID := login.organizationIDs[0]
	fmt.Printf("TestMain: using organization ID: %s\n", orgID)
	if err := os.Setenv("FUNDAMENT_ORGANIZATION_ID", orgID); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: failed to set FUNDAMENT_ORGANIZATION_ID: %v\n", err)
		os.Exit(1)
	}

	// 60 retries × 2 s = 2 min; bumped from 30 to accommodate slower CI
	// environments where the authz-worker takes longer to propagate tuples.
	const maxRetries = 60
	const retryInterval = 2 * time.Second

	// If FUNDAMENT_API_KEY is already set (e.g. when the authz-worker is not
	// running locally), skip dynamic key creation and use the provided key directly.
	apiKeyToken := os.Getenv("FUNDAMENT_API_KEY")
	usingPresetKey := apiKeyToken != ""
	var apiKeyID string // only set when we create the key dynamically; used for cleanup

	if usingPresetKey {
		fmt.Println("TestMain: using pre-set FUNDAMENT_API_KEY, skipping dynamic key creation")
	} else {
		apiKeyToken, apiKeyID, err = createDynamicAPIKey(ctx, endpoint, orgID, login.accessToken, maxRetries, retryInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("TestMain: created API key %s\n", apiKeyID)

		if err := os.Setenv("FUNDAMENT_API_KEY", apiKeyToken); err != nil {
			fmt.Fprintf(os.Stderr, "TestMain: failed to set FUNDAMENT_API_KEY: %v\n", err)
			os.Exit(1)
		}
	}

	tokenClient := authnv1connect.NewTokenServiceClient(http.DefaultClient, authnEndpoint)
	clusterClient := organizationv1connect.NewClusterServiceClient(http.DefaultClient, endpoint)

	ready := false
	for i := range maxRetries {
		exchangeReq := connect.NewRequest(&authnv1.ExchangeTokenRequest{})
		exchangeReq.Header().Set("Authorization", "Bearer "+apiKeyToken)
		exchangeResp, err := tokenClient.ExchangeToken(ctx, exchangeReq)
		if err != nil {
			code := connect.CodeOf(err)
			// For a pre-set key, unauthenticated/internal means the key is
			// permanently invalid — retrying won't help.
			// For a dynamically created key, unauthenticated can be transient
			// while the authn-worker propagates the new key.
			if usingPresetKey && (code == connect.CodeInternal || code == connect.CodeUnauthenticated) {
				fmt.Fprintf(os.Stderr, "TestMain: token exchange failed permanently — FUNDAMENT_API_KEY may be invalid or not found on the server: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("TestMain: token exchange attempt %d/%d failed: %v\n", i+1, maxRetries, err)
			time.Sleep(retryInterval)
			continue
		}

		listReq := connect.NewRequest(&organizationv1.ListClustersRequest{})
		listReq.Header().Set("Authorization", "Bearer "+exchangeResp.Msg.GetAccessToken())
		listReq.Header().Set(organization.OrganizationHeader, orgID)
		_, err = clusterClient.ListClusters(ctx, listReq)
		if err != nil {
			if connect.CodeOf(err) == connect.CodePermissionDenied {
				fmt.Printf("TestMain: authz not ready yet, attempt %d/%d, retrying...\n", i+1, maxRetries)
				time.Sleep(retryInterval)
				continue
			}
		}

		ready = true
		break
	}

	if !ready {
		fmt.Fprintln(os.Stderr, "TestMain: authz did not become ready within timeout")
		os.Exit(1)
	}

	code := m.Run()

	// Best-effort cleanup: a pre-set key is the caller's to manage; only delete
	// keys we created dynamically.
	if !usingPresetKey {
		apiKeyClient := organizationv1connect.NewAPIKeyServiceClient(http.DefaultClient, endpoint)
		fmt.Printf("TestMain: deleting dynamic API key %s\n", apiKeyID)
		cleanupLogin, err := passwordLogin(authnEndpoint, testUserEmail, testUserPassword)
		if err != nil {
			// Best-effort: a cleanup failure must not change the test exit code.
			fmt.Fprintf(os.Stderr, "TestMain: failed to get cleanup JWT (best-effort): %v\n", err)
		} else {
			deleteReq := connect.NewRequest(organizationv1.DeleteAPIKeyRequest_builder{
				ApiKeyId: apiKeyID,
			}.Build())
			deleteReq.Header().Set("Authorization", "Bearer "+cleanupLogin.accessToken)
			deleteReq.Header().Set(organization.OrganizationHeader, orgID)
			if _, err := apiKeyClient.DeleteAPIKey(ctx, deleteReq); err != nil {
				fmt.Fprintf(os.Stderr, "TestMain: failed to delete API key (best-effort): %v\n", err)
			}
		}
	}

	os.Exit(code)
}

// createDynamicAPIKey creates a test API key, retrying until the authz-worker
// has written Alice's org-membership tuples to OpenFGA (required by the
// CreateAPIKey permission check). It returns the key token and its ID; the ID
// is needed for cleanup.
func createDynamicAPIKey(ctx context.Context, endpoint, orgID, accessToken string, maxRetries int, retryInterval time.Duration) (token, id string, err error) {
	apiKeyClient := organizationv1connect.NewAPIKeyServiceClient(http.DefaultClient, endpoint)

	fmt.Println("TestMain: creating dynamic test API key")
	for i := range maxRetries {
		createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
			Name: "terraform-acc-test-" + acctest.RandString(6),
		}.Build())
		createReq.Header().Set("Authorization", "Bearer "+accessToken)
		createReq.Header().Set(organization.OrganizationHeader, orgID)

		createResp, err := apiKeyClient.CreateAPIKey(ctx, createReq)
		if err != nil {
			if connect.CodeOf(err) == connect.CodePermissionDenied {
				fmt.Printf("TestMain: CreateAPIKey not authorized yet (authz-worker pending), attempt %d/%d, retrying...\n", i+1, maxRetries)
				time.Sleep(retryInterval)
				continue
			}
			return "", "", fmt.Errorf("CreateAPIKey failed: %w", err)
		}

		return createResp.Msg.GetToken(), createResp.Msg.GetId(), nil
	}

	return "", "", fmt.Errorf("could not create dynamic API key within timeout")
}

type loginResponse struct {
	accessToken     string
	organizationIDs []string
}

func passwordLogin(authnEndpoint, email, password string) (loginResponse, error) {
	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		return loginResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(authnEndpoint+"/login/password", "application/json", bytes.NewReader(body))
	if err != nil {
		return loginResponse{}, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return loginResponse{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		User        struct {
			OrganizationIds []string `json:"organization_ids"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return loginResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return loginResponse{}, fmt.Errorf("empty access token in response")
	}
	return loginResponse{
		accessToken:     tokenResp.AccessToken,
		organizationIDs: tokenResp.User.OrganizationIds,
	}, nil
}
