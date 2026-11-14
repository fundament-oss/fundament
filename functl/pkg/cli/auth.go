package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"connectrpc.com/connect"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/functl/pkg/client"
	"github.com/fundament-oss/fundament/functl/pkg/config"
)

// AuthCmd contains authentication subcommands.
type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Login with an API key."`
	Status AuthStatusCmd `cmd:"" help:"Show current authentication status."`
	Logout AuthLogoutCmd `cmd:"" help:"Remove stored credentials."`
}

// AuthLoginCmd handles the login command.
type AuthLoginCmd struct {
	APIKey string `help:"API key to use for authentication. If not provided, will prompt." arg:"" optional:""`
}

// Run executes the login command.
func (c *AuthLoginCmd) Run(ctx *Context) error {
	apiKey := c.APIKey

	// If no API key provided, prompt for it
	if apiKey == "" {
		fmt.Print("Enter your API key: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		apiKey = strings.TrimSpace(input)
	}

	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// Validate the API key by trying to get user info
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	testClient := client.New(apiKey, cfg.APIEndpoint, cfg.AuthnURL, "")
	resp, err := testClient.Authn().GetUserInfo(context.Background(), authnv1.GetUserInfoRequest_builder{}.Build())
	if err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}

	// Save the credentials
	creds := &config.Credentials{
		APIKey: apiKey,
	}
	if err := config.SaveCredentials(creds); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	fmt.Printf("Logged in as %s\n", resp.GetUser().GetName())
	if len(resp.GetUser().GetOrganizationIds()) == 0 {
		fmt.Println()
		fmt.Println(NoOrganizationsHint)
	}
	return nil
}

// Values of "reason" in the JSON that `auth status` prints when the caller is not
// authenticated; scripts branch on them.
const (
	authReasonNotLoggedIn = "not_logged_in"
	authReasonRejected    = "rejected"
	authReasonUnreachable = "unreachable"
	authReasonError       = "error"
)

// classifyAuthError maps a failed user-info call to a reason and the error the
// command exits with.
func classifyAuthError(err error, authnURL string) (string, error) {
	switch connect.CodeOf(err) {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied:
		return authReasonRejected, fmt.Errorf("credentials rejected, they may be invalid or expired: check FUNDAMENT_API_KEY if it is set, else run 'functl auth login' to sign in again: %w", err)
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return authReasonUnreachable, fmt.Errorf("could not reach %s: %w", authnURL, err)
	default:
		return authReasonError, fmt.Errorf("checking authentication: %w", err)
	}
}

// reportNotAuthenticated writes the JSON answer for -o json and returns failure,
// so the command exits non-zero in both output formats.
func reportNotAuthenticated(w io.Writer, output OutputFormat, reason string, failure error) error {
	if output == OutputJSON {
		err := writeJSON(w, map[string]any{
			"authenticated": false,
			"reason":        reason,
			"message":       failure.Error(),
		})
		if err != nil {
			return err
		}
	}
	return failure
}

// AuthStatusCmd handles the status command. It exits 0 only when the API key
// (FUNDAMENT_API_KEY, else the stored one) authenticates.
type AuthStatusCmd struct{}

// Run executes the status command.
func (c *AuthStatusCmd) Run(ctx *Context) error {
	return c.run(ctx, os.Stdout)
}

// run writes the JSON answer to w, so every failure reaches -o json as a reason.
func (c *AuthStatusCmd) run(ctx *Context, w io.Writer) error {
	creds, err := config.LoadCredentials()
	if errors.Is(err, config.ErrNotAuthenticated) {
		return reportNotAuthenticated(w, ctx.Output, authReasonNotLoggedIn, err)
	}
	if err != nil {
		return reportNotAuthenticated(w, ctx.Output, authReasonError, err)
	}

	// Try to get user info
	cfg, err := config.LoadConfig()
	if err != nil {
		return reportNotAuthenticated(w, ctx.Output, authReasonError, fmt.Errorf("failed to load config: %w", err))
	}

	apiClient := client.New(creds.APIKey, cfg.APIEndpoint, cfg.AuthnURL, "")
	resp, err := apiClient.Authn().GetUserInfo(context.Background(), authnv1.GetUserInfoRequest_builder{}.Build())
	if err != nil {
		reason, failure := classifyAuthError(err, cfg.AuthnURL)
		return reportNotAuthenticated(w, ctx.Output, reason, failure)
	}

	user := resp.GetUser()

	if ctx.Output == OutputJSON {
		return writeJSON(w, map[string]any{
			"authenticated":    true,
			"user_id":          user.GetId(),
			"user_name":        user.GetName(),
			"organization_ids": user.GetOrganizationIds(),
		})
	}

	tw := NewTableWriter()
	PrintKeyValue(tw, "Authenticated", "yes")
	PrintKeyValue(tw, "User ID", user.GetId())
	PrintKeyValue(tw, "User Name", user.GetName())
	if len(user.GetOrganizationIds()) == 0 {
		PrintKeyValue(tw, "Organization IDs", "none")
	} else {
		PrintKeyValue(tw, "Organization IDs", user.GetOrganizationIds())
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}
	if len(user.GetOrganizationIds()) == 0 {
		fmt.Println()
		fmt.Println(NoOrganizationsHint)
	}
	return nil
}

// AuthLogoutCmd handles the logout command.
type AuthLogoutCmd struct{}

// Run executes the logout command.
func (c *AuthLogoutCmd) Run(ctx *Context) error {
	if err := config.DeleteCredentials(); err != nil {
		return fmt.Errorf("failed to delete credentials: %w", err)
	}
	fmt.Println("Logged out successfully")
	return nil
}
