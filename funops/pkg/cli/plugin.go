package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/funops/pkg/db/gen"
)

// PluginCmd groups plugin review commands: the operator path to the same
// transitions marketplace-admin-api's ReviewService makes (FUN-20).
type PluginCmd struct {
	List           PluginListCmd           `cmd:"" help:"List plugin versions, the pending review queue by default."`
	Approve        PluginApproveCmd        `cmd:"" help:"Approve a pending plugin version, publishing it to the catalog."`
	RequestChanges PluginRequestChangesCmd `cmd:"" name:"request-changes" help:"Send a pending plugin version back to its publisher with feedback."`
	Reject         PluginRejectCmd         `cmd:"" help:"Reject a pending plugin version outright."`
}

// PluginListCmd lists plugin versions.
type PluginListCmd struct {
	Status string `help:"Version status to list: ${enum}." enum:"pending,draft,changes_requested,approved,rejected,withdrawn,all" default:"pending"`
}

// PluginApproveCmd approves a pending plugin version. Approving a plugin's
// first version makes the listing live in the public catalog: publication is
// nothing more than the published timestamp this stamps.
type PluginApproveCmd struct {
	Plugin       string `arg:"" help:"Plugin ID, or plugin name with --organization." required:""`
	Version      string `arg:"" help:"Plugin version." required:""`
	Reviewer     string `help:"Reviewer recorded on the round: a DCIM user ID or email address." required:""`
	Organization string `help:"Owning organization name; required when the plugin is addressed by name."`
	Feedback     string `help:"Feedback for the publisher."`
}

// PluginRequestChangesCmd closes the round with feedback; the developer may
// resubmit, which opens a fresh round rather than reviving this one.
type PluginRequestChangesCmd struct {
	Plugin       string `arg:"" help:"Plugin ID, or plugin name with --organization." required:""`
	Version      string `arg:"" help:"Plugin version." required:""`
	Reviewer     string `help:"Reviewer recorded on the round: a DCIM user ID or email address." required:""`
	Organization string `help:"Owning organization name; required when the plugin is addressed by name."`
	Feedback     string `help:"Feedback for the publisher." required:""`
}

// PluginRejectCmd refuses the version outright. Unlike request-changes this is
// terminal: a rejected version is resubmitted as a new version, not revived.
type PluginRejectCmd struct {
	Plugin       string `arg:"" help:"Plugin ID, or plugin name with --organization." required:""`
	Version      string `arg:"" help:"Plugin version." required:""`
	Reviewer     string `help:"Reviewer recorded on the round: a DCIM user ID or email address." required:""`
	Organization string `help:"Owning organization name; required when the plugin is addressed by name."`
	Reason       string `help:"Rejection reason: ${enum}." enum:"incomplete_metadata,duplicate,security_concerns,naming_guidelines,out_of_scope,other" required:""`
	Feedback     string `help:"Feedback for the publisher."`
}

// Run executes the plugin list command.
func (c *PluginListCmd) Run(ctx *Context) error {
	// 'all' means no status filter, which the query takes as NULL.
	status := pgtype.Text{String: c.Status, Valid: c.Status != "all"}

	ctx.Logger.Debug("listing plugin versions", "status", c.Status)

	rows, err := ctx.Queries.PluginVersionList(context.Background(), db.PluginVersionListParams{Status: status})
	if err != nil {
		return fmt.Errorf("failed to list plugin versions: %w", err)
	}

	return outputPluginVersionList(ctx.Output, rows)
}

// Run executes the plugin approve command.
func (c *PluginApproveCmd) Run(ctx *Context) error {
	return decidePluginVersion(ctx, &pluginDecision{
		plugin:       c.Plugin,
		version:      c.Version,
		organization: c.Organization,
		reviewer:     c.Reviewer,
		status:       dbconst.PluginDefinitionStatus_Approved,
		feedback:     c.Feedback,
	})
}

// Run executes the plugin request-changes command.
func (c *PluginRequestChangesCmd) Run(ctx *Context) error {
	// Kong's required only checks the flag was passed; asking for changes
	// without saying which is a dead end, as the admin API's contract puts it.
	if strings.TrimSpace(c.Feedback) == "" {
		return errors.New("feedback is required: say what should change")
	}
	return decidePluginVersion(ctx, &pluginDecision{
		plugin:       c.Plugin,
		version:      c.Version,
		organization: c.Organization,
		reviewer:     c.Reviewer,
		status:       dbconst.PluginDefinitionStatus_ChangesRequested,
		feedback:     c.Feedback,
	})
}

