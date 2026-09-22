package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// UserCmd groups user-related commands.
type UserCmd struct {
	Create UserCreateCmd `cmd:"" help:"Register a user by email ahead of their first sign-in."`
	List   UserListCmd   `cmd:"" help:"List users and the organizations they belong to."`
}

// UserCreateCmd registers a user by email before they have signed in.
// Signing in never creates an organization, so this is how an operator gets a
// user into an organization before that person shows up: register the address,
// then 'funops organization member add'. The authn-api links the row to the
// identity provider account on the first sign-in at this address.
type UserCreateCmd struct {
	Email string `arg:"" help:"Email address the user will sign in with." required:""`
	Name  string `help:"Display name. Defaults to the email address; replaced by the identity provider's name at first sign-in."`
}

// UserListCmd lists users.
type UserListCmd struct {
	Organization        string `help:"Only users that belong to this organization (by name)."`
	WithoutOrganization bool   `help:"Only users that belong to no organization at all."`
}

// userRef is what a command accepts to name a user: a user ID or an email
// address. Names are not unique and are what the identity provider says they
// are, so they cannot be used to address a user.
type userRef struct {
	id    uuid.UUID
	email string
}

// parseUserRef reads "<user-id>|<email>". A UUID is a user ID; anything with an
// @ is an email address.
func parseUserRef(ref string) (userRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return userRef{}, errors.New("user is required: pass a user ID or an email address")
	}
	if id, err := uuid.Parse(ref); err == nil {
		return userRef{id: id}, nil
	}
	if strings.Contains(ref, "@") {
		return userRef{email: ref}, nil
	}
	return userRef{}, fmt.Errorf("invalid user %q: expected a user ID or an email address", ref)
}

// userRow is the shape every lookup returns, so callers do not care whether the
// user was named by ID or by email.
type userRow struct {
	ID          uuid.UUID
	Name        string
	Email       string
	ExternalRef string
}

// lookupUser resolves a userRef to an existing user, or pgx.ErrNoRows.
func lookupUser(ctx context.Context, queries *db.Queries, ref userRef) (userRow, error) {
	if ref.email == "" {
		u, err := queries.UserGetByID(ctx, db.UserGetByIDParams{ID: ref.id})
		if err != nil {
			return userRow{}, fmt.Errorf("getting user by id: %w", err)
		}
		return userRow{ID: u.ID, Name: u.Name, Email: u.Email.String, ExternalRef: u.ExternalRef.String}, nil
	}
	u, err := queries.UserFindByEmail(ctx, db.UserFindByEmailParams{Email: ref.email})
	if err != nil {
		return userRow{}, fmt.Errorf("finding user by email: %w", err)
	}
	return userRow{ID: u.ID, Name: u.Name, Email: u.Email.String, ExternalRef: u.ExternalRef.String}, nil
}

// Run executes the user create command.
func (c *UserCreateCmd) Run(ctx *Context) error {
	if !strings.Contains(c.Email, "@") {
		return fmt.Errorf("invalid email address %q", c.Email)
	}

	name := c.Name
	if name == "" {
		name = c.Email
	}

	ctx.Logger.Debug("creating user", "email", c.Email, "name", name)

	existing, err := ctx.Queries.UserFindByEmail(context.Background(), db.UserFindByEmailParams{Email: c.Email})
	if err == nil {
		return fmt.Errorf("user with email %q already exists: %s", c.Email, existing.ID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to look up user: %w", err)
	}

	u, err := ctx.Queries.UserCreate(context.Background(), db.UserCreateParams{
		Name:  name,
		Email: c.Email,
	})
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	ctx.Logger.Info("registered user; the account is linked when someone first signs in at this address", "email", c.Email, "id", u.ID.String())

	return outputUserCreate(ctx.Output, u.ID)
}

// Run executes the user list command.
func (c *UserListCmd) Run(ctx *Context) error {
	if c.Organization != "" && c.WithoutOrganization {
		return errors.New("--organization and --without-organization cannot be combined")
	}

	ctx.Logger.Debug("listing users", "organization", c.Organization, "without_organization", c.WithoutOrganization)

	if c.Organization != "" {
		if _, err := ctx.Queries.OrganizationGetIDByName(context.Background(), db.OrganizationGetIDByNameParams{Name: c.Organization}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("organization %q not found", c.Organization)
			}
			return fmt.Errorf("failed to look up organization: %w", err)
		}
	}

	users, err := ctx.Queries.UserList(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	users = slices.DeleteFunc(users, func(u db.UserListRow) bool {
		switch {
		case c.WithoutOrganization:
			return len(u.OrganizationNames) != 0
		case c.Organization != "":
			return !slices.Contains(u.OrganizationNames, c.Organization)
		default:
			return false
		}
	})

	ctx.Logger.Debug("users listed", "count", len(users))

	return outputUserList(ctx.Output, users)
}

// userCreateOutput is the JSON output structure for user create.
type userCreateOutput struct {
	ID string `json:"id"`
}

// userOutput is the JSON output structure for a user.
type userOutput struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	ExternalRef   string   `json:"external_ref"`
	Organizations []string `json:"organizations"`
	Created       string   `json:"created"`
}

func outputUserCreate(format OutputFormat, id uuid.UUID) error {
	switch format {
	case OutputJSON:
		return PrintJSON(userCreateOutput{
			ID: id.String(),
		})
	case OutputTable:
		fmt.Println(id.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputUserList(format OutputFormat, users []db.UserListRow) error {
	switch format {
	case OutputJSON:
		output := make([]userOutput, len(users))
		for i, u := range users {
			output[i] = userOutput{
				ID:            u.ID.String(),
				Name:          u.Name,
				Email:         u.Email.String,
				ExternalRef:   u.ExternalRef.String,
				Organizations: u.OrganizationNames,
				Created:       u.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "ID\tNAME\tEMAIL\tEXTERNAL_REF\tORGANIZATIONS\tCREATED"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range users {
			u := &users[i]
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				u.ID.String(),
				u.Name,
				u.Email.String,
				u.ExternalRef.String,
				formatOrganizationNames(u.OrganizationNames),
				u.Created.Time.Format(TimeFormat),
			); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
		}
		if err := w.Flush(); err != nil {
			return fmt.Errorf("flushing output: %w", err)
		}
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

// formatOrganizationNames renders a user's organizations for the table; a user
// without any is the case operators look for, so it says so in words.
func formatOrganizationNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ",")
}
