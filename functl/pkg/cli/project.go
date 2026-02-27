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
	Member ProjectMemberCmd `cmd:"" help:"Manage project members."`
}

// ProjectListCmd handles the project list command.
type ProjectListCmd struct {
	Cluster string `arg:"" help:"Cluster ID to list projects for."`
}

// Run executes the project list command.
func (c *ProjectListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
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
	fmt.Fprintln(w, "ID\tNAME\tCLUSTER ID\tCREATED")
	for _, project := range projects {
		created := ""
		if project.GetCreated() != nil {
			created = project.GetCreated().AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			project.GetId(),
			project.GetName(),
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
	apiClient, err := NewClientFromConfig()
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
	PrintKeyValue(w, "Cluster ID", project.GetClusterId())

	if project.GetCreated().IsValid() {
		PrintKeyValue(w, "Created", project.GetCreated().AsTime().Format(TimeFormat))
	}

	return w.Flush()
}

// ProjectCreateCmd handles the project create command.
type ProjectCreateCmd struct {
	Cluster string `arg:"" help:"Cluster ID to create the project in."`
	Name    string `arg:"" help:"Name of the project to create."`
}

// Run executes the project create command.
func (c *ProjectCreateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.Projects().CreateProject(context.Background(), connect.NewRequest(organizationv1.CreateProjectRequest_builder{
		ClusterId: c.Cluster,
		Name:      c.Name,
	}.Build()))
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	projectID := resp.Msg.GetProjectId()

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": projectID,
			"name":       c.Name,
		})
	}

	fmt.Printf("Created project %s (ID: %s)\n", c.Name, projectID)
	return nil
}
