package provider

import (
	"context"

	"github.com/caarlos0/env/v11"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure FundamentProvider satisfies various provider interfaces.
var _ provider.Provider = &FundamentProvider{}

// FundamentProvider defines the provider implementation.
type FundamentProvider struct {
	version string
}

// FundamentProviderModel describes the provider data model.
type FundamentProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

// FundamentEnvConfig describes the environment variable configuration.
type FundamentEnvConfig struct {
	Endpoint string `env:"FUNDAMENT_ENDPOINT"`
	Token    string `env:"FUNDAMENT_TOKEN"`
}

// New returns a function that creates a new FundamentProvider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FundamentProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *FundamentProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fundament"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *FundamentProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Fundament organization API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "The endpoint URL for the Fundament organization API. Can also be set via the FUNDAMENT_ENDPOINT environment variable. Example: https://api.fundament.example.com",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "The JWT token for authenticating with the Fundament API. Can also be set via the FUNDAMENT_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

// Configure prepares a Fundament API client for data sources and resources.
func (p *FundamentProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config FundamentProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse environment variables
	envConfig, err := env.ParseAs[FundamentEnvConfig]()
	if err != nil {
		resp.Diagnostics.AddError(
			"Environment Variable Error",
			"Failed to parse environment variables: "+err.Error(),
		)
		return
	}

	// Get endpoint: config takes precedence over environment variable
	endpoint := config.Endpoint.ValueString()
	if endpoint == "" {
		endpoint = envConfig.Endpoint
	}

	if endpoint == "" {
		resp.Diagnostics.AddError(
			"Missing Endpoint",
			"The provider cannot create the Fundament API client as there is a missing or empty value for the Fundament API endpoint. "+
				"Set the endpoint value in the configuration or use the FUNDAMENT_ENDPOINT environment variable.",
		)
		return
	}

	// Get token: config takes precedence over environment variable
	token := config.Token.ValueString()
	if token == "" {
		token = envConfig.Token
	}

	if token == "" {
		resp.Diagnostics.AddError(
			"Missing Token",
			"The provider cannot create the Fundament API client as there is a missing or empty value for the Fundament API token. "+
				"Set the token value in the configuration or use the FUNDAMENT_TOKEN environment variable.",
		)
		return
	}

	// Create the Fundament client
	tflog.Debug(ctx, "Creating Fundament client", map[string]any{
		"endpoint": endpoint,
	})
	client := NewFundamentClient(endpoint, token)

	tflog.Info(ctx, "Fundament provider configured successfully")

	// Make the client available during DataSource and Resource type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

// Resources defines the resources implemented in the provider.
func (p *FundamentProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *FundamentProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
		NewClustersDataSource,
	}
}
