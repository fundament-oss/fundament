package cli

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// UserCmd groups user-related commands.
type UserCmd struct {
	Create UserCreateCmd `cmd:"" help:"Register a user by email ahead of their first sign-in."`
	List   UserListCmd   `cmd:"" help:"List users and the organizations they belong to."`
	Delete UserDeleteCmd `cmd:"" help:"Delete a registration nobody has signed in with."`
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
	Organization        string `help:"Only accepted members of this organization (by name)."`
	WithoutOrganization bool   `help:"Only users that are an accepted member of no organization; an unanswered invitation does not count."`
}

// UserDeleteCmd soft-deletes a user nobody has signed in with, along with any
// memberships and invitations they hold. A registration made at a mistyped
// address would otherwise stay claimable by whoever signs in there. An account
// somebody has signed in to is not a registration and is refused.
type UserDeleteCmd struct {
	User string `arg:"" help:"User ID or email address." required:""`
}

// userRef is what a command accepts to name a user: a user ID or an email
// address. Names are not unique and are what the identity provider says they
// are, so they cannot be used to address a user.
type userRef struct {
	id    uuid.UUID
	email string
}

// parseUserRef reads "<user-id>|<email>". A UUID is a user ID; anything else
// has to be a bare, well-formed email address.
func parseUserRef(ref string) (userRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return userRef{}, errors.New("user is required: pass a user ID or an email address")
	}
	if id, err := uuid.Parse(ref); err == nil {
		return userRef{id: id}, nil
	}
	email, err := parseEmail(ref)
	if err != nil {
		return userRef{}, fmt.Errorf("invalid user %q: expected a user ID or an email address", ref)
	}
	return userRef{email: email}, nil
}

// parseEmail accepts a bare address and nothing else: no display name, no
// angle brackets, exactly one @ with something on both sides. A row registered
// at a malformed address could never be claimed by a sign-in, so it is better
// refused here than left with a membership nobody can use.
func parseEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return "", fmt.Errorf("invalid email address %q", s)
	}
	if addr.Name != "" || addr.Address != s {
		return "", fmt.Errorf("invalid email address %q: expected a bare address", s)
	}
	local, domain, ok := strings.Cut(addr.Address, "@")
	if !ok || local == "" || !strings.Contains(domain, ".") {
		return "", fmt.Errorf("invalid email address %q", s)
	}
	return addr.Address, nil
}

// userRow is the shape every lookup returns, so callers do not care whether the
// user was named by ID or by email.
type userRow struct {
	ID          uuid.UUID
	Name        string
	Email       string
	ExternalRef string
}

// errUserNotFound is returned by lookupUser when nothing matches the reference.
var errUserNotFound = errors.New("user not found")

// lookupUser resolves a userRef to an existing user, or errUserNotFound.
// Email is not unique in tenant.users. Several rows at one address is a data
// problem an operator should see, not one this tool should quietly pick a
// side of, so that is an error naming the rows; the user ID still works.
func lookupUser(ctx context.Context, queries *db.Queries, ref userRef) (userRow, error) {
	if ref.email == "" {
		u, err := queries.UserGetByID(ctx, db.UserGetByIDParams{ID: ref.id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return userRow{}, errUserNotFound
			}
			return userRow{}, fmt.Errorf("getting user by id: %w", err)
		}
		return userRow{ID: u.ID, Name: u.Name, Email: u.Email.String, ExternalRef: u.ExternalRef.String}, nil
	}
	rows, err := queries.UserFindByEmail(ctx, db.UserFindByEmailParams{Email: ref.email})
	if err != nil {
		return userRow{}, fmt.Errorf("finding user by email: %w", err)
	}
	switch len(rows) {
	case 0:
		return userRow{}, errUserNotFound
	case 1:
		u := rows[0]
		return userRow{ID: u.ID, Name: u.Name, Email: u.Email.String, ExternalRef: u.ExternalRef.String}, nil
	default:
		return userRow{}, fmt.Errorf("%d users match email %q (%s): pass the user ID instead", len(rows), ref.email, joinUserIDs(rows))
	}
}

