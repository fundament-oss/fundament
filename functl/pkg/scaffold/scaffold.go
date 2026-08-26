// Package scaffold generates a new, standalone Fundament plugin project from a
// set of templates embedded in the functl binary.
//
// Generation is pure: it renders every file into memory and only then writes to
// disk, so a broken template cannot leave a half-scaffolded directory behind. It
// never touches the network and never reads credentials -- creating a plugin
// does not require a Fundament account.
package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// Templates are embedded with the "all:" prefix so Go does not skip entries
// whose names begin with "." or "_". Without it .gitignore, .dockerignore and
// the vanilla console's _shared.js helper library would silently vanish from the
// binary, and a scaffolded plugin would build cleanly but serve a broken console.
//
//go:embed all:templates
var templatesFS embed.FS

// templateSuffix is stripped from every embedded path when the file is written.
// Every template file carries it, with no exceptions: a file literally named
// go.mod would exclude templates/ from this module, and a file named *.go would
// be compiled and vetted as part of functl itself.
const templateSuffix = ".tmpl"

// Template names.
const (
	TemplateMinimal = "minimal"
	TemplateHelm    = "helm"
)

// Console UI variants.
const (
	ConsoleNone    = "none"
	ConsoleVanilla = "vanilla"
	ConsoleVite    = "vite"
)

// Options describes the plugin project to generate.
type Options struct {
	Name        string
	DisplayName string
	Description string
	Author      string
	License     string
	Module      string
	Template    string
	Console     string
	CRD         string
	Kind        string
	Dir         string
	SDKVersion  string
	SDKReplace  string
	Force       bool
}

// data is what the templates see. It is a distinct type from Options so derived
// values (Go identifiers, the CRD split into its parts, the template-set
// booleans) are computed once, in one place, rather than in template expressions.
type data struct {
	Name        string
	DisplayName string
	Description string
	Author      string
	License     string
	Module      string
	SDKVersion  string
	SDKReplace  string

	// BinaryName is the compiled binary and the default image repository name.
	BinaryName string
	// GoType is the plugin's Go type, e.g. "MyPluginPlugin" -> "MyPlugin".
	GoType string

	CRD            string
	ResourcePlural string
	Group          string
	Kind           string

	IsHelm         bool
	HasConsole     bool
	ConsoleVanilla bool
	ConsoleVite    bool
}

// Generate renders the plugin project described by opts into opts.Dir and
// returns the paths written, relative to opts.Dir, in sorted order.
//
//nolint:gocritic // hugeParam: Options is this package's public entry point; a value keeps the API obvious and Generate is called once per run.
func Generate(opts Options) ([]string, error) {
	if err := validate(&opts); err != nil {
		return nil, err
	}

	d := newData(&opts)

	sets := []string{"base", "plugin-" + opts.Template}
	switch opts.Console {
	case ConsoleNone:
	case ConsoleVanilla:
		sets = append(sets, "console-vanilla")
	case ConsoleVite:
		sets = append(sets, "console-vite")
	default:
		panic(fmt.Sprintf("scaffold: unhandled console variant %q", opts.Console))
	}

	files := map[string][]byte{}
	for _, set := range sets {
		if err := renderSet(set, d, files); err != nil {
			return nil, err
		}
	}

	if err := writeAll(opts.Dir, files); err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

func validate(opts *Options) error {
	if err := validateName(opts.Name); err != nil {
		return err
	}
	if err := validateModule(opts.Module); err != nil {
		return err
	}
	switch opts.Template {
	case TemplateMinimal, TemplateHelm:
	default:
		return fmt.Errorf("unknown template %q (want %q or %q)", opts.Template, TemplateMinimal, TemplateHelm)
	}
	switch opts.Console {
	case ConsoleNone, ConsoleVanilla, ConsoleVite:
	default:
		return fmt.Errorf("unknown console variant %q (want %q, %q or %q)", opts.Console, ConsoleNone, ConsoleVanilla, ConsoleVite)
	}
	if err := validateCRD(opts.CRD); err != nil {
		return err
	}
	if err := validateKind(opts.Kind); err != nil {
		return err
	}
	if opts.Dir == "" {
		return fmt.Errorf("target directory must not be empty")
	}
	if opts.SDKVersion == "" {
		opts.SDKVersion = DefaultSDKVersion
	}
	return validateTargetDir(opts.Dir, opts.Force)
}

func newData(opts *Options) *data {
	plural, group, _ := strings.Cut(opts.CRD, ".")
	return &data{
		Name:           opts.Name,
		DisplayName:    opts.DisplayName,
		Description:    opts.Description,
		Author:         opts.Author,
		License:        opts.License,
		Module:         opts.Module,
		SDKVersion:     opts.SDKVersion,
		SDKReplace:     opts.SDKReplace,
		BinaryName:     opts.Name + "-plugin",
		GoType:         goTypeName(opts.Name),
		CRD:            opts.CRD,
		ResourcePlural: plural,
		Group:          group,
		Kind:           opts.Kind,
		IsHelm:         opts.Template == TemplateHelm,
		HasConsole:     opts.Console != ConsoleNone,
		ConsoleVanilla: opts.Console == ConsoleVanilla,
		ConsoleVite:    opts.Console == ConsoleVite,
	}
}

// goTypeName turns a DNS-label plugin name into an UpperCamelCase Go identifier:
// "cert-manager" -> "CertManager".
func goTypeName(name string) string {
	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '-' {
			upper = true
			continue
		}
		if upper && r >= 'a' && r <= 'z' {
			b.WriteRune(r - 'a' + 'A')
		} else {
			b.WriteRune(r)
		}
		upper = false
	}
	out := b.String()
	// A name starting with a digit is a valid DNS label but not a Go identifier.
	if out != "" && out[0] >= '0' && out[0] <= '9' {
		out = "Plugin" + out
	}
	return out
}

// renderSet renders every template under templates/<set>/ into files, keyed by
// the destination path relative to the project root. Paths are rendered through
// the template engine too, so a filename can depend on the resource being
// scaffolded (console/{{.ResourcePlural}}-list.html).
func renderSet(set string, d *data, files map[string][]byte) error {
	root := "templates/" + set
	if err := fs.WalkDir(templatesFS, root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk templates: %w", err)
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, templateSuffix) {
			return fmt.Errorf("template %q does not end in %q", p, templateSuffix)
		}

		rel := strings.TrimSuffix(strings.TrimPrefix(p, root+"/"), templateSuffix)
		dest, err := renderString("path:"+p, rel, d)
		if err != nil {
			return err
		}

		src, err := templatesFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read template %q: %w", p, err)
		}
		out, err := renderString(p, string(src), d)
		if err != nil {
			return err
		}

		if existing, dup := files[dest]; dup && !bytes.Equal(existing, []byte(out)) {
			return fmt.Errorf("template %q would overwrite %q with different content", p, dest)
		}
		files[dest] = []byte(out)
		return nil
	}); err != nil {
		return fmt.Errorf("render template set %q: %w", set, err)
	}
	return nil
}

func renderString(name, text string, d *data) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("render template %q: %w", name, err)
	}
	return buf.String(), nil
}

func writeAll(dir string, files map[string][]byte) error {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, rel := range paths {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		//nolint:gosec // G301: a scaffolded project directory is ordinary source, not a secret.
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("create directory for %q: %w", rel, err)
		}
		//nolint:gosec // G306: a scaffolded project file is ordinary source, not a secret.
		if err := os.WriteFile(full, files[rel], 0o644); err != nil {
			return fmt.Errorf("write %q: %w", rel, err)
		}
	}
	return nil
}
