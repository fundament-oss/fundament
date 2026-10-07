package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	Status string `help:"Version status to list, or 'all'." enum:"pending,draft,changes_requested,approved,rejected,withdrawn,all" default:"pending"`
}

// PluginApproveCmd approves a pending plugin version. Approving a plugin's
// first version makes the listing live in the public catalog: publication is
// nothing more than the published timestamp this stamps.
type PluginApproveCmd struct {
	Plugin       string `arg:"" help:"Plugin name or plugin ID." required:""`
	Version      string `arg:"" help:"Plugin version." required:""`
	Reviewer     string `help:"Reviewer recorded on the round: a DCIM user ID or email address." required:""`
	Organization string `help:"Owning organization name, when the plugin name is ambiguous."`
	Feedback     string `help:"Feedback for the publisher."`
}

// PluginRequestChangesCmd closes the round with feedback; the developer may
// resubmit, which opens a fresh round rather than reviving this one.
type PluginRequestChangesCmd struct {
	Plugin       string `arg:"" help:"Plugin name or plugin ID." required:""`
	Version      string `arg:"" help:"Plugin version." required:""`
	Reviewer     string `help:"Reviewer recorded on the round: a DCIM user ID or email address." required:""`
	Organization string `help:"Owning organization name, when the plugin name is ambiguous."`
	Feedback     string `help:"Feedback for the publisher." required:""`
}

// PluginRejectCmd refuses the version outright. Unlike request-changes this is
// terminal: a rejected version is resubmitted as a new version, not revived.
type PluginRejectCmd struct {
	Plugin       string `arg:"" help:"Plugin name or plugin ID." required:""`
	Version      string `arg:"" help:"Plugin version." required:""`
	Reviewer     string `help:"Reviewer recorded on the round: a DCIM user ID or email address." required:""`
	Organization string `help:"Owning organization name, when the plugin name is ambiguous."`
	Reason       string `help:"Rejection reason." enum:"incomplete_metadata,duplicate,security_concerns,naming_guidelines,out_of_scope,other" required:""`
	Feedback     string `help:"Feedback for the publisher."`
}

// Run executes the plugin list command.
func (c *PluginListCmd) Run(ctx *Context) error {
	status := c.Status
	if status == "all" {
		status = ""
	}

	ctx.Logger.Debug("listing plugin versions", "status", status)

	rows, err := ctx.Queries.PluginVersionList(context.Background(), db.PluginVersionListParams{Status: status})
	if err != nil {
		return fmt.Errorf("failed to list plugin versions: %w", err)
	}

	return outputPluginVersionList(ctx.Output, rows)
}

// Run executes the plugin approve command.
func (c *PluginApproveCmd) Run(ctx *Context) error {
	return decidePluginVersion(ctx, c.Plugin, c.Version, c.Organization, c.Reviewer, pluginDecision{
		status:   dbconst.PluginDefinitionStatus_Approved,
		feedback: c.Feedback,
	})
}

// Run executes the plugin request-changes command.
func (c *PluginRequestChangesCmd) Run(ctx *Context) error {
	return decidePluginVersion(ctx, c.Plugin, c.Version, c.Organization, c.Reviewer, pluginDecision{
		status:   dbconst.PluginDefinitionStatus_ChangesRequested,
		feedback: c.Feedback,
	})
}

// Run executes the plugin reject command.
func (c *PluginRejectCmd) Run(ctx *Context) error {
	return decidePluginVersion(ctx, c.Plugin, c.Version, c.Organization, c.Reviewer, pluginDecision{
		status:          dbconst.PluginDefinitionStatus_Rejected,
		rejectionReason: c.Reason,
		feedback:        c.Feedback,
	})
}

// pluginDecision is the reviewer's verdict: the status the version moves to,
// and what gets recorded on the round.
type pluginDecision struct {
	status          dbconst.PluginDefinitionStatus
	rejectionReason string
	feedback        string
}

