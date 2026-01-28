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

// NamespaceCmd groups namespace-related commands.
type NamespaceCmd struct {
	Create NamespaceCreateCmd `cmd:"" help:"Create a new namespace."`
	List   NamespaceListCmd   `cmd:"" help:"List namespaces in an organization."`
	Delete NamespaceDeleteCmd `cmd:"" help:"Delete a namespace."`
}

// NamespaceCreateCmd creates a new namespace.
type NamespaceCreateCmd struct {
	Identifier string `arg:"" help:"Namespace identifier: <organization>/<project>/<namespace>." required:""`
	Cluster    string `help:"Cluster name." required:""`
}

// NamespaceListCmd lists namespaces in an organization.
type NamespaceListCmd struct {
	Organization string `arg:"" help:"Organization name." required:""`
}

// NamespaceDeleteCmd deletes a namespace.
type NamespaceDeleteCmd struct {
	Identifier string `arg:"" help:"Namespace identifier: <organization>/<project>/<namespace>." required:""`
}

// parseNamespaceIdentifier splits "<organization>/<project>/<namespace>" into its parts.
func parseNamespaceIdentifier(identifier string) (organization, project, namespace string, err error) {
	parts := strings.SplitN(identifier, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid namespace identifier '%s': expected format <organization>/<project>/<namespace>", identifier)
	}
	return parts[0], parts[1], parts[2], nil
}

// Run executes the namespace create command.
func (c *NamespaceCreateCmd) Run(ctx *Context) error {
	org, project, namespace, err := parseNamespaceIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("creating namespace", "organization", org, "project", project, "namespace", namespace, "cluster", c.Cluster)

	ns, err := ctx.Queries.NamespaceCreate(context.Background(), db.NamespaceCreateParams{
		OrganizationName: org,
		ProjectName:      project,
		Name:             namespace,
		ClusterName:      c.Cluster,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project '%s/%s' or cluster '%s' not found", org, project, c.Cluster)
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			return fmt.Errorf("namespace '%s' already exists in project '%s/%s'", namespace, org, project)
		}
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	ctx.Logger.Debug("namespace created", "id", ns.ID.String())

	return outputNamespaceCreate(ctx.Output, ns)
}

// Run executes the namespace list command.
func (c *NamespaceListCmd) Run(ctx *Context) error {
	ctx.Logger.Debug("listing namespaces", "organization", c.Organization)

	namespaces, err := ctx.Queries.NamespaceList(context.Background(), db.NamespaceListParams{
		OrganizationName: c.Organization,
	})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	ctx.Logger.Debug("namespaces listed", "count", len(namespaces))

	return outputNamespaceList(ctx.Output, c.Organization, namespaces)
}

// Run executes the namespace delete command.
func (c *NamespaceDeleteCmd) Run(ctx *Context) error {
	org, project, namespace, err := parseNamespaceIdentifier(c.Identifier)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("deleting namespace", "organization", org, "project", project, "namespace", namespace)

	rowsAffected, err := ctx.Queries.NamespaceDelete(context.Background(), db.NamespaceDeleteParams{
		OrganizationName: org,
		ProjectName:      project,
		NamespaceName:    namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("namespace '%s' not found", c.Identifier)
	}

	ctx.Logger.Info("deleted namespace", "identifier", c.Identifier)

	return nil
}

// namespaceCreateOutput is the JSON output structure for namespace create.
type namespaceCreateOutput struct {
	ID string `json:"id"`
}

// namespaceOutput is the JSON output structure for a namespace.
type namespaceOutput struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Cluster    string `json:"cluster"`
	Created    string `json:"created"`
}

func outputNamespaceCreate(format OutputFormat, ns db.NamespaceCreateRow) error {
	switch format {
	case OutputJSON:
		return PrintJSON(namespaceCreateOutput{
			ID: ns.ID.String(),
		})
	case OutputTable:
		fmt.Println(ns.ID.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}

func outputNamespaceList(format OutputFormat, organization string, namespaces []db.NamespaceListRow) error {
	switch format {
	case OutputJSON:
		output := make([]namespaceOutput, len(namespaces))
		for i, ns := range namespaces {
			output[i] = namespaceOutput{
				ID:         ns.ID.String(),
				Identifier: organization + "/" + ns.ProjectName + "/" + ns.Name,
				Cluster:    ns.ClusterName,
				Created:    ns.Created.Time.Format(TimeFormat),
			}
		}
		return PrintJSON(output)
	case OutputTable:
		w := NewTableWriter()
		fmt.Fprintln(w, "ID\tIDENTIFIER\tCLUSTER\tCREATED")
		for _, ns := range namespaces {
			fmt.Fprintf(w, "%s\t%s/%s/%s\t%s\t%s\n",
				ns.ID.String(),
				organization,
				ns.ProjectName,
				ns.Name,
				ns.ClusterName,
				ns.Created.Time.Format(TimeFormat),
			)
		}
		return w.Flush()
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
