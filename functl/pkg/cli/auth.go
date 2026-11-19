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
	err = cfg.Resolve(context.Background())
	if err != nil {
		return err //nolint:wrapcheck // already names the installation and the cause
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
)

// notAuthenticated is the answer for a caller who is not authenticated.
type notAuthenticated struct {
	reason  string
	message string
	hint    string
}

var (
	notLoggedIn = notAuthenticated{
		reason:  authReasonNotLoggedIn,
		message: "not logged in",
		hint:    "Run 'functl auth login' to authenticate.",
	}
	keyRejected = notAuthenticated{
		reason:  authReasonRejected,
		message: "credentials rejected, they may be invalid or expired",
		hint:    "Check FUNDAMENT_API_KEY if it is set, else run 'functl auth login' to sign in again.",
	}
)

// AuthStatusCmd handles the status command. Not being authenticated is an answer, so
// the command exits 0 whenever the check ran; it fails only when it could not check,
// for example with an unreachable platform or an unreadable config or credentials file.
type AuthStatusCmd struct{}

// Run executes the status command.
func (c *AuthStatusCmd) Run(ctx *Context) error {
	return c.run(ctx, os.Stdout)
}

func (c *AuthStatusCmd) run(ctx *Context, w io.Writer) error {
	creds, err := config.LoadCredentials()
	if errors.Is(err, config.ErrNotAuthenticated) {
		return reportNotAuthenticated(w, ctx.Output, notLoggedIn)
	}
	if err != nil {
		return fmt.Errorf("checking authentication: %w", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	err = cfg.Resolve(context.Background())
	if err != nil {
		return err //nolint:wrapcheck // already names the installation and the cause
	}

	apiClient := client.New(creds.APIKey, cfg.APIEndpoint, cfg.AuthnURL, "")
	resp, err := apiClient.Authn().GetUserInfo(context.Background(), authnv1.GetUserInfoRequest_builder{}.Build())
	if err != nil {
		rejected, failure := classifyAuthError(err, cfg.AuthnURL)
		if rejected {
			return reportNotAuthenticated(w, ctx.Output, keyRejected)
		}
		return failure
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

// classifyAuthError tells a key the platform rejected, which is an answer, from a
// check that failed, which is returned as the command's error.
func classifyAuthError(err error, authnURL string) (rejected bool, failure error) {
	switch connect.CodeOf(err) {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied:
		return true, nil
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return false, fmt.Errorf("could not reach %s: %w", authnURL, err)
	default:
		return false, fmt.Errorf("checking authentication: %w", err)
	}
}

// reportNotAuthenticated prints the answer for a caller who is not authenticated:
// JSON under -o json, else the message and the hint.
func reportNotAuthenticated(w io.Writer, output OutputFormat, answer notAuthenticated) error {
	if output == OutputJSON {
		return writeJSON(w, map[string]any{
			"authenticated": false,
			"reason":        answer.reason,
			"message":       answer.message,
			"hint":          answer.hint,
		})
	}
	_, err := fmt.Fprintf(w, "Not authenticated: %s\n%s\n", answer.message, answer.hint)
	return err //nolint:wrapcheck // a failed write to stdout needs no context
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
