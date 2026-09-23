package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// OrganizationMemberCmd groups organization membership commands. Users cannot
// create or join organizations themselves; an operator assigns them here.
type OrganizationMemberCmd struct {
	Add    OrganizationMemberAddCmd    `cmd:"" help:"Assign a user to an organization."`
	List   OrganizationMemberListCmd   `cmd:"" help:"List the members of an organization."`
	Remove OrganizationMemberRemoveCmd `cmd:"" help:"Remove a user from an organization."`
}

// OrganizationMemberAddCmd assigns a user to an organization. The user has
// to exist unless --create-user says to register the address: an add that
// silently created accounts would turn a typo into an unclaimable membership.
type OrganizationMemberAddCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
	User         string `arg:"" help:"User ID or email address of an existing user." required:""`
	Permission   string `help:"Permission in the organization: viewer or admin." default:"viewer" enum:"viewer,admin"`
	CreateUser   bool   `help:"Register the email address as a new user first, for someone who has not signed in yet. The membership is then waiting when they do."`
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

// Run executes the organization member add command. One transaction: the
// user is either registered or looked up, then the membership is written, so
// a failure leaves neither half behind.
func (c *OrganizationMemberAddCmd) Run(ctx *Context) error {
	ref, err := parseUserRef(c.User)
	if err != nil {
		return err
	}
	if c.CreateUser && ref.email == "" {
		return errors.New("--create-user registers an email address; pass the address rather than a user ID")
	}

	permission, err := parsePermission(c.Permission)
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

	orgID, err := lookupOrganizationID(bgCtx, qtx, c.Organization)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("adding organization member", "organization", c.Organization, "user", c.User, "permission", permission, "create_user", c.CreateUser)

	var userID uuid.UUID
	if c.CreateUser {
		// Refused rather than reused: an address that exists is a user to add
		// without the flag, and quietly doing that would hide a wrong assumption.
		existing, err := qtx.UserFindByEmail(bgCtx, db.UserFindByEmailParams{Email: ref.email})
		if err != nil {
			return fmt.Errorf("failed to look up user: %w", err)
		}
		if len(existing) > 0 {
			return fmt.Errorf("user with email %q already exists (%s): add them without --create-user", ref.email, joinUserIDs(existing))
		}

		created, err := qtx.UserCreate(bgCtx, db.UserCreateParams{Name: ref.email, Email: ref.email})
		if err != nil {
			return fmt.Errorf("failed to register user: %w", err)
		}
		userID = created.ID
	} else {
		user, err := lookupUser(bgCtx, qtx, ref)
		if err != nil {
			if errors.Is(err, errUserNotFound) && ref.email != "" {
				return fmt.Errorf("user %q not found: pass --create-user to register the address for someone who has not signed in yet", c.User)
			}
			if errors.Is(err, errUserNotFound) {
				return fmt.Errorf("user %q not found", c.User)
			}
			return fmt.Errorf("failed to look up user: %w", err)
		}
		userID = user.ID
	}

	membership, err := qtx.MembershipCreate(bgCtx, db.MembershipCreateParams{
		OrganizationID: orgID,
		UserID:         userID,
		Permission:     permission,
	})
	if err != nil {
		// A live membership already exists. If it is an invitation nobody has
		// answered yet, assigning them is the answer; otherwise there is
		// nothing to do. Any other error is a real one.
		pgErr, ok := errors.AsType[*pgconn.PgError](err)
		if !ok || pgErr.ConstraintName != dbconst.ConstraintOrganizationsUsersUqUser {
			return fmt.Errorf("failed to add organization member: %w", err)
		}
		// The failed insert aborted the transaction; the update needs a fresh one.
		if err := tx.Rollback(bgCtx); err != nil {
			return fmt.Errorf("failed to roll back transaction: %w", err)
		}
		accepted, err := ctx.Queries.MembershipAcceptPending(bgCtx, db.MembershipAcceptPendingParams{
			OrganizationID: orgID,
			UserID:         userID,
			Permission:     permission,
		})
		if err != nil {
			return fmt.Errorf("failed to accept pending invitation: %w", err)
		}
		if accepted == 0 {
			return fmt.Errorf("user %q is already a member of organization %q", c.User, c.Organization)
		}
		ctx.Logger.Info("accepted pending invitation on the user's behalf", "organization", c.Organization, "user_id", userID.String(), "permission", permission)
		return nil
	}

	if err := tx.Commit(bgCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if c.CreateUser {
		ctx.Logger.Info("registered user and added organization member; the account is linked when someone first signs in at this address",
			"organization", c.Organization, "user_id", userID.String(), "email", ref.email, "permission", permission)
	} else {
		ctx.Logger.Info("added organization member", "organization", c.Organization, "user_id", userID.String(), "permission", permission)
	}

	return outputMembershipCreate(ctx.Output, membership.ID)
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
		if errors.Is(err, errUserNotFound) {
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
