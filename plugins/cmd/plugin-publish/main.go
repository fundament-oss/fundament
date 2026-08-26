// Command plugin-publish injects a resolved image digest into a plugin's
// image-free source definition.yaml and uploads the resulting manifest to
// organization-api via PutPluginDefinition (the server computes the hash).
//
// The image reference (repo@sha256:<digest>) is provided by the caller
// (--image or PLUGIN_IMAGE) — the `just plugins publish` recipe builds+pushes the
// image and passes the pushed digest. The catalog plugin id is resolved by
// (organization, name) via ListPlugins — a bare name can match several
// publishers — or supplied explicitly via --plugin-id. The organization is the
// one FUNDAMENT_ORGANIZATION_ID names, resolved via GetOrganization; override
// with --organization. Auth: bearer token from FUNDAMENT_TOKEN plus the
// organization context in FUNDAMENT_ORGANIZATION_ID (PutPluginDefinition is
// org-scoped).
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// setMapValue sets key=value on a YAML mapping node, replacing an existing key
// or appending a new scalar pair.
func setMapValue(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].Kind = yaml.ScalarNode
			m.Content[i+1].Tag = "!!str"
			m.Content[i+1].Value = value
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// injectImage sets spec.image + spec.imagePullPolicy on the source manifest,
// preserving the rest of the document, and returns the published bytes.
func injectImage(src []byte, image, pullPolicy string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("unexpected manifest structure")
	}
	root := doc.Content[0]
	var spec *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "spec" {
			spec = root.Content[i+1]
			break
		}
	}
	if spec == nil || spec.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("manifest has no spec mapping")
	}
	setMapValue(spec, "image", image)
	setMapValue(spec, "imagePullPolicy", pullPolicy)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return out, nil
}

// withAuth returns a context that attaches the bearer token (FUNDAMENT_TOKEN)
// and organization context to the next Connect call. Every authenticated
// PluginService call needs these — ListPlugins as much as PutPluginDefinition.
// Call it once per request; the returned context carries call-specific state.
func withAuth(ctx context.Context, orgID string) context.Context {
	ctx, callInfo := connect.NewClientContext(ctx)
	if tok := os.Getenv("FUNDAMENT_TOKEN"); tok != "" {
		callInfo.RequestHeader().Set("Authorization", "Bearer "+tok)
	}
	callInfo.RequestHeader().Set("Fun-Organization", orgID)
	return ctx
}

// resolveOrganizationName looks up the publishing organization's name for the
// org this run is acting as. ListPlugins reports the publisher by name, but the
// caller only knows its id, so resolve one to the other rather than making the
// operator supply both and risk them disagreeing.
func resolveOrganizationName(ctx context.Context, client organizationv1connect.OrganizationServiceClient, orgID string) (string, error) {
	resp, err := client.GetOrganization(withAuth(ctx, orgID), organizationv1.GetOrganizationRequest_builder{
		Id: orgID,
	}.Build())
	if err != nil {
		return "", fmt.Errorf("get organization %s: %w", orgID, err)
	}
	return resp.GetOrganization().GetName(), nil
}

// resolvePluginID looks up the catalog plugin id by name via ListPlugins. A
// plugin's identity is (organization, name), so a bare name can match entries
// from several publishers — narrow with --organization, or pass --plugin-id.
func resolvePluginID(ctx context.Context, client organizationv1connect.PluginServiceClient, name, organization, orgID string) (string, error) {
	resp, err := client.ListPlugins(withAuth(ctx, orgID), organizationv1.ListPluginsRequest_builder{}.Build())
	if err != nil {
		return "", fmt.Errorf("list plugins: %w", err)
	}
	var matches []*organizationv1.PluginSummary
	for _, p := range resp.GetPlugins() {
		if p.GetName() != name {
			continue
		}
		if organization != "" && p.GetOrganizationName() != organization {
			continue
		}
		matches = append(matches, p)
	}
	switch len(matches) {
	case 0:
		if organization != "" {
			return "", fmt.Errorf("no catalog entry for %q published by %q", name, organization)
		}
		return "", fmt.Errorf("no catalog entry for %q — create the plugin in the appstore first", name)
	case 1:
		return matches[0].GetId(), nil
	default:
		publishers := make([]string, 0, len(matches))
		for _, p := range matches {
			publishers = append(publishers, p.GetOrganizationName())
		}
		return "", fmt.Errorf("%q is published by more than one organization (%s); pass --organization or --plugin-id",
			name, strings.Join(publishers, ", "))
	}
}

