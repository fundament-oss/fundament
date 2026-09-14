package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/functl/pkg/scaffold"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
	"github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1/registryv1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// gitConfigTimeout bounds the optional `git config` lookups used for defaults.
const gitConfigTimeout = 2 * time.Second

// PluginCmd contains the plugin developer commands: scaffolding a project on
// disk, which never talks to the Fundament API and so needs no authentication,
// and pushing a version to the marketplace registry (registry.v1, FUN-20). The
// developer portal edits the listing copy, but a version is published from
// here — the server hashes the manifest bytes into the consent record and reads
// the image out of them.
type PluginCmd struct {
	Create  PluginCreateCmd  `cmd:"" help:"Scaffold a new plugin project."`
	Publish PluginPublishCmd `cmd:"" help:"Push a plugin version to the marketplace registry."`
}

// PluginCreateCmd scaffolds a standalone plugin project.
type PluginCreateCmd struct {
	Name string `arg:"" optional:"" help:"Plugin name (lowercase DNS label, e.g. my-plugin)."`

	DisplayName string `help:"Human-readable name shown in the console."`
	Description string `help:"One-line description of what the plugin does."`
	Author      string `help:"Plugin author."`
	License     string `help:"SPDX license identifier. The text is written to LICENSE for MIT, Apache-2.0, GPL-3.0-only and EUPL-1.2; anything else gets a placeholder."`
	Module      string `help:"Go module path for the generated project."`
	Template    string `help:"Project template: minimal or helm." enum:"minimal,helm," default:""`
	Console     string `help:"Console UI variant: none, vanilla or vite." enum:"none,vanilla,vite," default:""`
	CRD         string `help:"Custom resource the console pages read, as <plural>.<group>."`
	Kind        string `help:"Kubernetes Kind of that custom resource, e.g. Widget."`

	Dir        string `help:"Directory to create the project in (default: ./<name>)."`
	SDKVersion string `name:"sdk-version" help:"fundament version to pin for the plugin SDK." default:"${sdk_version}"`
	SDKReplace string `name:"sdk-replace" help:"Point the generated go.mod at a local fundament checkout instead of a published release."`

	Git   bool `help:"Run 'git init' in the new project." default:"true" negatable:""`
	Tidy  bool `help:"Run 'go mod tidy' in the new project." default:"true" negatable:""`
	Force bool `help:"Write into the target directory even if it is not empty."`
	Yes   bool `help:"Accept all defaults without prompting." short:"y"`
}

// Run executes the plugin create command.
func (c *PluginCreateCmd) Run(ctx *Context) error {
	opts, err := c.resolve()
	if err != nil {
		return err
	}

	files, err := scaffold.Generate(opts)
	if err != nil {
		return fmt.Errorf("failed to scaffold plugin: %w", err)
	}

	if ctx.Output != OutputJSON {
		fmt.Printf("Created %s in %s (%d files)\n", opts.Name, opts.Dir, len(files))
	}

	// git init and go mod tidy are conveniences: the project on disk is already
	// correct without them, so a missing tool warns rather than fails. Failing
	// here would force the user to re-run into a now non-empty directory. They
	// run before the JSON is printed, and report into it: a script that skipped
	// them without knowing would hit "missing go.sum entry" on its first build.
	gitInit := true
	if c.Git {
		gitInit = runOptional(opts.Dir, "git", "init", "--quiet")
	}
	tidied := true
	if c.Tidy {
		tidied = c.tidy(&opts)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]any{
			"name":   opts.Name,
			"dir":    opts.Dir,
			"files":  files,
			"git":    gitInit,
			"tidied": tidied,
		})
	}

	c.printNextSteps(&opts, tidied)
	return nil
}

// tidy runs `go mod tidy` and reports whether it succeeded. A failure is not
// fatal -- the files on disk are already correct -- but it does mean there is no
// go.sum, so the very next `go build` fails with "missing go.sum entry". That is
// confusing enough on its own that it is worth explaining here rather than
// letting a one-line warning scroll past under a cheerful "Next steps".
func (c *PluginCreateCmd) tidy(opts *scaffold.Options) bool {
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(os.Stderr, "\nwarning: go not found, skipping 'go mod tidy'.")
		return false
	}

	//nolint:gosec // G204: literal command and arguments.
	cmd := exec.CommandContext(context.Background(), "go", "mod", "tidy")
	cmd.Dir = opts.Dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true
	}

	fmt.Fprintf(os.Stderr, "\nwarning: 'go mod tidy' failed, so the project has no go.sum and will not build yet:\n\n")
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		fmt.Fprintf(os.Stderr, "    %s\n", line)
	}

	if strings.Contains(string(out), "unknown revision") {
		fmt.Fprintf(os.Stderr, `
fundament %s could not be resolved. Until that version exists, point the
project at a local checkout of the fundament repository:

    cd %s
    go mod edit -replace github.com/fundament-oss/fundament=/path/to/fundament
    go mod tidy

Passing --sdk-replace=/path/to/fundament to 'functl plugin create'
does the same thing when the project is generated.
`, opts.SDKVersion, opts.Dir)
	}
	return false
}

