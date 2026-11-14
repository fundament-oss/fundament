package provider

import (
	"context"
	"errors"
	"fmt"
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

	"github.com/fundament-oss/fundament/common/discovery"
)

// Ensure FundamentProvider satisfies various provider interfaces.
var _ provider.Provider = &FundamentProvider{}

// FundamentProvider defines the provider implementation.
type FundamentProvider struct {
	version string
}

// FundamentProviderModel describes the provider data model.
type FundamentProviderModel struct {
	URL             types.String `tfsdk:"url"`
	Endpoint        types.String `tfsdk:"endpoint"`
	ApiKey          types.String `tfsdk:"api_key"`
	AuthnEndpoint   types.String `tfsdk:"authn_endpoint"`
	OrganizationID  types.String `tfsdk:"organization_id"`
	KubeAPIProxyURL types.String `tfsdk:"kube_api_proxy_url"`

	MarketplaceInstallURL types.String `tfsdk:"marketplace_install_url"`
}

// FundamentEnvConfig describes the environment variable configuration.
type FundamentEnvConfig struct {
	URL             string `env:"FUNDAMENT_URL"`
	Endpoint        string `env:"FUNDAMENT_ENDPOINT"`
	ApiKey          string `env:"FUNDAMENT_API_KEY"`
	AuthnEndpoint   string `env:"FUNDAMENT_AUTHN_ENDPOINT"`
	OrganizationID  string `env:"FUNDAMENT_ORGANIZATION_ID"`
	KubeAPIProxyURL string `env:"FUNDAMENT_KUBE_API_PROXY_URL"`

	MarketplaceInstallURL string `env:"FUNDAMENT_MARKETPLACE_INSTALL_URL"`
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
		Description: "Manages the clusters, projects, namespaces and members of a Fundament organization.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "The root URL of the Fundament installation, for example https://fundament.example. The provider reads the URLs of the installation's APIs from its discovery document at /.well-known/fundament.json. Can also be set via the FUNDAMENT_URL environment variable. Required unless endpoint and authn_endpoint are both set (setting only endpoint is deprecated: authn_endpoint is then derived from it).",
				Optional:    true,
				Validators: []validator.String{
					httpURLValidator{},
				},
			},
			"endpoint": schema.StringAttribute{
				Description: "The URL of the Fundament organization API, for example https://organization.fundament.example. Can also be set via the FUNDAMENT_ENDPOINT environment variable. If not provided, discovered from url.",
				Optional:    true,
				Validators: []validator.String{
					httpURLValidator{},
				},
			},
			"api_key": schema.StringAttribute{
				Description: "The API key the provider authenticates with. Required: set it here or, to keep it out of the configuration, in the FUNDAMENT_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
				Validators: []validator.String{
					apiKeyValidator{},
				},
			},
			"authn_endpoint": schema.StringAttribute{
				Description: "The endpoint URL for the Fundament authentication API (for API key exchange). Can also be set via the FUNDAMENT_AUTHN_ENDPOINT environment variable. If not provided, discovered from url. Deprecated: without url it is derived from endpoint by replacing 'organization' with 'authn' in the host.",
				Optional:    true,
				Validators: []validator.String{
					httpURLValidator{},
				},
			},
			"organization_id": schema.StringAttribute{
				Description: "The ID of the organization to manage, shown in the console under General. Required: set it here or in the FUNDAMENT_ORGANIZATION_ID environment variable.",
				Optional:    true,
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"kube_api_proxy_url": schema.StringAttribute{
				Description: "The base URL of the kube-api-proxy service. Required for managing plugin installations. Can also be set via the FUNDAMENT_KUBE_API_PROXY_URL environment variable. If not provided, discovered from url.",
				Optional:    true,
				Validators: []validator.String{
					httpURLValidator{},
				},
			},
			"marketplace_install_url": schema.StringAttribute{
				Description: "The base URL of the marketplace install API (install.v1). Required to install a plugin without a pinned plugin_version. Can also be set via the FUNDAMENT_MARKETPLACE_INSTALL_URL environment variable. If not provided, discovered from url.",
				Optional:    true,
				Validators: []validator.String{
					httpURLValidator{},
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

	// Config takes precedence over environment variables.
	rootURL, err := resolveOptionalURL(config.URL.ValueString(), envConfig.URL, "FUNDAMENT_URL")
	if err != nil {
		resp.Diagnostics.AddError("Invalid URL", err.Error())
		return
	}

	kubeAPIProxyURL, err := resolveOptionalURL(config.KubeAPIProxyURL.ValueString(), envConfig.KubeAPIProxyURL, "FUNDAMENT_KUBE_API_PROXY_URL")
	if err != nil {
		resp.Diagnostics.AddError("Invalid URL", err.Error())
		return
	}

	marketplaceInstallURL, err := resolveOptionalURL(config.MarketplaceInstallURL.ValueString(), envConfig.MarketplaceInstallURL, "FUNDAMENT_MARKETPLACE_INSTALL_URL")
	if err != nil {
		resp.Diagnostics.AddError("Invalid URL", err.Error())
		return
	}

	urls := serviceURLs{
		organization:       config.Endpoint.ValueString(),
		authn:              config.AuthnEndpoint.ValueString(),
		kubeAPIProxy:       kubeAPIProxyURL,
		marketplaceInstall: marketplaceInstallURL,
	}
	if urls.organization == "" {
		urls.organization = envConfig.Endpoint
	}
	if urls.authn == "" {
		urls.authn = envConfig.AuthnEndpoint
	}

	err = urls.discover(ctx, rootURL)
	if err != nil {
		// Only the organization and authn APIs are needed to configure; the
		// others are optional, so an installation that has given those two
		// explicitly does not depend on its discovery document.
		if urls.organization == "" || urls.authn == "" {
			resp.Diagnostics.AddError("Endpoint Discovery Failed", err.Error())
			return
		}
		// An installation that serves no document yet is not worth a warning
		// on every run when everything required is set.
		if errors.Is(err, discovery.ErrNotFound) {
			tflog.Debug(ctx, "Installation serves no discovery document", map[string]any{"error": err.Error()})
		} else {
			resp.Diagnostics.AddWarning("Endpoint Discovery Failed", err.Error())
		}
	}

	if urls.authn == "" && urls.organization != "" && rootURL == "" {
		urls.authn = deriveAuthnEndpoint(urls.organization)
		resp.Diagnostics.AddWarning(
			"Deprecated: derived authn endpoint",
			fmt.Sprintf("Neither url nor authn_endpoint is set, so the authentication API was guessed as %s by replacing 'organization' with 'authn' in endpoint. "+
				"This guess is deprecated and will be removed: set url (FUNDAMENT_URL) to the installation's root URL, or set authn_endpoint (FUNDAMENT_AUTHN_ENDPOINT).", urls.authn),
		)
	}

	if urls.organization == "" || urls.authn == "" {
		resp.Diagnostics.AddError(
			"Missing Endpoint",
			"The provider cannot create the Fundament API client as it does not know the organization or authentication API. "+
				"Set url (FUNDAMENT_URL) to the installation's root URL, or set both endpoint (FUNDAMENT_ENDPOINT) and authn_endpoint (FUNDAMENT_AUTHN_ENDPOINT).",
		)
		return
	}
	endpoint, authnEndpoint := urls.organization, urls.authn

	apiKey := config.ApiKey.ValueString()
	if apiKey == "" {
		apiKey = envConfig.ApiKey
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

	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing Authentication",
			"The 'api_key' (FUNDAMENT_API_KEY) must be provided.",
		)
		return
	}

	var client *FundamentClient

	tflog.Debug(ctx, "Using API key authentication", map[string]any{
		"endpoint":       endpoint,
		"authn_endpoint": authnEndpoint,
	})

	tm := NewTokenManager(apiKey, authnEndpoint)

	// Validate API key by doing initial token exchange
	_, err = tm.GetToken(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Key Authentication Failed",
			"Failed to exchange API key for token: "+err.Error(),
		)
		return
	}

	client = NewFundamentClientWithTokenManager(endpoint, tm, organizationID, urls.kubeAPIProxy, urls.marketplaceInstall)

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
		NewPluginInstallationResource,
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

// deriveAuthnEndpoint guesses the authn endpoint from the organization endpoint
// by replacing 'organization' with 'authn' in the host.
//
// Deprecated: the guess is wrong for hosts such as organization-api.<domain>.
// It only remains for configurations from before url, until
// https://gitlab.com/digilab.overheid.nl/miscellaneous/issues/-/work_items/1480
func deriveAuthnEndpoint(organizationEndpoint string) string {
	u, err := url.Parse(organizationEndpoint)
	if err != nil {
		return strings.Replace(organizationEndpoint, "organization", "authn", 1)
	}

	u.Host = strings.Replace(u.Host, "organization", "authn", 1)
	return u.String()
}

// serviceURLs are the APIs the provider talks to. marketplaceInstall is the
// install.v1 API, which marketplace-catalog-api serves.
type serviceURLs struct {
	organization       string
	authn              string
	kubeAPIProxy       string
	marketplaceInstall string
}

// discover fills the URLs that are not set from the discovery document of the
// installation at rootURL. It makes no request when there is nothing to fill
// or no rootURL.
func (u *serviceURLs) discover(ctx context.Context, rootURL string) error {
	complete := u.organization != "" && u.authn != "" && u.kubeAPIProxy != "" && u.marketplaceInstall != ""
	if rootURL == "" || complete {
		return nil
	}

	doc, err := discovery.Fetch(ctx, nil, rootURL)
	if err != nil {
		return fmt.Errorf("discovering the endpoints of %s: %w", rootURL, err)
	}

	if u.organization == "" {
		u.organization = doc.OrganizationAPI
	}
	if u.authn == "" {
		u.authn = doc.AuthnAPI
	}
	if u.kubeAPIProxy == "" {
		u.kubeAPIProxy = doc.KubeAPIProxy
	}
	if u.marketplaceInstall == "" {
		u.marketplaceInstall = doc.MarketplaceCatalogAPI
	}
	return nil
}

// resolveOptionalURL returns the configured URL, else the one from envName. Only
// the env value is checked here: the schema validator covers the attribute.
func resolveOptionalURL(configValue, envValue, envName string) (string, error) {
	if configValue != "" || envValue == "" {
		return configValue, nil
	}
	if err := checkHTTPURL(envValue); err != nil {
		return "", fmt.Errorf("%s: %w", envName, err)
	}
	return envValue, nil
}