// Run executes the plugin reject command.
func (c *PluginRejectCmd) Run(ctx *Context) error {
	return decidePluginVersion(ctx, &pluginDecision{
		plugin:          c.Plugin,
		version:         c.Version,
		organization:    c.Organization,
		reviewer:        c.Reviewer,
		status:          dbconst.PluginDefinitionStatus_Rejected,
		rejectionReason: c.Reason,
		feedback:        c.Feedback,
	})
}

// maxFeedbackLen is the admin API's cap on decision feedback (review.proto);
// the direct-DB path keeps to the same contract.
const maxFeedbackLen = 4000

// pluginDecision is one review verdict: which version it targets, the status
// the version moves to, and what gets recorded on the round.
type pluginDecision struct {
	plugin          string
	version         string
	organization    string
	reviewer        string
	status          dbconst.PluginDefinitionStatus
	rejectionReason string
	feedback        string
}

// decidePluginVersion runs the shared half of every review verdict, mirroring
// marketplace-admin-api's decide: only a PENDING version with an open round
// can be decided, the version's status and the round's record move in one
// transaction, and both writes are guarded so a decision racing a withdrawal
// (or another reviewer) fails rather than half-landing.
func decidePluginVersion(ctx *Context, d *pluginDecision) error {
	if utf8.RuneCountInString(d.feedback) > maxFeedbackLen {
		return fmt.Errorf("feedback must be at most %d characters", maxFeedbackLen)
	}

	bgCtx := context.Background()

	tx, err := ctx.DB.Pool.Begin(bgCtx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer rollback.Rollback(bgCtx, tx, ctx.Logger)

	qtx := ctx.Queries.WithTx(tx)

	plug, err := lookupPlugin(bgCtx, qtx, d.plugin, d.organization)
	if err != nil {
		return err
	}

	ver, err := qtx.PluginVersionGet(bgCtx, db.PluginVersionGetParams{PluginID: plug.id, PluginVersion: d.version})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("plugin %q has no version %q", d.plugin, d.version)
		}
		return fmt.Errorf("failed to look up plugin version: %w", err)
	}
	if ver.Status != dbconst.PluginDefinitionStatus_Pending {
		return fmt.Errorf("version %q of plugin %q is %s; only a pending version can be decided", d.version, d.plugin, ver.Status)
	}
	if !ver.OpenSubmissionID.Valid {
		return fmt.Errorf("version %q of plugin %q has no open submission round", d.version, d.plugin)
	}

	reviewerID, err := lookupReviewerID(bgCtx, qtx, d.reviewer)
	if err != nil {
		return err
	}

	var updated int64
	if d.status == dbconst.PluginDefinitionStatus_Approved {
		updated, err = qtx.PluginVersionApprove(bgCtx, db.PluginVersionApproveParams{ID: ver.ID})
	} else {
		updated, err = qtx.PluginVersionSetStatus(bgCtx, db.PluginVersionSetStatusParams{ID: ver.ID, Status: string(d.status)})
	}
	if err != nil {
		return fmt.Errorf("failed to update version status: %w", err)
	}
	if updated == 0 {
		return fmt.Errorf("version %q of plugin %q is no longer pending", d.version, d.plugin)
	}

	rejectionReason := pgtype.Text{}
	if d.rejectionReason != "" {
		rejectionReason = pgtype.Text{String: d.rejectionReason, Valid: true}
	}
	closed, err := qtx.SubmissionDecide(bgCtx, db.SubmissionDecideParams{
		ID:              uuid.UUID(ver.OpenSubmissionID.Bytes),
		ReviewerUserID:  pgtype.UUID{Bytes: reviewerID, Valid: true},
		RejectionReason: rejectionReason,
		Feedback:        d.feedback,
	})
	if err != nil {
		return fmt.Errorf("failed to record decision: %w", err)
	}
	if closed == 0 {
		return errors.New("the submission round closed underneath this decision; nothing was recorded")
	}

	if err := tx.Commit(bgCtx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	ctx.Logger.Info("decided plugin version", "organization", plug.organization, "plugin", plug.name,
		"version", d.version, "status", string(d.status), "reviewer", reviewerID.String())

	return nil
}

// resolvedPlugin is a plugin ref resolved to a row: the ID the queries key on
// plus the name and organization the operator-facing output names it by.
type resolvedPlugin struct {
	id           uuid.UUID
	name         string
	organization string
}

