package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure ProjectDataSource satisfies various datasource interfaces.
var _ datasource.DataSource = &ProjectDataSource{}
var _ datasource.DataSourceWithConfigure = &ProjectDataSource{}

// ProjectDataSource defines the data source implementation.
type ProjectDataSource struct {
	client *FundamentClient
}

// NewProjectDataSource creates a new ProjectDataSource.
func NewProjectDataSource() datasource.DataSource {
	return &ProjectDataSource{}
}

// Metadata returns the data source type name.
func (d *ProjectDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

// Schema defines the schema for the data source.
func (d *ProjectDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a single project by name. Project names are unique per cluster, not per organization: " +
			"when two clusters have a project with this name, set cluster_name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the project.",
				Computed:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster this project belongs to.",
				Computed:    true,
			},
			"cluster_name": schema.StringAttribute{
				Description: "The name of the cluster the project belongs to. Optional: only needed when several clusters have a project with this name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the project to look up.",
				Required:    true,
			},
			"alias": schema.StringAttribute{
				Description: "The alias of the project.",
				Computed:    true,
			},
			"created": schema.StringAttribute{
				Description: "The timestamp when the project was created.",
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *ProjectDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*FundamentClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *FundamentClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

// Read refreshes the Terraform state with the latest data.
func (d *ProjectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ProjectModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The Fundament client was not configured. Please report this issue to the provider developers.",
		)
		return
	}

	tflog.Debug(ctx, "Reading project", map[string]any{
		"name": config.Name.ValueString(),
	})

	project, clusterName, err := d.findProject(ctx, config.Name.ValueString(), config.ClusterName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Project", err.Error())
		return
	}

	config.ID = types.StringValue(project.GetId())
	config.Name = types.StringValue(project.GetName())
	config.Alias = types.StringValue(project.GetAlias())
	config.ClusterID = types.StringValue(project.GetClusterId())
	config.ClusterName = types.StringValue(clusterName)
	config.Created = timestampValue(project.GetCreated())

	tflog.Debug(ctx, "Read project successfully", map[string]any{
		"id":   config.ID.ValueString(),
		"name": config.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// findProject looks the project up on every cluster, or on the named one.
// GetProjectByName does not take a cluster and returns any one of several
// projects with the name.
func (d *ProjectDataSource) findProject(ctx context.Context, name, clusterName string) (*organizationv1.Project, string, error) {
	clusters := map[string]string{} // ID to name
	if clusterName != "" {
		resp, err := d.client.ClusterService.GetClusterByName(ctx, organizationv1.GetClusterByNameRequest_builder{Name: clusterName}.Build())
		if err != nil {
			return nil, "", fmt.Errorf("cluster %q: %w", clusterName, err)
		}
		clusters[resp.GetCluster().GetId()] = resp.GetCluster().GetName()
	} else {
		resp, err := d.client.ClusterService.ListClusters(ctx, organizationv1.ListClustersRequest_builder{}.Build())
		if err != nil {
			return nil, "", fmt.Errorf("list clusters: %w", err)
		}
		for _, c := range resp.GetClusters() {
			clusters[c.GetId()] = c.GetName()
		}
	}

	var found []*organizationv1.Project
	for clusterID := range clusters {
		resp, err := d.client.ProjectService.ListProjects(ctx, organizationv1.ListProjectsRequest_builder{ClusterId: clusterID}.Build())
		if err != nil {
			return nil, "", fmt.Errorf("list projects of cluster %q: %w", clusters[clusterID], err)
		}
		for _, p := range resp.GetProjects() {
			if p.GetName() == name {
				found = append(found, p)
			}
		}
	}

	switch len(found) {
	case 0:
		if clusterName != "" {
			return nil, "", fmt.Errorf("cluster %q has no project %q", clusterName, name)
		}
		return nil, "", fmt.Errorf("no cluster has a project %q", name)
	case 1:
		return found[0], clusters[found[0].GetClusterId()], nil
	default:
		names := make([]string, 0, len(found))
		for _, p := range found {
			names = append(names, clusters[p.GetClusterId()])
		}
		slices.Sort(names)
		return nil, "", fmt.Errorf("clusters %s each have a project %q; set cluster_name", strings.Join(names, ", "), name)
	}
}
