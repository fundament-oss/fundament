package cli

import (
	"bufio"
	"context"
	"fmt"
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
		return err
	}

	testClient := client.New(apiKey, cfg.APIEndpoint, cfg.AuthnURL, "")
	resp, err := testClient.Authn().GetUserInfo(context.Background(), connect.NewRequest(&authnv1.GetUserInfoRequest{}))
	if err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}

	// Save the credentials
	creds := &config.Credentials{
		APIKey: apiKey,
	}
	if err := config.SaveCredentials(creds); err != nil {
		return err
	}

	fmt.Printf("Logged in as %s\n", resp.Msg.User.Name)
	return nil
}

// AuthStatusCmd handles the status command.
type AuthStatusCmd struct{}

// Run executes the status command.
func (c *AuthStatusCmd) Run(ctx *Context) error {
	creds, err := config.LoadCredentials()
	if err != nil {
		fmt.Println("Not authenticated")
		fmt.Println("Run 'functl auth login' to authenticate")
		return nil
	}

	// Try to get user info
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	apiClient := client.New(creds.APIKey, cfg.APIEndpoint, cfg.AuthnURL, "")
	resp, err := apiClient.Authn().GetUserInfo(context.Background(), connect.NewRequest(&authnv1.GetUserInfoRequest{}))
	if err != nil {
		fmt.Println("Authentication failed: credentials may be invalid or expired")
		fmt.Println("Run 'functl auth login' to re-authenticate")
		return nil
	}

	user := resp.Msg.User

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]any{
			"authenticated":    true,
			"user_id":          user.Id,
			"user_name":        user.Name,
			"organization_ids": user.OrganizationIds,
		})
	}

	w := NewTableWriter()
	PrintKeyValue(w, "Authenticated", "yes")
	PrintKeyValue(w, "User ID", user.Id)
	PrintKeyValue(w, "User Name", user.Name)
	PrintKeyValue(w, "Organization IDs", user.OrganizationIds)
	return w.Flush()
}

// AuthLogoutCmd handles the logout command.
type AuthLogoutCmd struct{}

// Run executes the logout command.
func (c *AuthLogoutCmd) Run(ctx *Context) error {
	if err := config.DeleteCredentials(); err != nil {
		return err
	}
	fmt.Println("Logged out successfully")
	return nil
}