// lookupPlugin resolves a plugin addressed by ID, or by name qualified with
// --organization. The two forms never mix: without an organization the ref
// must be a plugin ID, and with one it is always a name — even a name that
// parses as a UUID (plugins_ck_name allows an all-hex one).
func lookupPlugin(ctx context.Context, queries *db.Queries, ref, organization string) (resolvedPlugin, error) {
	if organization == "" {
		id, err := uuid.Parse(ref)
		if err != nil {
			return resolvedPlugin{}, fmt.Errorf("plugin %q is not a plugin ID: address a plugin by ID, or by name with --organization", ref)
		}

		row, err := queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return resolvedPlugin{}, fmt.Errorf("plugin %q not found", ref)
			}
			return resolvedPlugin{}, fmt.Errorf("failed to look up plugin: %w", err)
		}

		return resolvedPlugin{id: row.ID, name: row.Name, organization: row.OrganizationName}, nil
	}

	row, err := queries.PluginGetByName(ctx, db.PluginGetByNameParams{Name: ref, OrganizationName: organization})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, idErr := uuid.Parse(ref); idErr == nil {
				return resolvedPlugin{}, fmt.Errorf("plugin %q not found in organization %q: a plugin ID needs no --organization", ref, organization)
			}
			return resolvedPlugin{}, fmt.Errorf("plugin %q not found in organization %q", ref, organization)
		}
		return resolvedPlugin{}, fmt.Errorf("failed to look up plugin: %w", err)
	}
	return resolvedPlugin{id: row.ID, name: row.Name, organization: row.OrganizationName}, nil
}

// lookupReviewerID resolves a reviewer to a DCIM staff user (dcim.users), the
// store marketplace-admin-api authenticates decisions against. Email carries
// no unique constraint there, so several matches is an error naming the rows;
// the user ID still works.
func lookupReviewerID(ctx context.Context, queries *db.Queries, reviewer string) (uuid.UUID, error) {
	ref, err := parseUserRef(reviewer)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid reviewer %q: expected a user ID or an email address", reviewer)
	}

	if ref.email == "" {
		row, err := queries.ReviewerGetByID(ctx, db.ReviewerGetByIDParams{ID: ref.id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, fmt.Errorf("reviewer %q not found", reviewer)
			}
			return uuid.Nil, fmt.Errorf("failed to look up reviewer: %w", err)
		}
		return row.ID, nil
	}

	rows, err := queries.ReviewerFindByEmail(ctx, db.ReviewerFindByEmailParams{Email: ref.email})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to look up reviewer: %w", err)
	}
	switch len(rows) {
	case 0:
		return uuid.Nil, fmt.Errorf("reviewer %q not found", reviewer)
	case 1:
		return rows[0].ID, nil
	default:
		// Named so the operator can tell the rows apart; bare UUIDs would not.
		matches := make([]string, len(rows))
		for i := range rows {
			matches[i] = fmt.Sprintf("%s %s", rows[i].Name, rows[i].ID.String())
		}
		return uuid.Nil, fmt.Errorf("%d reviewers match email %q (%s): pass the user ID instead",
			len(rows), ref.email, strings.Join(matches, ", "))
	}
}

// pluginVersionOutput is the JSON output structure for a plugin version. The
// plugin ID is listed because it addresses a plugin in the decision commands
// without naming its organization.
type pluginVersionOutput struct {
	Organization string `json:"organization"`
	Plugin       string `json:"plugin"`
	PluginID     string `json:"plugin_id"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	Submitted    string `json:"submitted,omitempty"`
	Published    string `json:"published,omitempty"`
}

func outputPluginVersionList(format OutputFormat, rows []db.PluginVersionListRow) error {
	switch format {
	case OutputJSON:
		output := make([]pluginVersionOutput, len(rows))
		for i := range rows {
			row := &rows[i]
			output[i] = pluginVersionOutput{
				Organization: row.OrganizationName,
				Plugin:       row.PluginName,
				PluginID:     row.PluginID.String(),
				Version:      row.PluginVersion,
				Status:       string(row.Status),
				Submitted:    formatNullableTime(row.Submitted),
				Published:    formatNullableTime(row.Published),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		if _, err := fmt.Fprintln(w, "ORGANIZATION\tPLUGIN\tPLUGIN_ID\tVERSION\tSTATUS\tSUBMITTED\tPUBLISHED"); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		for i := range rows {
			row := &rows[i]
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				row.OrganizationName,
				row.PluginName,
				row.PluginID.String(),
				row.PluginVersion,
				row.Status,
				formatNullableTime(row.Submitted),
				formatNullableTime(row.Published),
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

// formatNullableTime renders a timestamp that may be unset; a blank keeps the
// column quiet.
func formatNullableTime(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(TimeFormat)
}
