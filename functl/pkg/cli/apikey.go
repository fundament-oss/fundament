package cli

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// APIKeyCmd contains API key subcommands.
type APIKeyCmd struct {
	List   APIKeyListCmd   `cmd:"" help:"List all API keys."`
	Create APIKeyCreateCmd `cmd:"" help:"Create a new API key."`
	Revoke APIKeyRevokeCmd `cmd:"" help:"Revoke an API key."`
	Delete APIKeyDeleteCmd `cmd:"" help:"Delete an API key."`
}

// APIKeyListCmd handles the apikey list command.
type APIKeyListCmd struct{}

// Run executes the apikey list command.
func (c *APIKeyListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.APIKeys().ListAPIKeys(context.Background(), connect.NewRequest(&organizationv1.ListAPIKeysRequest{}))
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	apiKeys := resp.Msg.ApiKeys

	if ctx.Output == OutputJSON {
		return PrintJSON(apiKeys)
	}

	if len(apiKeys) == 0 {
		fmt.Println("No API keys found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "ID\tNAME\tPREFIX\tCREATED\tEXPIRES\tLAST USED\tREVOKED")
	for _, key := range apiKeys {
		created := ""
		if key.Created.IsValid() {
			created = key.Created.AsTime().Format(TimeFormat)
		}
		expires := "never"
		if key.Expires.IsValid() {
			expires = key.Expires.AsTime().Format(TimeFormat)
		}
		lastUsed := "never"
		if key.LastUsed.IsValid() {
			lastUsed = key.LastUsed.AsTime().Format(TimeFormat)
		}
		revoked := "no"
		if key.Revoked.IsValid() {
			revoked = key.Revoked.AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s...\t%s\t%s\t%s\t%s\n",
			key.Id,
			key.Name,
			key.TokenPrefix,
			created,
			expires,
			lastUsed,
			revoked,
		)
	}
	return w.Flush()
}

// APIKeyCreateCmd handles the apikey create command.
type APIKeyCreateCmd struct {
	Name      string `arg:"" help:"Name for the API key."`
	ExpiresIn string `help:"How long until the Key expires. Format is specified in string, e.g. '1h', '300s', or '5m'. Omit for no expiry." short:"e"`
}

// Run executes the apikey create command.
func (c *APIKeyCreateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	req := &organizationv1.CreateAPIKeyRequest{
		Name:      c.Name,
		ExpiresIn: c.ExpiresIn,
	}

	resp, err := apiClient.APIKeys().CreateAPIKey(context.Background(), connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(resp.Msg)
	}

	fmt.Println("API key created successfully!")
	fmt.Println()
	fmt.Printf("ID:    %s\n", resp.Msg.Id)
	fmt.Printf("Token: %s\n", resp.Msg.Token)
	fmt.Println()
	fmt.Println("IMPORTANT: Copy this token now. You will not be able to see it again.")
	return nil
}

// APIKeyRevokeCmd handles the apikey revoke command.
type APIKeyRevokeCmd struct {
	APIKeyID string `arg:"" help:"ID of the API key to revoke."`
}

// Run executes the apikey revoke command.
func (c *APIKeyRevokeCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	_, err = apiClient.APIKeys().RevokeAPIKey(context.Background(), connect.NewRequest(&organizationv1.RevokeAPIKeyRequest{
		ApiKeyId: c.APIKeyID,
	}))
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	fmt.Printf("API key %s has been revoked\n", c.APIKeyID)
	return nil
}

// APIKeyDeleteCmd handles the apikey delete command.
type APIKeyDeleteCmd struct {
	APIKeyID string `arg:"" help:"ID of the API key to delete."`
}

// Run executes the apikey delete command.
func (c *APIKeyDeleteCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	_, err = apiClient.APIKeys().DeleteAPIKey(context.Background(), connect.NewRequest(&organizationv1.DeleteAPIKeyRequest{
		ApiKeyId: c.APIKeyID,
	}))
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	fmt.Printf("API key %s has been deleted\n", c.APIKeyID)
	return nil
}
