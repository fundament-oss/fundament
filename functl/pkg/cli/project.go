package cli

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// ProjectCmd contains project subcommands.
type ProjectCmd struct {
	List   ProjectListCmd   `cmd:"" help:"List all projects."`
	Get    ProjectGetCmd    `cmd:"" help:"Get project details."`
	Create ProjectCreateCmd `cmd:"" help:"Create a new project."`
	Update ProjectUpdateCmd `cmd:"" help:"Update a project."`
	Member ProjectMemberCmd `cmd:"" help:"Manage project members."`
}

// ProjectListCmd handles the project list command.
type ProjectListCmd struct {
	Cluster string `arg:"" help:"Cluster ID to list projects for."`
}

// Run executes the project list command.
func (c *ProjectListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}

	listReq := organizationv1.ListProjectsRequest_builder{ClusterId: c.Cluster}.Build()
	resp, err := apiClient.Projects().ListProjects(context.Background(), connect.NewRequest(listReq))
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	projects := resp.Msg.GetProjects()

	if ctx.Output == OutputJSON {
		return PrintJSON(projects)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "ID\tNAME\tALIAS\tCLUSTER ID\tCREATED")
	for _, project := range projects {
		created := ""
		if project.GetCreated() != nil {
			created = project.GetCreated().AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			project.GetId(),
			project.GetName(),
			project.GetAlias(),
			project.GetClusterId(),
			created,
		)
	}
	return w.Flush()
}

// ProjectGetCmd handles the project get command.
type ProjectGetCmd struct {
	ProjectID string `arg:"" help:"Project ID to get."`
}

// Run executes the project get command.
func (c *ProjectGetCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}

	resp, err := apiClient.Projects().GetProject(context.Background(), connect.NewRequest(organizationv1.GetProjectRequest_builder{
		ProjectId: c.ProjectID,
	}.Build()))
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	project := resp.Msg.GetProject()

	if ctx.Output == OutputJSON {
		return PrintJSON(project)
	}

	w := NewTableWriter()
	PrintKeyValue(w, "ID", project.GetId())
	PrintKeyValue(w, "Name", project.GetName())
	PrintKeyValue(w, "Alias", project.GetAlias())
	PrintKeyValue(w, "Cluster ID", project.GetClusterId())

	if project.GetCreated().IsValid() {
		PrintKeyValue(w, "Created", project.GetCreated().AsTime().Format(TimeFormat))
	}

	return w.Flush()
}

// ProjectCreateCmd handles the project create command.
type ProjectCreateCmd struct {
	Cluster string `arg:"" help:"Cluster ID to create the project in."`
	Name    string `arg:"" help:"Name of the project to create (immutable, DNS-1123 label)."`
	Alias   string `optional:"" short:"a" help:"Human-readable label for the project (defaults to name)."`
}

// Run executes the project create command.
func (c *ProjectCreateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}

	alias := c.Name
	if c.Alias != "" {
		alias = c.Alias
	}

	req := organizationv1.CreateProjectRequest_builder{
		ClusterId: c.Cluster,
		Name:      c.Name,
		Alias:     &alias,
	}.Build()

	resp, err := apiClient.Projects().CreateProject(context.Background(), connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	projectID := resp.Msg.GetProjectId()

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": projectID,
			"name":       c.Name,
			"alias":      alias,
		})
	}

	fmt.Printf("Created project %s (ID: %s)\n", alias, projectID)
	return nil
}

// ProjectUpdateCmd handles the project update command.
type ProjectUpdateCmd struct {
	ProjectID string `arg:"" help:"Project ID to update."`
	Alias     string `required:"" short:"a" help:"New alias for the project."`
}

// Run executes the project update command.
func (c *ProjectUpdateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}

	alias := c.Alias
	req := organizationv1.UpdateProjectRequest_builder{
		ProjectId: c.ProjectID,
		Alias:     &alias,
	}.Build()

	_, err = apiClient.Projects().UpdateProject(context.Background(), connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": c.ProjectID,
			"alias":      c.Alias,
		})
	}

	fmt.Printf("Updated project %s alias to %q\n", c.ProjectID, c.Alias)
	return nil
}
