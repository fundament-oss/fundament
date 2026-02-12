package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"fundament": providerserver.NewProtocol6WithError(New("test")()),
}

func TestDeriveAuthnEndpoint(t *testing.T) {
	tests := []struct {
		name                 string
		organizationEndpoint string
		expected             string
	}{
		{
			name:                 "replaces organization subdomain with authn",
			organizationEndpoint: "http://organization.fundament.localhost:8080",
			expected:             "http://authn.fundament.localhost:8080",
		},
		{
			name:                 "handles https scheme",
			organizationEndpoint: "https://organization.example.com",
			expected:             "https://authn.example.com",
		},
		{
			name:                 "handles endpoint with path",
			organizationEndpoint: "http://organization.example.com/api/v1",
			expected:             "http://authn.example.com/api/v1",
		},
		{
			name:                 "handles endpoint with query string",
			organizationEndpoint: "http://organization.example.com?foo=bar",
			expected:             "http://authn.example.com?foo=bar",
		},
		{
			name:                 "no change when organization not in host",
			organizationEndpoint: "http://api.example.com",
			expected:             "http://api.example.com",
		},
		{
			name:                 "only replaces first occurrence",
			organizationEndpoint: "http://organization.organization.example.com",
			expected:             "http://authn.organization.example.com",
		},
		{
			name:                 "handles port number",
			organizationEndpoint: "http://organization.example.com:9090",
			expected:             "http://authn.example.com:9090",
		},
		{
			name:                 "returns unchanged when no host to replace",
			organizationEndpoint: "not-a-valid-url-organization",
			expected:             "not-a-valid-url-organization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveAuthnEndpoint(tt.organizationEndpoint)
			if result != tt.expected {
				t.Errorf("deriveAuthnEndpoint(%q) = %q, want %q", tt.organizationEndpoint, result, tt.expected)
			}
		})
	}
}
