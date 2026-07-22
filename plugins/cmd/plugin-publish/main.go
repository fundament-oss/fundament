// Command plugin-publish injects a resolved image digest into a plugin's
// image-free source definition.yaml and uploads the resulting manifest to
// organization-api via PutPluginDefinition (the server computes the hash).
//
// The image reference (repo@sha256:<digest>) is provided by the caller
// (--image or PLUGIN_IMAGE) — the `just plugin-publish` recipe builds+pushes the
// image and passes the pushed digest. The catalog plugin id is resolved by name
// via ListPlugins (or supplied explicitly via --plugin-id). Auth: bearer token
// from FUNDAMENT_TOKEN plus the organization context in FUNDAMENT_ORGANIZATION_ID
// (PutPluginDefinition is org-scoped).
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

// withAuth attaches the bearer token (FUNDAMENT_TOKEN) and organization context
// to a Connect request. Every authenticated PluginService call needs these —
// ListPlugins as much as PutPluginDefinition.
func withAuth(req interface{ Header() http.Header }, orgID string) {
	if tok := os.Getenv("FUNDAMENT_TOKEN"); tok != "" {
		req.Header().Set("Authorization", "Bearer "+tok)
	}
	req.Header().Set("Fun-Organization", orgID)
}

// resolvePluginID looks up the catalog plugin id by name via ListPlugins.
func resolvePluginID(ctx context.Context, client organizationv1connect.PluginServiceClient, name, orgID string) (string, error) {
	req := connect.NewRequest(organizationv1.ListPluginsRequest_builder{}.Build())
	withAuth(req, orgID)

	resp, err := client.ListPlugins(ctx, req)
	if err != nil {
		return "", fmt.Errorf("list plugins: %w", err)
	}
	for _, p := range resp.Msg.GetPlugins() {
		if p.GetName() == name {
			return p.GetId(), nil
		}
	}
	return "", fmt.Errorf("no catalog entry for %q — create the plugin in the appstore first", name)
}

func main() {
	var pluginName, image, pluginID string
	flag.StringVar(&pluginName, "plugin", "", "plugin name (directory under plugins/)")
	flag.StringVar(&image, "image", os.Getenv("PLUGIN_IMAGE"), "resolved image digest reference (repo@sha256:...)")
	flag.StringVar(&pluginID, "plugin-id", "", "optional catalog plugin uuid; when empty, resolved by name via ListPlugins")
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

	// Path is relative to the repo root: run via `just plugin-publish` (or from the root).
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
		pluginID, err = resolvePluginID(ctx, client, def.Metadata.Name, orgID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve plugin id: %v\n", err)
			os.Exit(1)
		}
	}

	req := connect.NewRequest(organizationv1.PutPluginDefinitionRequest_builder{
		PluginId:      pluginID,
		PluginVersion: def.Metadata.Version,
		Manifest:      published,
	}.Build())

	withAuth(req, orgID)

	resp, err := client.PutPluginDefinition(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("published plugin=%s version=%s hash=%s id=%s definition_id=%s\n",
		def.Metadata.Name, resp.Msg.GetPluginVersion(), resp.Msg.GetHash(), resp.Msg.GetPluginId(), resp.Msg.GetId())
}