// decidePluginVersion runs the shared half of every review verdict, mirroring
// marketplace-admin-api's decide: only a PENDING version with an open round
// can be decided, the version's status and the round's record move in one
// transaction, and both writes are guarded so a decision racing a withdrawal
// (or another reviewer) fails rather than half-landing.
func decidePluginVersion(ctx *Context, pluginRef, version, organization, reviewer string, d pluginDecision) error {
	bgCtx := context.Background()

	tx, err := ctx.DB.Pool.Begin(bgCtx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer rollback.Rollback(bgCtx, tx, ctx.Logger)

	qtx := ctx.Queries.WithTx(tx)

	pluginID, err := lookupPluginID(bgCtx, qtx, pluginRef, organization)
	if err != nil {
		return err
	}

	ver, err := qtx.PluginVersionGet(bgCtx, db.PluginVersionGetParams{PluginID: pluginID, PluginVersion: version})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("plugin %q has no version %q", pluginRef, version)
		}
		return fmt.Errorf("failed to look up plugin version: %w", err)
	}
	if ver.Status != dbconst.PluginDefinitionStatus_Pending {
		return fmt.Errorf("version %q of plugin %q is %s; only a pending version can be decided", version, pluginRef, ver.Status)
	}
	if !ver.HasOpenRound {
		return fmt.Errorf("version %q of plugin %q has no open submission round", version, pluginRef)
	}

	reviewerID, err := lookupReviewerID(bgCtx, qtx, reviewer)
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
		return fmt.Errorf("version %q of plugin %q is no longer pending", version, pluginRef)
	}

	rejectionReason := pgtype.Text{}
	if d.rejectionReason != "" {
		rejectionReason = pgtype.Text{String: d.rejectionReason, Valid: true}
	}
	closed, err := qtx.SubmissionDecide(bgCtx, db.SubmissionDecideParams{
		PluginDefinitionID: ver.ID,
		ReviewerUserID:     pgtype.UUID{Bytes: reviewerID, Valid: true},
		RejectionReason:    rejectionReason,
		Feedback:           d.feedback,
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

	ctx.Logger.Info("decided plugin version", "plugin", pluginRef, "version", version,
		"status", string(d.status), "reviewer", reviewerID.String())

	return nil
}

// lookupPluginID resolves a plugin named by ID or by name. Names are unique
// per organization, not globally, so a bare name matching several listings is
// refused naming the organizations; --organization narrows it down.
func lookupPluginID(ctx context.Context, queries *db.Queries, ref, organization string) (uuid.UUID, error) {
	if id, err := uuid.Parse(ref); err == nil {
		row, err := queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, fmt.Errorf("plugin %q not found", ref)
			}
			return uuid.Nil, fmt.Errorf("failed to look up plugin: %w", err)
		}
		if organization != "" && row.OrganizationName != organization {
			return uuid.Nil, fmt.Errorf("plugin %q is published by organization %q, not %q", ref, row.OrganizationName, organization)
		}
		return row.ID, nil
	}

	rows, err := queries.PluginFindByName(ctx, db.PluginFindByNameParams{Name: ref, OrganizationName: organization})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to look up plugin: %w", err)
	}
	switch len(rows) {
	case 0:
		if organization != "" {
			return uuid.Nil, fmt.Errorf("plugin %q not found in organization %q", ref, organization)
		}
		return uuid.Nil, fmt.Errorf("plugin %q not found", ref)
	case 1:
		return rows[0].ID, nil
	default:
		names := make([]string, len(rows))
		for i := range rows {
			names[i] = rows[i].OrganizationName
		}
		return uuid.Nil, fmt.Errorf("plugin %q is published by %d organizations (%s): pass --organization or the plugin ID",
			ref, len(rows), strings.Join(names, ", "))
	}
}

// lookupReviewerID resolves a reviewer to a DCIM staff user (dcim.users), the
// store marketplace-admin-api authenticates decisions against. Email carries
// no unique constraint there, so several matches is an error naming the rows;
// the user ID still works.
func lookupReviewerID(ctx context.Context, queries *db.Queries, reviewer string) (uuid.UUID, error) {
	ref, err := parseUserRef(reviewer)
	if err != nil {
		return uuid.Nil, err
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
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID.String()
		}
		return uuid.Nil, fmt.Errorf("%d reviewers match email %q (%s): pass the user ID instead",
			len(rows), ref.email, strings.Join(ids, ", "))
	}
}

// pluginVersionOutput is the JSON output structure for a plugin version.
type pluginVersionOutput struct {
	Organization string `json:"organization"`
	Plugin       string `json:"plugin"`
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
				Version:      row.PluginVersion,
				Status:       string(row.Status),
				Submitted:    formatNullableTime(row.Submitted),
				Published:    formatNullableTime(row.Published),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ORGANIZATION\tPLUGIN\tVERSION\tSTATUS\tSUBMITTED\tPUBLISHED")
		for i := range rows {
			row := &rows[i]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				row.OrganizationName,
				row.PluginName,
				row.PluginVersion,
				row.Status,
				formatNullableTime(row.Submitted),
				formatNullableTime(row.Published),
			)
		}
		return w.Flush()
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
