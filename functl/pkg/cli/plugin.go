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

	"golang.org/x/term"

	"github.com/fundament-oss/fundament/functl/pkg/scaffold"
)

// gitConfigTimeout bounds the optional `git config` lookups used for defaults.
const gitConfigTimeout = 2 * time.Second

// PluginCmd contains plugin development subcommands. None of them talk to the
// Fundament API, so none of them require authentication.
type PluginCmd struct {
	Create PluginCreateCmd `cmd:"" help:"Scaffold a new plugin project."`
}

// PluginCreateCmd scaffolds a standalone plugin project.
type PluginCreateCmd struct {
	Name string `arg:"" optional:"" help:"Plugin name (lowercase DNS label, e.g. my-plugin)."`

	DisplayName string `help:"Human-readable name shown in the console."`
	Description string `help:"One-line description of what the plugin does."`
	Author      string `help:"Plugin author."`
	License     string `help:"SPDX license identifier."`
	Module      string `help:"Go module path for the generated project."`
	Template    string `help:"Project template: minimal or helm." enum:"minimal,helm," default:""`
	Console     string `help:"Console UI variant: none, vanilla or vite." enum:"none,vanilla,vite," default:""`
	CRD         string `help:"Custom resource the console pages read, as <plural>.<group>."`
	Kind        string `help:"Kubernetes Kind of that custom resource, e.g. Widget."`

	Dir        string `help:"Directory to create the project in (default: ./<name>)."`
	SDKVersion string `name:"sdk-version" help:"plugin-sdk version to pin." default:"${sdk_version}"`
	SDKReplace string `name:"sdk-replace" help:"Point the generated go.mod at a local plugin-sdk checkout instead of a published release."`

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

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]any{
			"name":  opts.Name,
			"dir":   opts.Dir,
			"files": files,
		})
	}

	fmt.Printf("Created %s in %s (%d files)\n", opts.Name, opts.Dir, len(files))

	// git init and go mod tidy are conveniences: the project on disk is already
	// correct without them, so a missing tool warns rather than fails. Failing
	// here would force the user to re-run into a now non-empty directory.
	if c.Git {
		runOptional(opts.Dir, "git", "init", "--quiet")
	}
	tidied := true
	if c.Tidy {
		tidied = c.tidy(&opts)
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
plugin-sdk %s is not published yet. Until it is, point the project at a local
checkout of the fundament repository:

    cd %s
    go mod edit -replace github.com/fundament-oss/fundament/plugin-sdk=/path/to/fundament/plugin-sdk
    go mod tidy

Passing --sdk-replace=/path/to/fundament/plugin-sdk to 'functl plugin create'
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
	license, err := p.ask("License (SPDX id)", c.License, "Apache-2.0")
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
	fmt.Println("Publishing a standalone plugin is not supported yet; see the generated README.")
	fmt.Println("Docs: https://github.com/fundament-oss/fundament/tree/master/docs/developer/plugins")
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

// runOptional runs a command in dir, reporting failures as warnings. Used for
// steps that improve the result but are not required for it to be correct.
func runOptional(dir, name string, args ...string) {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s not found, skipping '%s %s'\n", name, name, strings.Join(args, " "))
		return
	}
	// No timeout: `go mod tidy` on a cold module cache is legitimately slow, and
	// this runs in the foreground where the user can interrupt it.
	//nolint:gosec // G204: every call site passes a literal command name and arguments.
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: '%s %s' failed: %v\n", name, strings.Join(args, " "), err)
	}
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

// titleCaseIdentifier turns "my-plugin" into "MyPlugin".
func titleCaseIdentifier(name string) string {
	return strings.ReplaceAll(titleCase(name), " ", "")
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