// resolve fills in every option, prompting on a terminal and falling back to
// defaults otherwise, then hands the result to the scaffolder for validation.
func (c *PluginCreateCmd) resolve() (scaffold.Options, error) {
	p := newPrompter(c.Yes)

	name, err := p.ask("Plugin name", c.Name, "")
	if err != nil {
		return scaffold.Options{}, err
	}
	if name == "" {
		return scaffold.Options{}, fmt.Errorf("plugin name is required: pass it as an argument, e.g. 'functl plugin create my-plugin'")
	}

	displayName, err := p.ask("Display name", c.DisplayName, titleCase(name))
	if err != nil {
		return scaffold.Options{}, err
	}
	description, err := p.ask("Description", c.Description, "A Fundament plugin.")
	if err != nil {
		return scaffold.Options{}, err
	}
	author, err := p.ask("Author", c.Author, gitConfig("user.name"))
	if err != nil {
		return scaffold.Options{}, err
	}
	license, err := p.ask("License ("+strings.Join(scaffold.Licenses, ", ")+", or another SPDX id)", c.License, "Apache-2.0")
	if err != nil {
		return scaffold.Options{}, err
	}
	module, err := p.ask("Go module path", c.Module, defaultModule(name))
	if err != nil {
		return scaffold.Options{}, err
	}
	template, err := p.ask("Template (minimal, helm)", c.Template, scaffold.TemplateMinimal)
	if err != nil {
		return scaffold.Options{}, err
	}
	console, err := p.ask("Console UI (none, vanilla, vite)", c.Console, scaffold.ConsoleNone)
	if err != nil {
		return scaffold.Options{}, err
	}

	// The CRD only shapes generated output when there is a console to render it
	// or a chart whose CRDs the plugin verifies, so it is only worth asking about
	// in those cases.
	crdDefault := plural(name) + ".example.com"
	kindDefault := titleCaseIdentifier(name)
	crd, kind := c.CRD, c.Kind
	if console != scaffold.ConsoleNone || template == scaffold.TemplateHelm {
		if crd, err = p.ask("Custom resource (<plural>.<group>)", crd, crdDefault); err != nil {
			return scaffold.Options{}, err
		}
		if kind, err = p.ask("Kind", kind, kindDefault); err != nil {
			return scaffold.Options{}, err
		}
	}
	if crd == "" {
		crd = crdDefault
	}
	if kind == "" {
		kind = kindDefault
	}

	dir := c.Dir
	if dir == "" {
		dir = "./" + name
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return scaffold.Options{}, fmt.Errorf("resolve target directory: %w", err)
	}

	return scaffold.Options{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Author:      author,
		License:     license,
		Module:      module,
		Template:    template,
		Console:     console,
		CRD:         crd,
		Kind:        kind,
		Dir:         abs,
		SDKVersion:  c.SDKVersion,
		SDKReplace:  c.SDKReplace,
		Force:       c.Force,
	}, nil
}

