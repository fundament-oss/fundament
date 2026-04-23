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

	apiKeyClient := organizationv1connect.NewAPIKeyServiceClient(http.DefaultClient, endpoint)

	const maxRetries = 30
	const retryInterval = 2 * time.Second

	// Create a dynamic test API key.
	// Retry until the authz-worker has written Alice's org-membership tuples to
	// OpenFGA (required by the CreateAPIKey permission check).
	fmt.Println("TestMain: creating dynamic test API key")
	var apiKeyToken, apiKeyID string
	for i := range maxRetries {
		createReq := connect.NewRequest(organizationv1.CreateAPIKeyRequest_builder{
			Name: "terraform-acc-test-" + acctest.RandString(6),
		}.Build())
		createReq.Header().Set("Authorization", "Bearer "+login.accessToken)
		createReq.Header().Set(organization.OrganizationHeader, orgID)

		createResp, err := apiKeyClient.CreateAPIKey(ctx, createReq)
		if err != nil {
			if connect.CodeOf(err) == connect.CodePermissionDenied {
				fmt.Printf("TestMain: CreateAPIKey not authorized yet (authz-worker pending), attempt %d/%d, retrying...\n", i+1, maxRetries)
				time.Sleep(retryInterval)
				continue
			}
			fmt.Fprintf(os.Stderr, "TestMain: CreateAPIKey failed: %v\n", err)
			os.Exit(1)
		}

		apiKeyToken = createResp.Msg.GetToken()
		apiKeyID = createResp.Msg.GetId()
		break
	}

	if apiKeyToken == "" {
		fmt.Fprintln(os.Stderr, "TestMain: could not create dynamic API key within timeout")
		os.Exit(1)
	}
	fmt.Printf("TestMain: created API key %s\n", apiKeyID)

	if err := os.Setenv("FUNDAMENT_API_KEY", apiKeyToken); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: failed to set FUNDAMENT_API_KEY: %v\n", err)
		os.Exit(1)
	}

	tokenClient := authnv1connect.NewTokenServiceClient(http.DefaultClient, authnEndpoint)
	clusterClient := organizationv1connect.NewClusterServiceClient(http.DefaultClient, endpoint)

	ready := false
	for i := range maxRetries {
		exchangeReq := connect.NewRequest(&authnv1.ExchangeTokenRequest{})
		exchangeReq.Header().Set("Authorization", "Bearer "+apiKeyToken)
		exchangeResp, err := tokenClient.ExchangeToken(ctx, exchangeReq)
		if err != nil {
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

	// Best-effort cleanup — delete the dynamic API key using a fresh JWT
	// (the earlier JWT may have expired after long test runs).
	fmt.Printf("TestMain: deleting dynamic API key %s\n", apiKeyID)
	cleanupLogin, err := passwordLogin(authnEndpoint, testUserEmail, testUserPassword)
	if err != nil {
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

	os.Exit(code)
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
