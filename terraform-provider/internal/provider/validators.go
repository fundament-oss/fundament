package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// uuidValidator validates that a string is a valid UUID using the google/uuid library.
type uuidValidator struct{}

func (v uuidValidator) Description(_ context.Context) string {
	return "value must be a valid UUID"
}

func (v uuidValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uuidValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := uuid.Parse(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid UUID",
			fmt.Sprintf("Value %q is not a valid UUID: %s", req.ConfigValue.ValueString(), err),
		)
	}
}

// apiKeyValidator validates that a string starts with the "fun_" prefix.
type apiKeyValidator struct{}

func (v apiKeyValidator) Description(_ context.Context) string {
	return "value must be a valid API key starting with \"fun_\""
}

func (v apiKeyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v apiKeyValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if !strings.HasPrefix(req.ConfigValue.ValueString(), "fun_") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid API Key",
			fmt.Sprintf("API key must start with \"fun_\", got %q", req.ConfigValue.ValueString()),
		)
	}
}

// httpURLValidator validates that a string is a valid HTTP or HTTPS URL.
type httpURLValidator struct{}

func (v httpURLValidator) Description(_ context.Context) string {
	return "value must be a valid HTTP or HTTPS URL"
}

func (v httpURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v httpURLValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	raw := req.ConfigValue.ValueString()

	u, err := url.Parse(raw)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid URL",
			fmt.Sprintf("Value %q is not a valid URL: %s", raw, err),
		)
		return
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid URL Scheme",
			fmt.Sprintf("URL must use http or https scheme, got %q", u.Scheme),
		)
		return
	}

	if u.Host == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid URL",
			fmt.Sprintf("URL %q is missing a host", raw),
		)
	}
}

// jwtValidator validates that a string is a well-formed JWT token.
type jwtValidator struct{}

func (v jwtValidator) Description(_ context.Context) string {
	return "value must be a valid JWT token"
}

func (v jwtValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v jwtValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	_, _, err := parser.ParseUnverified(req.ConfigValue.ValueString(), jwt.MapClaims{})
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid JWT Token",
			fmt.Sprintf("Value is not a valid JWT token: %s", err),
		)
	}
}
