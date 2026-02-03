package cli

import (
	"context"
	"fmt"
)

// ProjectCmd contains project subcommands.
type ProjectCmd struct {
	List   ProjectListCmd   `cmd:"" help:"List all projects."`
	Get    ProjectGetCmd    `cmd:"" help:"Get project details."`
	Create ProjectCreateCmd `cmd:"" help:"Create a new project."`
}

// ProjectListCmd handles the project list command.
type ProjectListCmd struct{}

// Run executes the project list command.
func (c *ProjectListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	projects, err := apiClient.ListProjects(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(projects)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "ID\tNAME\tCREATED")
	for _, project := range projects {
		created := ""
		if project.CreatedAt != nil {
			created = project.CreatedAt.AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			project.Id,
			project.Name,
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

	project, err := apiClient.GetProject(context.Background(), c.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(project)
	}

	w := NewTableWriter()
	PrintKeyValue(w, "ID", project.Id)
	PrintKeyValue(w, "Name", project.Name)
	if project.CreatedAt != nil {
		PrintKeyValue(w, "Created", project.CreatedAt.AsTime().Format(TimeFormat))
	}
	return w.Flush()
}

// ProjectCreateCmd handles the project create command.
type ProjectCreateCmd struct {
	Name string `arg:"" help:"Name of the project to create."`
}

// Run executes the project create command.
func (c *ProjectCreateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	projectID, err := apiClient.CreateProject(context.Background(), c.Name)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": projectID,
			"name":       c.Name,
		})
	}

	fmt.Printf("Created project %s (ID: %s)\n", c.Name, projectID)
	return nil
}
