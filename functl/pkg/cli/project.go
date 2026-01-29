package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/fundament-oss/fundament/functl/pkg/db/gen"
)

type ProjectCmd struct {
	Create ProjectCreateCmd `cmd:"" help:"Create a new project."`
	Get    ProjectGetCmd    `cmd:"" help:"Get project details."`
	List   ProjectListCmd   `cmd:"" help:"List projects in an organization."`
	Update ProjectUpdateCmd `cmd:"" help:"Update a project."`
	Delete ProjectDeleteCmd `cmd:"" help:"Delete a project."`
}

type ProjectCreateCmd struct {
	Identifier string `arg:"" help:"Project identifier: <organization>/<project>." required:""`
}

type ProjectGetCmd struct {
	Identifier string `arg:"" help:"Project identifier: <organization>/<project>." required:""`
}

type ProjectListCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

type ProjectUpdateCmd struct {
	Identifier string `arg:"" help:"Project identifier: <organization>/<project>." required:""`
	Name       string `help:"New project name." required:""`
}

type ProjectDeleteCmd struct {
	Identifier string `arg:"" help:"Project identifier: <organization>/<project>." required:""`
}

// parseProjectIdentifier splits "<organization>/<project>" into its parts.
func parseProjectIdentifier(identifier string) (organization, project string, err error) {
	parts := strings.SplitN(identifier, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid project identifier '%s': expected format <organization>/<project>", identifier)
	}
	return parts[0], parts[1], nil
}

// Run executes the project create command.
func (c *ProjectCreateCmd) Run(ctx *Context) error {
	org, project, err := parseProjectIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("creating project", "organization", org, "project", project)

	p, err := ctx.Queries.ProjectCreate(context.Background(), db.ProjectCreateParams{
		OrganizationName: org,
		Name:             project,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization '%s' not found", org)
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("project '%s' already exists in organization '%s'", project, org)
		}
		return fmt.Errorf("failed to create project: %w", err)
	}

	ctx.Logger.Debug("project created", "id", p.ID.String())

	return outputProjectCreate(ctx.Output, p)
}

// Run executes the project get command.
func (c *ProjectGetCmd) Run(ctx *Context) error {
	org, project, err := parseProjectIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("getting project", "organization", org, "project", project)

	p, err := ctx.Queries.ProjectGet(context.Background(), db.ProjectGetParams{
		OrganizationName: org,
		ProjectName:      project,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project '%s' not found", c.Identifier)
		}
		return fmt.Errorf("failed to get project: %w", err)
	}

	ctx.Logger.Debug("project retrieved", "id", p.ID.String())

	return outputProjectGet(ctx.Output, org, p)
}

// Run executes the project list command.
func (c *ProjectListCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing projects", "organization", c.Organization)

	projects, err := ctx.Queries.ProjectList(context.Background(), db.ProjectListParams{
		OrganizationName: c.Organization,
	})
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	ctx.Logger.Debug("projects listed", "count", len(projects))

	return outputProjectList(ctx.Output, c.Organization, projects)
}

// Run executes the project update command.
func (c *ProjectUpdateCmd) Run(ctx *Context) error {
	org, project, err := parseProjectIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("updating project", "organization", org, "project", project, "new_name", c.Name)

	p, err := ctx.Queries.ProjectUpdate(context.Background(), db.ProjectUpdateParams{
		OrganizationName: org,
		ProjectName:      project,
		NewName:          c.Name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project '%s' not found", c.Identifier)
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("project '%s' already exists in organization '%s'", c.Name, org)
		}
		return fmt.Errorf("failed to update project: %w", err)
	}

	ctx.Logger.Debug("project updated", "id", p.ID.String())

	return outputProjectUpdate(ctx.Output, p)
}

// Run executes the project delete command.
func (c *ProjectDeleteCmd) Run(ctx *Context) error {
	org, project, err := parseProjectIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("deleting project", "organization", org, "project", project)

	rowsAffected, err := ctx.Queries.ProjectDelete(context.Background(), db.ProjectDeleteParams{
		OrganizationName: org,
		ProjectName:      project,
	})
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("project '%s' not found", c.Identifier)
	}

	ctx.Logger.Info("deleted project", "identifier", c.Identifier)

	return nil
}

// projectCreateOutput is the JSON output structure for project create.
type projectCreateOutput struct {
	ID string `json:"id"`
}

// projectOutput is the JSON output structure for a project in list.
type projectOutput struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Created    string `json:"created"`
}

// projectDetailOutput is the JSON output structure for project get.
type projectDetailOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

func outputProjectCreate(format OutputFormat, p db.ProjectCreateRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(projectCreateOutput{
			ID: p.ID.String(),
		})
	case OutputTable:
		fmt.Println(p.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputProjectGet(format OutputFormat, organization string, p db.ProjectGetRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(projectDetailOutput{
			ID:      p.ID.String(),
			Name:    p.Name,
			Created: p.Created.Time.Format(TimeFormat),
		})
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintf(w, "ID:\t%s\n", p.ID.String())
		fmt.Fprintf(w, "Name:\t%s\n", p.Name)
		fmt.Fprintf(w, "Organization:\t%s\n", organization)
		fmt.Fprintf(w, "Created:\t%s\n", p.Created.Time.Format(TimeFormat))
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputProjectList(format OutputFormat, organization string, projects []db.ProjectListRow) error {
	switch format {
	case OutputJSON:
		output := make([]projectOutput, len(projects))
		for i, p := range projects {
			output[i] = projectOutput{
				ID:         p.ID.String(),
				Identifier: organization + "/" + p.Name,
				Created:    p.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tIDENTIFIER\tCREATED")
		for _, p := range projects {
			fmt.Fprintf(w, "%s\t%s/%s\t%s\n",
				p.ID.String(),
				organization,
				p.Name,
				p.Created.Time.Format(TimeFormat),
			)
		}
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputProjectUpdate(format OutputFormat, p db.ProjectUpdateRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(projectCreateOutput{
			ID: p.ID.String(),
		})
	case OutputTable:
		fmt.Println(p.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
