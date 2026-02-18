package provider

import (
	"context"
	"net/url"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	Endpoint       types.String `tfsdk:"endpoint"`
	Token          types.String `tfsdk:"token"`
	ApiKey         types.String `tfsdk:"api_key"`
	AuthnEndpoint  types.String `tfsdk:"authn_endpoint"`
	OrganizationID types.String `tfsdk:"organization_id"`
}

// FundamentEnvConfig describes the environment variable configuration.
type FundamentEnvConfig struct {
	Endpoint       string `env:"FUNDAMENT_ENDPOINT"`
	Token          string `env:"FUNDAMENT_TOKEN"`
	ApiKey         string `env:"FUNDAMENT_API_KEY"`
	AuthnEndpoint  string `env:"FUNDAMENT_AUTHN_ENDPOINT"`
	OrganizationID string `env:"FUNDAMENT_ORGANIZATION_ID"`
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
				Validators: []validator.String{
					httpURLValidator{},
				},
			},
			"token": schema.StringAttribute{
				Description: "The JWT token for authenticating with the Fundament API. Can also be set via the FUNDAMENT_TOKEN environment variable. Mutually exclusive with api_key.",
				Optional:    true,
				Sensitive:   true,
				Validators: []validator.String{
					jwtValidator{},
				},
			},
			"api_key": schema.StringAttribute{
				Description: "API key for authenticating with the Fundament API. Can also be set via the FUNDAMENT_API_KEY environment variable. Mutually exclusive with token.",
				Optional:    true,
				Sensitive:   true,
				Validators: []validator.String{
					apiKeyValidator{},
				},
			},
			"authn_endpoint": schema.StringAttribute{
				Description: "The endpoint URL for the Fundament authentication API (for API key exchange). Can also be set via the FUNDAMENT_AUTHN_ENDPOINT environment variable. If not provided, derived from endpoint by replacing 'organization' with 'authn' in the subdomain.",
				Optional:    true,
				Validators: []validator.String{
					httpURLValidator{},
				},
			},
			"organization_id": schema.StringAttribute{
				Description: "The ID of the organization to operate on. Can also be set via the FUNDAMENT_ORGANIZATION_ID environment variable.",
				Required:    true,
				Validators: []validator.String{
					uuidValidator{},
				},
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

	// Get authentication credentials: config takes precedence over environment variable
	token := config.Token.ValueString()
	if token == "" {
		token = envConfig.Token
	}

	apiKey := config.ApiKey.ValueString()
	if apiKey == "" {
		apiKey = envConfig.ApiKey
	}

	authnEndpoint := config.AuthnEndpoint.ValueString()
	if authnEndpoint == "" {
		authnEndpoint = envConfig.AuthnEndpoint
	}

	// Get organization_id: config takes precedence over environment variable
	organizationID := config.OrganizationID.ValueString()
	if organizationID == "" {
		organizationID = envConfig.OrganizationID
	}

	if organizationID == "" {
		resp.Diagnostics.AddError(
			"Missing Organization ID",
			"The provider cannot create the Fundament API client as there is a missing or empty value for the organization ID. "+
				"Set the organization_id value in the configuration or use the FUNDAMENT_ORGANIZATION_ID environment variable.",
		)
		return
	}

	// Validate mutual exclusivity of token and api_key
	if token != "" && apiKey != "" {
		resp.Diagnostics.AddError(
			"Invalid Authentication Configuration",
			"Both 'token' and 'api_key' are provided. Please provide only one authentication method.",
		)
		return
	}

	if token == "" && apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing Authentication",
			"Either 'token' (FUNDAMENT_TOKEN) or 'api_key' (FUNDAMENT_API_KEY) must be provided.",
		)
		return
	}

	var client *FundamentClient

	if apiKey != "" {
		// API key authentication - derive authn_endpoint if not provided
		if authnEndpoint == "" {
			authnEndpoint = deriveAuthnEndpoint(endpoint)
		}

		tflog.Debug(ctx, "Using API key authentication", map[string]any{
			"endpoint":       endpoint,
			"authn_endpoint": authnEndpoint,
		})

		tm := NewTokenManager(apiKey, authnEndpoint)

		// Validate API key by doing initial token exchange
		_, err := tm.GetToken(ctx)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Key Authentication Failed",
				"Failed to exchange API key for token: "+err.Error(),
			)
			return
		}

		client = NewFundamentClientWithTokenManager(endpoint, tm, organizationID)
	} else {
		// Direct token authentication
		tflog.Debug(ctx, "Using token authentication", map[string]any{
			"endpoint": endpoint,
		})
		client = NewFundamentClient(endpoint, token, organizationID)
	}

	tflog.Info(ctx, "Fundament provider configured successfully")

	// Make the client available during DataSource and Resource type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

// Resources defines the resources implemented in the provider.
func (p *FundamentProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
		NewProjectResource,
		NewProjectMemberResource,
		NewNamespaceResource,
		NewOrganizationMemberResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *FundamentProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
		NewClustersDataSource,
		NewProjectDataSource,
		NewProjectsDataSource,
		NewProjectMembersDataSource,
		NewNamespaceDataSource,
		NewClusterNamespacesDataSource,
		NewProjectNamespacesDataSource,
		NewOrganizationMembersDataSource,
	}
}

// deriveAuthnEndpoint derives the authn endpoint from the organization endpoint
// by replacing 'organization' with 'authn' in the subdomain.
func deriveAuthnEndpoint(organizationEndpoint string) string {
	u, err := url.Parse(organizationEndpoint)
	if err != nil {
		// Fall back to simple string replacement if URL parsing fails
		return strings.Replace(organizationEndpoint, "organization", "authn", 1)
	}

	// Replace "organization" with "authn" in the host
	u.Host = strings.Replace(u.Host, "organization", "authn", 1)
	return u.String()
}
