package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"gopkg.in/yaml.v3"

	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1/registryv1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// PluginCmd contains plugin publishing subcommands. This is the marketplace's
// publishing surface (registry.v1, FUN-20): the developer portal edits the
// listing copy, but a version is pushed from here — the server hashes the
// manifest bytes into the consent record and reads the image out of them.
type PluginCmd struct {
	Publish PluginPublishCmd `cmd:"" help:"Push a plugin version to the marketplace registry."`
}

// PluginPublishCmd pushes one version: it injects the resolved image digest
// into an image-free source definition.yaml and uploads the resulting
// manifest via CreatePluginVersion. The pushed version lands in DRAFT;
// --submit opens the review round in the same run.
type PluginPublishCmd struct {
	Definition string `arg:"" type:"existingfile" help:"Path to the plugin's source definition.yaml (image-free; the digest is injected)."`
	Image      string `required:"" env:"PLUGIN_IMAGE" help:"Resolved image digest reference (repo@sha256:...)."`
	PluginID   string `name:"plugin-id" help:"Listing uuid; when empty, resolved by the manifest's metadata.name via ListPlugins."`
	Create     bool   `help:"Reserve the listing first when it does not exist, seeded from the manifest's metadata."`
	Submit     bool   `help:"Open a review round for the pushed version in the same run."`
}

// Run executes the plugin publish command.
func (c *PluginPublishCmd) Run(ctx *Context) error {
	src, err := os.ReadFile(c.Definition) //nolint:gosec // path comes from the operator's own CLI argument
	if err != nil {
		return fmt.Errorf("read definition: %w", err)
	}

	published, err := injectImage(src, c.Image, "IfNotPresent")
	if err != nil {
		return fmt.Errorf("inject image: %w", err)
	}

	// Strict validation, the same parse the server runs — a rejection here is
	// the same rejection the registry would answer with.
	def, err := pluginruntime.ParseDefinition(published)
	if err != nil {
		return fmt.Errorf("invalid published manifest: %w", err)
	}

	registryURL := ctx.Config.RegistryURL
	if registryURL == "" {
		return errors.New("no registry endpoint: set registry_url in the functl config or FUNCTL_REGISTRY_URL")
	}

	apiClient, err := NewClientFromConfigWithOrg(ctx)
	if err != nil {
		return err
	}
	pubClient := apiClient.Publications(registryURL)
	callCtx := context.Background()

	pluginID := c.PluginID
	if pluginID == "" {
		pluginID, err = resolvePluginID(callCtx, pubClient, def.Metadata.Name)
		if errors.Is(err, errListingNotFound) && c.Create {
			pluginID, err = createPlugin(callCtx, pubClient, def)
			if err == nil {
				fmt.Printf("created listing plugin=%s id=%s\n", def.Metadata.Name, pluginID)
			}
		}
		if err != nil {
			return fmt.Errorf("resolve plugin id: %w", err)
		}
	}

	resp, err := pubClient.CreatePluginVersion(callCtx, registryv1.CreatePluginVersionRequest_builder{
		PluginId: pluginID,
		Version:  def.Metadata.Version,
		Manifest: published,
	}.Build())
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeAlreadyExists {
			// Create-only by design: an approved version's hash is a consent
			// record and stays immutable (FUN-20).
			return fmt.Errorf("%w\n(versions are create-only; bump metadata.version — a -dev prerelease suffix works)", err)
		}
		return fmt.Errorf("publish failed: %w", err)
	}

	version := resp.GetVersion()
	fmt.Printf("published plugin=%s version=%s hash=%s id=%s version_id=%s status=%s\n",
		def.Metadata.Name, version.GetVersion(), version.GetDefinitionHash(),
		version.GetPluginId(), version.GetId(), version.GetStatus())

	if c.Submit {
		submitted, err := pubClient.SubmitPluginVersion(callCtx, registryv1.SubmitPluginVersionRequest_builder{
			PluginVersionId: version.GetId(),
		}.Build())
		if err != nil {
			return fmt.Errorf("submit failed: %w", err)
		}
		fmt.Printf("submitted version_id=%s status=%s\n",
			submitted.GetVersion().GetId(), submitted.GetVersion().GetStatus())
	}

	return nil
}

// resolvePluginID looks up the listing id by name. ListPlugins is already
// scoped to the caller's organization by the registry, so a bare name cannot
// collide with another publisher's listing of the same name.
func resolvePluginID(ctx context.Context, pubClient registryv1connect.PublicationServiceClient, name string) (string, error) {
	resp, err := pubClient.ListPlugins(ctx, registryv1.ListPluginsRequest_builder{}.Build())
	if err != nil {
		return "", fmt.Errorf("list plugins: %w", err)
	}
	for _, p := range resp.GetPlugins() {
		if p.GetName() == name {
			return p.GetId(), nil
		}
	}
	return "", fmt.Errorf("%w: no listing named %q in this organization — pass --create to reserve it", errListingNotFound, name)
}

// errListingNotFound marks the one resolvePluginID failure --create may act
// on. A transport error, an expired token or a permission denial must
// propagate instead of being answered with a CreatePlugin.
var errListingNotFound = errors.New("listing not found")

// createPlugin reserves the listing, seeded from the manifest's metadata. The
// listing copy (display name, descriptions, tags) is authored properly through
// the developer portal afterwards; this is just enough to publish against.
func createPlugin(ctx context.Context, pubClient registryv1connect.PublicationServiceClient, def pluginruntime.PluginDefinition) (string, error) {
	displayName := def.Metadata.DisplayName
	if displayName == "" {
		displayName = def.Metadata.Name
	}
	// Truncate on rune boundaries: the field's max_len counts characters, and
	// a byte cut inside a multi-byte rune would fail protobuf marshalling.
	descriptionShort := def.Metadata.Description
	if runes := []rune(descriptionShort); len(runes) > 255 {
		descriptionShort = string(runes[:255])
	}
	tags := def.Metadata.Tags
	if len(tags) > 20 {
		tags = tags[:20]
	}

	resp, err := pubClient.CreatePlugin(ctx, registryv1.CreatePluginRequest_builder{
		Name:             def.Metadata.Name,
		DisplayName:      displayName,
		DescriptionShort: descriptionShort,
		Description:      def.Metadata.Description,
		Tags:             tags,
		RepositoryUrl:    def.Metadata.URLs.Repository,
		License:          def.Metadata.License,
		Visibility:       registryv1.PluginVisibility_PLUGIN_VISIBILITY_PUBLIC,
	}.Build())
	if err != nil {
		return "", fmt.Errorf("create plugin: %w", err)
	}
	return resp.GetPlugin().GetId(), nil
}

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
