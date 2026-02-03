package cli

import (
	"context"
	"fmt"
)

// APIKeyCmd contains API key subcommands.
type APIKeyCmd struct {
	List   APIKeyListCmd   `cmd:"" help:"List all API keys."`
	Create APIKeyCreateCmd `cmd:"" help:"Create a new API key."`
	Revoke APIKeyRevokeCmd `cmd:"" help:"Revoke an API key."`
}

// APIKeyListCmd handles the apikey list command.
type APIKeyListCmd struct{}

// Run executes the apikey list command.
func (c *APIKeyListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	apiKeys, err := apiClient.ListAPIKeys(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

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
		if key.CreatedAt != nil {
			created = key.CreatedAt.AsTime().Format(TimeFormat)
		}
		expires := "never"
		if key.ExpiresAt != nil {
			expires = key.ExpiresAt.AsTime().Format(TimeFormat)
		}
		lastUsed := "never"
		if key.LastUsedAt != nil {
			lastUsed = key.LastUsedAt.AsTime().Format(TimeFormat)
		}
		revoked := "no"
		if key.RevokedAt != nil {
			revoked = key.RevokedAt.AsTime().Format(TimeFormat)
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
	Name          string `arg:"" help:"Name for the API key."`
	ExpiresInDays *int64 `help:"Number of days until the key expires. Omit for no expiry." short:"e"`
}

// Run executes the apikey create command.
func (c *APIKeyCreateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.CreateAPIKey(context.Background(), c.Name, c.ExpiresInDays)
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(resp)
	}

	fmt.Println("API key created successfully!")
	fmt.Println()
	fmt.Printf("ID:    %s\n", resp.Id)
	fmt.Printf("Token: %s\n", resp.Token)
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

	if err := apiClient.RevokeAPIKey(context.Background(), c.APIKeyID); err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	fmt.Printf("API key %s has been revoked\n", c.APIKeyID)
	return nil
}