func (c *PluginCreateCmd) printNextSteps(opts *scaffold.Options, tidied bool) {
	// Show a relative path when the project is under the working directory, and
	// the absolute one otherwise: "cd ../../../../tmp/x" helps nobody.
	rel := opts.Dir
	if cwd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(cwd, opts.Dir); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", rel)
	if !tidied {
		fmt.Println("  go mod tidy                  # needs network access, or a warm module cache")
	}
	fmt.Println("  just build")
	fmt.Println("  just test")
	if opts.Console == scaffold.ConsoleVite {
		fmt.Println()
		fmt.Println("  cd console-ui && bun install && bun run build")
		fmt.Println("  git add console-ui/bun.lock  # then switch the Dockerfile to --frozen-lockfile")
	}
	fmt.Println()
	fmt.Println("Then fill in spec.permissions.rbac in definition.yaml and every TODO marker.")
	fmt.Println("Push a version with 'functl plugin publish definition.yaml --image=<repo@sha256:...>'.")
	fmt.Println("Docs: https://github.com/fundament-oss/fundament/tree/master/docs/developer/plugins")
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

// prompter reads answers from stdin when it is a terminal, and otherwise takes
// the default so scripted and CI use never blocks on a prompt.
type prompter struct {
	interactive bool
	reader      *bufio.Reader
}

func newPrompter(yes bool) *prompter {
	// One reader for the whole run: a fresh bufio.Reader per prompt can swallow
	// input that the previous one already buffered.
	return &prompter{interactive: !yes && isTerminal(os.Stdin), reader: bufio.NewReader(os.Stdin)}
}

// ask returns value when it is already set, otherwise prompts (showing def as
// the default), otherwise returns def.
func (p *prompter) ask(label, value, def string) (string, error) {
	if value != "" {
		return value, nil
	}
	if !p.interactive {
		return def, nil
	}

	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	input, err := p.reader.ReadString('\n')
	if err != nil {
		// EOF means the input ended (a closed terminal, or stdin redirected from
		// something empty). Taking the default is more useful than failing on a
		// question the user cannot be asked any more.
		if !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		fmt.Println()
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return def, nil
	}
	return input, nil
}

// isTerminal reports whether f is a terminal, i.e. someone is there to answer a
// prompt. os.ModeCharDevice is not good enough: /dev/null is a character device
// too, so that test prompts into an immediate EOF whenever stdin is </dev/null.
func isTerminal(f *os.File) bool {
	//nolint:gosec // G115: a file descriptor always fits in an int; term.IsTerminal takes one.
	return term.IsTerminal(int(f.Fd()))
}

// runOptional runs a command in dir, reporting failures as warnings and
// returning whether it succeeded. Used for steps that improve the result but
// are not required for it to be correct.
func runOptional(dir, name string, args ...string) bool {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s not found, skipping '%s %s'\n", name, name, strings.Join(args, " "))
		return false
	}
	// No timeout: `go mod tidy` on a cold module cache is legitimately slow, and
	// this runs in the foreground where the user can interrupt it.
	//nolint:gosec // G204: every call site passes a literal command name and arguments.
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: '%s %s' failed: %v\n", name, strings.Join(args, " "), err)
		return false
	}
	return true
}

func gitConfig(key string) string {
	// Reading one config key is instant; a hung git must not hold up a prompt.
	ctx, cancel := context.WithTimeout(context.Background(), gitConfigTimeout)
	defer cancel()
	//nolint:gosec // G204: key is a literal at every call site.
	out, err := exec.CommandContext(ctx, "git", "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// defaultModule guesses a module path from the current git remote, so a plugin
// created inside an already-cloned repository gets a plausible import path.
func defaultModule(name string) string {
	remote := gitConfig("remote.origin.url")
	if owner, ok := githubOwner(remote); ok {
		return "github.com/" + owner + "/" + name + "-plugin"
	}
	return "example.com/" + name + "-plugin"
}

// githubOwner extracts the owner from a GitHub remote in either the SSH
// (git@github.com:owner/repo.git) or HTTPS (https://github.com/owner/repo) form.
func githubOwner(remote string) (string, bool) {
	remote = strings.TrimSuffix(remote, ".git")
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(remote, prefix); ok {
			owner, _, found := strings.Cut(rest, "/")
			if found && owner != "" {
				return owner, true
			}
		}
	}
	return "", false
}

// titleCase turns "my-plugin" into "My Plugin".
func titleCase(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

// titleCaseIdentifier turns "my-plugin" into "MyPlugin". A plugin name may
// start with a digit ("1password"), which a Kind may not, so it gets the same
// "Plugin" prefix scaffold.goTypeName uses for the generated Go type.
func titleCaseIdentifier(name string) string {
	id := strings.ReplaceAll(titleCase(name), " ", "")
	if id != "" && id[0] >= '0' && id[0] <= '9' {
		id = "Plugin" + id
	}
	return id
}

// plural is a deliberately naive pluralisation for the default CRD name; the
// user is prompted and can correct it.
func plural(name string) string {
	name = strings.ReplaceAll(name, "-", "")
	if strings.HasSuffix(name, "s") {
		return name
	}
	return name + "s"
}