// joinUserIDs lists the IDs of matching rows for an error message.
func joinUserIDs(rows []db.UserFindByEmailRow) string {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID.String()
	}
	return strings.Join(ids, ", ")
}

// registerUser registers an address nobody has signed in with, refusing one
// that is in use: an address that exists is a user to add, and quietly reusing
// it would hide a wrong assumption. Both user create and member add
// --create-user go through here.
func registerUser(ctx context.Context, queries *db.Queries, email, name string) (uuid.UUID, error) {
	existing, err := queries.UserFindByEmail(ctx, db.UserFindByEmailParams{Email: email})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to look up user: %w", err)
	}
	if len(existing) > 0 {
		return uuid.Nil, fmt.Errorf("user with email %q already exists (%s)", email, joinUserIDs(existing))
	}

	created, err := queries.UserCreate(ctx, db.UserCreateParams{Name: name, Email: email})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to register user: %w", err)
	}
	return created.ID, nil
}

// Run executes the user create command.
func (c *UserCreateCmd) Run(ctx *Context) error {
	email, err := parseEmail(c.Email)
	if err != nil {
		return err
	}

	name := c.Name
	if name == "" {
		name = email
	}

	ctx.Logger.Debug("creating user", "email", email, "name", name)

	id, err := registerUser(context.Background(), ctx.Queries, email, name)
	if err != nil {
		return err
	}

	ctx.Logger.Info("registered user; the account is linked when someone first signs in at this address", "email", email, "id", id.String())

	return outputCreatedID(ctx.Output, id)
}

// Run executes the user delete command.
func (c *UserDeleteCmd) Run(ctx *Context) error {
	ref, err := parseUserRef(c.User)
	if err != nil {
		return err
	}

	bgCtx := context.Background()

	tx, err := ctx.DB.Pool.Begin(bgCtx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer rollback.Rollback(bgCtx, tx, ctx.Logger)

	qtx := ctx.Queries.WithTx(tx)

	user, err := lookupUser(bgCtx, qtx, ref)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			return fmt.Errorf("user %q not found", c.User)
		}
		return fmt.Errorf("failed to look up user: %w", err)
	}
	if user.ExternalRef != "" {
		return fmt.Errorf("user %q has signed in and is not a registration; only registrations nobody has signed in with can be deleted", c.User)
	}

	ctx.Logger.Debug("deleting user registration", "user_id", user.ID.String(), "email", user.Email)

	revoked, err := qtx.MembershipRevokeAllForUser(bgCtx, db.MembershipRevokeAllForUserParams{UserID: user.ID})
	if err != nil {
		return fmt.Errorf("failed to revoke memberships: %w", err)
	}

	deleted, err := qtx.UserDeleteRegistration(bgCtx, db.UserDeleteRegistrationParams{ID: user.ID})
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if deleted == 0 {
		return fmt.Errorf("user %q not found", c.User)
	}

	if err := tx.Commit(bgCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	ctx.Logger.Info("deleted user registration", "user_id", user.ID.String(), "email", user.Email, "memberships_revoked", revoked)

	return nil
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

	// Both filters go by accepted membership; an invitation someone has not
	// answered is listed, but is not an organization they are in.
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

// userOutput is the JSON output structure for a user.
type userOutput struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	ExternalRef   string   `json:"external_ref"`
	Organizations []string `json:"organizations"`
	Invitations   []string `json:"invitations"`
	Created       string   `json:"created"`
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
				Invitations:   u.InvitationNames,
				Created:       u.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "ID\tNAME\tEMAIL\tEXTERNAL_REF\tORGANIZATIONS\tINVITED_TO\tCREATED"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range users {
			u := &users[i]
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				u.ID.String(),
				u.Name,
				u.Email.String,
				u.ExternalRef.String,
				formatOrganizationNames(u.OrganizationNames),
				formatInvitationNames(u.InvitationNames),
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

// formatInvitationNames renders the organizations a user is invited to; most
// users have none, and a blank keeps the column quiet.
func formatInvitationNames(names []string) string {
	return strings.Join(names, ",")
}
