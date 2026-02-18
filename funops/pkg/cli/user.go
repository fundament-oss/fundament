package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// UserCmd groups user-related commands.
type UserCmd struct {
	Create UserCreateCmd `cmd:"" help:"Create a new user."`
	List   UserListCmd   `cmd:"" help:"List users in an organization."`
	Delete UserDeleteCmd `cmd:"" help:"Delete a user."`
}

// UserCreateCmd creates a new user.
type UserCreateCmd struct {
	Identifier  string `arg:"" help:"User identifier: <organization>/<user>." required:""`
	ExternalRef string `help:"External reference for the user." required:""`
}

// UserListCmd lists users in an organization.
type UserListCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

// UserDeleteCmd deletes a user.
type UserDeleteCmd struct {
	Identifier string `arg:"" help:"User identifier: <organization>/<user>." required:""`
}

// parseUserIdentifier splits "<organization>/<user>" into its parts.
func parseUserIdentifier(identifier string) (organization, user string, err error) {
	parts := strings.SplitN(identifier, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid user identifier '%s': expected format <organization>/<user>", identifier)
	}
	return parts[0], parts[1], nil
}

// Run executes the user create command.
func (c *UserCreateCmd) Run(ctx *Context) error {
	org, user, err := parseUserIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("creating user", "organization", org, "user", user, "external_ref", c.ExternalRef)

	// Create the user record
	u, err := ctx.Queries.UserCreate(context.Background(), db.UserCreateParams{
		Name:        user,
		ExternalRef: c.ExternalRef,
	})
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("user '%s' already exists", c.Identifier)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	_, err = ctx.Queries.UserCreateMembership(context.Background(), db.UserCreateMembershipParams{
		UserID:           u.ID,
		Permission:       dbconst.OrganizationsUserPermission_Viewer,
		Status:           dbconst.OrganizationsUserStatus_Accepted,
		OrganizationName: org,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization %q not found", org)
		}
		return fmt.Errorf("failed to create membership: %w", err)
	}

	ctx.Logger.Debug("user created", "id", u.ID.String())

	return outputUserCreate(ctx.Output, &u)
}

// Run executes the user list command.
func (c *UserListCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing users", "organization", c.Organization)

	orgID, err := ctx.Queries.OrganizationGetIDByName(context.Background(), db.OrganizationGetIDByNameParams{
		Name: c.Organization,
	})
	if err != nil {
		return fmt.Errorf("organization '%s' not found", c.Organization)
	}

	users, err := ctx.Queries.UserList(context.Background(), db.UserListParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	ctx.Logger.Debug("users listed", "count", len(users))

	return outputUserList(ctx.Output, c.Organization, users)
}

// Run executes the user delete command.
func (c *UserDeleteCmd) Run(ctx *Context) error {
	org, user, err := parseUserIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("deleting user", "organization", org, "user", user)

	rowsAffected, err := ctx.Queries.UserDelete(context.Background(), db.UserDeleteParams{
		OrganizationName: org,
		UserName:         user,
	})
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("user '%s' not found", c.Identifier)
	}

	ctx.Logger.Info("deleted user", "identifier", c.Identifier)

	return nil
}

// userCreateOutput is the JSON output structure for user create.
type userCreateOutput struct {
	ID string `json:"id"`
}

// userOutput is the JSON output structure for a user.
type userOutput struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	ExternalRef string `json:"external_ref"`
	Created     string `json:"created"`
}

func outputUserCreate(format OutputFormat, u *db.UserCreateRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(userCreateOutput{
			ID: u.ID.String(),
		})
	case OutputTable:
		fmt.Println(u.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputUserList(format OutputFormat, organization string, users []db.UserListRow) error {
	switch format {
	case OutputJSON:
		output := make([]userOutput, len(users))
		for i, u := range users {
			output[i] = userOutput{
				ID:          u.ID.String(),
				Identifier:  organization + "/" + u.Name,
				ExternalRef: u.ExternalRef.String,
				Created:     u.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tIDENTIFIER\tEXTERNAL_REF\tCREATED")
		for _, u := range users {
			fmt.Fprintf(w, "%s\t%s/%s\t%s\t%s\n",
				u.ID.String(),
				organization,
				u.Name,
				u.ExternalRef.String,
				u.Created.Time.Format(TimeFormat),
			)
		}
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
