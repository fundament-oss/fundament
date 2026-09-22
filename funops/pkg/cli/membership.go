package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// OrganizationMemberCmd groups organization membership commands. Users cannot
// create or join organizations themselves; an operator assigns them here.
type OrganizationMemberCmd struct {
	Add    OrganizationMemberAddCmd    `cmd:"" help:"Assign a user to an organization."`
	List   OrganizationMemberListCmd   `cmd:"" help:"List the members of an organization."`
	Remove OrganizationMemberRemoveCmd `cmd:"" help:"Remove a user from an organization."`
}

// OrganizationMemberAddCmd assigns a user to an organization.
type OrganizationMemberAddCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
	User         string `arg:"" help:"User ID or email address. An unknown email address is registered first, so the membership is waiting when that person signs in." required:""`
	Permission   string `help:"Permission in the organization: viewer or admin." default:"viewer" enum:"viewer,admin"`
}

// OrganizationMemberListCmd lists the members of an organization.
type OrganizationMemberListCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

// OrganizationMemberRemoveCmd removes a user from an organization.
type OrganizationMemberRemoveCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
	User         string `arg:"" help:"User ID or email address." required:""`
}

// lookupOrganizationID resolves an organization name to its ID.
func lookupOrganizationID(ctx context.Context, queries *db.Queries, name string) (uuid.UUID, error) {
	id, err := queries.OrganizationGetIDByName(ctx, db.OrganizationGetIDByNameParams{Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("organization %q not found", name)
		}
		return uuid.Nil, fmt.Errorf("failed to look up organization: %w", err)
	}
	return id, nil
}

// Run executes the organization member add command.
func (c *OrganizationMemberAddCmd) Run(ctx *Context) error {
	ref, err := parseUserRef(c.User)
	if err != nil {
		return err
	}

	permission, err := parsePermission(c.Permission)
	if err != nil {
		return err
	}

	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("adding organization member", "organization", c.Organization, "user", c.User, "permission", permission)

	user, err := lookupUser(bgCtx, ctx.Queries, ref)
	switch {
	case err == nil:
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("failed to look up user: %w", err)
	case ref.email == "":
		return fmt.Errorf("user %q not found", c.User)
	default:
		// Nobody has signed in at this address yet: register it, so the
		// membership is already theirs when they do.
		created, err := ctx.Queries.UserCreate(bgCtx, db.UserCreateParams{
			Name:  ref.email,
			Email: ref.email,
		})
		if err != nil {
			return fmt.Errorf("failed to register user: %w", err)
		}
		user = userRow{ID: created.ID, Name: created.Name, Email: created.Email.String}
		ctx.Logger.Info("registered user; the account is linked when someone first signs in at this address", "email", ref.email, "id", user.ID.String())
	}

	membership, err := ctx.Queries.MembershipCreate(bgCtx, db.MembershipCreateParams{
		OrganizationID: orgID,
		UserID:         user.ID,
		Permission:     permission,
	})
	if err == nil {
		ctx.Logger.Info("added organization member", "organization", c.Organization, "user_id", user.ID.String(), "email", user.Email, "permission", permission)
		return outputMembershipCreate(ctx.Output, membership.ID)
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); !ok || pgErr.ConstraintName != dbconst.ConstraintOrganizationsUsersUqUser {
		return fmt.Errorf("failed to add organization member: %w", err)
	}

	// The user already has a live membership. If it is an invitation nobody
	// answered yet, assigning them is the answer; otherwise there is nothing to do.
	accepted, err := ctx.Queries.MembershipAcceptPending(bgCtx, db.MembershipAcceptPendingParams{
		OrganizationID: orgID,
		UserID:         user.ID,
		Permission:     permission,
	})
	if err != nil {
		return fmt.Errorf("failed to accept pending invitation: %w", err)
	}
	if accepted == 0 {
		return fmt.Errorf("user %q is already a member of organization %q", c.User, c.Organization)
	}

	ctx.Logger.Info("accepted pending invitation on the user's behalf", "organization", c.Organization, "user_id", user.ID.String(), "email", user.Email, "permission", permission)

	return nil
}

// Run executes the organization member list command.
func (c *OrganizationMemberListCmd) Run(ctx *Context) error {
	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("listing organization members", "organization", c.Organization)

	members, err := ctx.Queries.MembershipList(bgCtx, db.MembershipListParams{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to list organization members: %w", err)
	}

	ctx.Logger.Debug("organization members listed", "count", len(members))

	return outputMembershipList(ctx.Output, members)
}

// Run executes the organization member remove command.
func (c *OrganizationMemberRemoveCmd) Run(ctx *Context) error {
	ref, err := parseUserRef(c.User)
	if err != nil {
		return err
	}

	bgCtx := context.Background()

	orgID, err := lookupOrganizationID(bgCtx, ctx.Queries, c.Organization)
	if err != nil {
		return err
	}

	user, err := lookupUser(bgCtx, ctx.Queries, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("user %q not found", c.User)
		}
		return fmt.Errorf("failed to look up user: %w", err)
	}

	ctx.Logger.Debug("removing organization member", "organization", c.Organization, "user_id", user.ID.String())

	rowsAffected, err := ctx.Queries.MembershipDelete(bgCtx, db.MembershipDeleteParams{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove organization member: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user %q is not a member of organization %q", c.User, c.Organization)
	}

	ctx.Logger.Info("removed organization member", "organization", c.Organization, "user_id", user.ID.String(), "email", user.Email)

	return nil
}

// parsePermission maps the flag value onto the database constant.
func parsePermission(permission string) (dbconst.OrganizationsUserPermission, error) {
	switch permission {
	case "admin":
		return dbconst.OrganizationsUserPermission_Admin, nil
	case "viewer":
		return dbconst.OrganizationsUserPermission_Viewer, nil
	default:
		return "", fmt.Errorf("invalid permission %q: expected viewer or admin", permission)
	}
}

// membershipCreateOutput is the JSON output structure for organization member add.
type membershipCreateOutput struct {
	ID string `json:"id"`
}

// membershipOutput is the JSON output structure for an organization member.
type membershipOutput struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	ExternalRef string `json:"external_ref"`
	Permission  string `json:"permission"`
	Status      string `json:"status"`
	Created     string `json:"created"`
}

func outputMembershipCreate(format OutputFormat, id uuid.UUID) error {
	switch format {
	case OutputJSON:
		return PrintJSON(membershipCreateOutput{
			ID: id.String(),
		})
	case OutputTable:
		fmt.Println(id.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputMembershipList(format OutputFormat, members []db.MembershipListRow) error {
	switch format {
	case OutputJSON:
		output := make([]membershipOutput, len(members))
		for i := range members {
			m := &members[i]
			output[i] = membershipOutput{
				ID:          m.ID.String(),
				UserID:      m.UserID.String(),
				Name:        m.Name,
				Email:       m.Email.String,
				ExternalRef: m.ExternalRef.String,
				Permission:  string(m.Permission),
				Status:      string(m.Status),
				Created:     m.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "USER_ID\tNAME\tEMAIL\tPERMISSION\tSTATUS\tCREATED"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range members {
			m := &members[i]
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				m.UserID.String(),
				m.Name,
				m.Email.String,
				m.Permission,
				m.Status,
				m.Created.Time.Format(TimeFormat),
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
