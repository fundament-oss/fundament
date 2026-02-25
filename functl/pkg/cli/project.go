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

	listReq := &organizationv1.ListProjectsRequest{ClusterId: c.Cluster}
	resp, err := apiClient.Projects().ListProjects(context.Background(), connect.NewRequest(listReq))
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	projects := resp.Msg.Projects

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
		if project.Created != nil {
			created = project.Created.AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			project.Id,
			project.Name,
			project.ClusterId,
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

	resp, err := apiClient.Projects().GetProject(context.Background(), connect.NewRequest(&organizationv1.GetProjectRequest{
		ProjectId: c.ProjectID,
	}))
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	project := resp.Msg.Project

	if ctx.Output == OutputJSON {
		return PrintJSON(project)
	}

	w := NewTableWriter()
	PrintKeyValue(w, "ID", project.Id)
	PrintKeyValue(w, "Name", project.Name)
	PrintKeyValue(w, "Cluster ID", project.ClusterId)

	if project.Created.IsValid() {
		PrintKeyValue(w, "Created", project.Created.AsTime().Format(TimeFormat))
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

	resp, err := apiClient.Projects().CreateProject(context.Background(), connect.NewRequest(&organizationv1.CreateProjectRequest{
		ClusterId: c.Cluster,
		Name:      c.Name,
	}))
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	projectID := resp.Msg.ProjectId

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": projectID,
			"name":       c.Name,
		})
	}

	fmt.Printf("Created project %s (ID: %s)\n", c.Name, projectID)
	return nil
}
