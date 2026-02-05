package cli

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

type NamespaceCmd struct {
	List   NamespaceListCmd   `cmd:"" help:"List namespaces."`
	Create NamespaceCreateCmd `cmd:"" help:"Create a namespace."`
	Delete NamespaceDeleteCmd `cmd:"" help:"Delete a namespace."`
}

type NamespaceListCmd struct {
	Cluster string `help:"Filter by cluster ID." short:"c" xor:"filter"`
	Project string `help:"Filter by project ID." short:"p" xor:"filter"`
}

func (c *NamespaceListCmd) Run(ctx *Context) error {
	if c.Cluster == "" && c.Project == "" {
		return errors.New("either --cluster or --project is required")
	}

	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	if c.Cluster != "" {
		resp, err := apiClient.Clusters().ListClusterNamespaces(context.Background(), connect.NewRequest(&organizationv1.ListClusterNamespacesRequest{
			ClusterId: c.Cluster,
		}))
		if err != nil {
			return fmt.Errorf("failed to list namespaces: %w", err)
		}

		namespaces := resp.Msg.Namespaces

		if ctx.Output == OutputJSON {
			return PrintJSON(namespaces)
		}

		if len(namespaces) == 0 {
			fmt.Println("No namespaces found")
			return nil
		}

		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tNAME\tPROJECT_ID\tCREATED")
		for _, ns := range namespaces {
			created := ""
			if ns.Created.IsValid() {
				created = ns.Created.AsTime().Format(TimeFormat)
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				ns.Id,
				ns.Name,
				ns.ProjectId,
				created,
			)
		}
		return w.Flush()
	}

	// List by project
	resp, err := apiClient.Projects().ListProjectNamespaces(context.Background(), connect.NewRequest(&organizationv1.ListProjectNamespacesRequest{
		ProjectId: c.Project,
	}))
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	namespaces := resp.Msg.Namespaces

	if ctx.Output == OutputJSON {
		return PrintJSON(namespaces)
	}

	if len(namespaces) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "ID\tNAME\tCLUSTER_ID\tCREATED")
	for _, ns := range namespaces {
		created := ""
		if ns.Created.IsValid() {
			created = ns.Created.AsTime().Format(TimeFormat)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			ns.Id,
			ns.Name,
			ns.ClusterId,
			created,
		)
	}
	return w.Flush()
}

// NamespaceCreateCmd handles the namespace create command.
type NamespaceCreateCmd struct {
	Name    string `arg:"" help:"Name for the namespace."`
	Cluster string `help:"Cluster ID." short:"c" required:""`
	Project string `help:"Project ID." short:"p" required:""`
}

// Run executes the namespace create command.
func (c *NamespaceCreateCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.Clusters().CreateNamespace(context.Background(), connect.NewRequest(&organizationv1.CreateNamespaceRequest{
		ProjectId: c.Project,
		ClusterId: c.Cluster,
		Name:      c.Name,
	}))
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	namespaceID := resp.Msg.NamespaceId

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"namespace_id": namespaceID,
			"name":         c.Name,
			"cluster_id":   c.Cluster,
			"project_id":   c.Project,
		})
	}

	fmt.Printf("Created namespace %s (ID: %s)\n", c.Name, namespaceID)
	return nil
}

// NamespaceDeleteCmd handles the namespace delete command.
type NamespaceDeleteCmd struct {
	NamespaceID string `arg:"" help:"Namespace ID to delete."`
}

// Run executes the namespace delete command.
func (c *NamespaceDeleteCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	_, err = apiClient.Clusters().DeleteNamespace(context.Background(), connect.NewRequest(&organizationv1.DeleteNamespaceRequest{
		NamespaceId: c.NamespaceID,
	}))
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	fmt.Printf("Namespace %s has been deleted\n", c.NamespaceID)
	return nil
}