func main() {
	var pluginName, image, pluginID, organization string
	var replace bool
	flag.StringVar(&pluginName, "plugin", "", "plugin name (directory under plugins/)")
	flag.StringVar(&image, "image", os.Getenv("PLUGIN_IMAGE"), "resolved image digest reference (repo@sha256:...)")
	flag.StringVar(&pluginID, "plugin-id", "", "optional catalog plugin uuid; when empty, resolved by name via ListPlugins")
	flag.StringVar(&organization, "organization", os.Getenv("FUNDAMENT_ORGANIZATION_NAME"), "publishing organization name; defaults to the name of FUNDAMENT_ORGANIZATION_ID, resolved via GetOrganization")
	flag.BoolVar(&replace, "replace", false, "republish: soft-delete an existing definition for this version and store this one in its place")
	flag.Parse()

	if pluginName == "" || image == "" {
		fmt.Fprintln(os.Stderr, "usage: plugin-publish --plugin <name> --image repo@sha256:<digest> [--plugin-id <uuid>]")
		os.Exit(2)
	}

	apiURL := os.Getenv("FUNDAMENT_ORG_API_URL")
	if apiURL == "" {
		fmt.Fprintln(os.Stderr, "FUNDAMENT_ORG_API_URL is required")
		os.Exit(1)
	}

	orgID := os.Getenv("FUNDAMENT_ORGANIZATION_ID")
	if orgID == "" {
		fmt.Fprintln(os.Stderr, "FUNDAMENT_ORGANIZATION_ID is required (PutPluginDefinition is org-scoped)")
		os.Exit(1)
	}

	// Path is relative to the repo root: run via `just plugins publish` (or from the root).
	src, err := os.ReadFile(filepath.Join("plugins", pluginName, "definition.yaml")) //nolint:gosec // path is built from a CLI flag, not untrusted input
	if err != nil {
		fmt.Fprintf(os.Stderr, "read definition: %v\n", err)
		os.Exit(1)
	}

	published, err := injectImage(src, image, "IfNotPresent")
	if err != nil {
		fmt.Fprintf(os.Stderr, "inject image: %v\n", err)
		os.Exit(1)
	}

	def, err := pluginruntime.ParseDefinition(published) // strict validation
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid published manifest: %v\n", err)
		os.Exit(1)
	}

	client := organizationv1connect.NewPluginServiceClient(&http.Client{Timeout: 30 * time.Second}, apiURL)
	ctx := context.Background()

	if pluginID == "" {
		if organization == "" {
			orgClient := organizationv1connect.NewOrganizationServiceClient(&http.Client{Timeout: 30 * time.Second}, apiURL)
			organization, err = resolveOrganizationName(ctx, orgClient, orgID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		}
		pluginID, err = resolvePluginID(ctx, client, def.Metadata.Name, organization, orgID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve plugin id: %v\n", err)
			os.Exit(1)
		}
	}

	resp, err := client.PutPluginDefinition(withAuth(ctx, orgID), organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID,
		PluginVersion: def.Metadata.Version,
		Manifest:      published,
		Replace:       replace,
	}.Build())
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("published plugin=%s version=%s hash=%s id=%s definition_id=%s\n",
		def.Metadata.Name, resp.GetPluginVersion(), resp.GetHash(), resp.GetPluginId(), resp.GetId())
}
