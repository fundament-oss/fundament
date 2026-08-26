package scaffold

import (
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden trees in testdata/golden")

// testOptions is a fully specified set of options, so a golden tree never
// depends on the machine it was generated on (git config, working directory).
func testOptions(tmpl, console, dir string) Options {
	return Options{
		Name:        "demo",
		DisplayName: "Demo",
		Description: "A demo plugin.",
		Author:      "Demo Author",
		License:     "Apache-2.0",
		Module:      "example.com/demo-plugin",
		Template:    tmpl,
		Console:     console,
		CRD:         "widgets.example.com",
		Kind:        "Widget",
		Dir:         dir,
		SDKVersion:  DefaultSDKVersion,
	}
}

func combinations() []struct{ Template, Console string } {
	return []struct{ Template, Console string }{
		{TemplateMinimal, ConsoleNone},
		{TemplateMinimal, ConsoleVanilla},
		{TemplateMinimal, ConsoleVite},
		{TemplateHelm, ConsoleNone},
		{TemplateHelm, ConsoleVanilla},
		{TemplateHelm, ConsoleVite},
	}
}

// TestGenerateGolden compares every template combination against a checked-in
// tree. Run `go test ./functl/pkg/scaffold -update` to refresh them.
func TestGenerateGolden(t *testing.T) {
	for _, c := range combinations() {
		name := c.Template + "-" + c.Console
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			files, err := Generate(testOptions(c.Template, c.Console, dir))
			require.NoError(t, err)
			require.NotEmpty(t, files)

			golden := filepath.Join("testdata", "golden", name)
			if *update {
				require.NoError(t, os.RemoveAll(golden))
				require.NoError(t, copyTree(dir, golden))
				return
			}

			assert.Equal(t, treeOf(t, golden), treeOf(t, dir))
		})
	}
}

// TestGeneratedProjectBuilds is the test that actually protects the templates: a
// golden diff catches drift, but only a compiler catches a template that emits
// invalid Go. The generated go.mod points at this repo's plugin-sdk so the test
// is hermetic and does not depend on a published tag.
func TestGeneratedProjectBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds each generated project; skipped under -short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found in PATH")
	}
	sdk, err := filepath.Abs(filepath.Join("..", "..", "..", "plugin-sdk"))
	require.NoError(t, err)
	require.DirExists(t, sdk)

	for _, c := range combinations() {
		t.Run(c.Template+"-"+c.Console, func(t *testing.T) {
			dir := t.TempDir()
			opts := testOptions(c.Template, c.Console, dir)
			opts.SDKReplace = sdk
			_, err := Generate(opts)
			require.NoError(t, err)

			for _, args := range [][]string{
				{"mod", "tidy"},
				{"build", "./..."},
				{"test", "./...", "-count=1"},
			} {
				//nolint:gosec // G204: goBin comes from exec.LookPath and args are literals.
				cmd := exec.CommandContext(t.Context(), goBin, args...)
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "go %s failed:\n%s", strings.Join(args, " "), out)
			}
		})
	}
}

// TestGeneratedGoIsFormatted keeps the templates honest: a scaffolded project
// should be gofmt-clean on its very first commit, and a compiler will not tell
// you otherwise. Conditional blocks in a template make it easy to emit valid but
// badly formatted Go -- misordered imports, stray blank lines.
func TestGeneratedGoIsFormatted(t *testing.T) {
	for _, c := range combinations() {
		t.Run(c.Template+"-"+c.Console, func(t *testing.T) {
			dir := t.TempDir()
			files, err := Generate(testOptions(c.Template, c.Console, dir))
			require.NoError(t, err)

			checked := 0
			for _, rel := range files {
				if filepath.Ext(rel) != ".go" {
					continue
				}
				checked++
				src, err := os.ReadFile(filepath.Join(dir, rel)) //nolint:gosec // G304: rel comes from the scaffolder.
				require.NoError(t, err)
				formatted, err := format.Source(src)
				require.NoError(t, err, "%s must be valid Go", rel)
				assert.Equal(t, string(formatted), string(src), "%s is not gofmt-clean", rel)
			}
			assert.Positive(t, checked, "no Go files were generated")
		})
	}
}

// TestTemplatesAreWellFormed catches the two mistakes the rest of the suite
// cannot: a template file added without the .tmpl suffix (which would be
// compiled as part of functl, or would hide go.mod from this module), and a
// template that does not parse.
func TestTemplatesAreWellFormed(t *testing.T) {
	count := 0
	err := fs.WalkDir(templatesFS, "templates", func(p string, entry fs.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() || p == "templates/README.md" {
			return nil
		}
		count++
		assert.True(t, strings.HasSuffix(p, templateSuffix), "template %q must end in %q", p, templateSuffix)

		src, err := templatesFS.ReadFile(p)
		require.NoError(t, err)
		_, err = template.New(p).Option("missingkey=error").Parse(string(src))
		assert.NoError(t, err, "template %q must parse", p)
		return nil
	})
	require.NoError(t, err)
	assert.Positive(t, count, "no templates were embedded")
}

// TestDotfilesAreEmbedded guards the `all:` prefix on the go:embed directive.
// Without it Go silently drops names beginning with "." or "_", which would cost
// the generated project its .gitignore and the vanilla console its entire
// _shared.js helper library -- with a clean build and no error anywhere.
func TestDotfilesAreEmbedded(t *testing.T) {
	for _, p := range []string{
		"templates/base/.gitignore.tmpl",
		"templates/base/.dockerignore.tmpl",
		"templates/console-vanilla/console/_shared.js.tmpl",
		"templates/console-vite/console/.gitkeep.tmpl",
	} {
		_, err := templatesFS.ReadFile(p)
		assert.NoError(t, err, "%s must be embedded (is the go:embed directive still using the all: prefix?)", p)
	}
}

func TestGenerateValidation(t *testing.T) {
	tests := map[string]struct {
		mutate func(*Options)
		errMsg string
	}{
		"empty name":        {func(o *Options) { o.Name = "" }, "must not be empty"},
		"uppercase name":    {func(o *Options) { o.Name = "Demo" }, "not a valid DNS label"},
		"trailing dash":     {func(o *Options) { o.Name = "demo-" }, "not a valid DNS label"},
		"underscore":        {func(o *Options) { o.Name = "my_plugin" }, "not a valid DNS label"},
		"name too long":     {func(o *Options) { o.Name = strings.Repeat("a", maxPluginNameLen+1) }, "exceeds maximum length"},
		"empty module":      {func(o *Options) { o.Module = "" }, "module path must not be empty"},
		"module with space": {func(o *Options) { o.Module = "example.com/my plugin" }, "whitespace"},
		"unknown template":  {func(o *Options) { o.Template = "rust" }, "unknown template"},
		"unknown console":   {func(o *Options) { o.Console = "svelte" }, "unknown console"},
		"crd without group": {func(o *Options) { o.CRD = "widgets" }, "<plural>.<group>"},
		"lowercase kind":    {func(o *Options) { o.Kind = "widget" }, "UpperCamelCase"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			opts := testOptions(TemplateMinimal, ConsoleNone, t.TempDir())
			tc.mutate(&opts)
			_, err := Generate(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

// A name of exactly the maximum length is still installable, so it must be accepted.
func TestGenerateAcceptsMaxLengthName(t *testing.T) {
	opts := testOptions(TemplateMinimal, ConsoleNone, t.TempDir())
	opts.Name = strings.Repeat("a", maxPluginNameLen)
	_, err := Generate(opts)
	require.NoError(t, err)
}

func TestGenerateRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	//nolint:gosec // G306: test fixture, not a secret.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("keep me"), 0o644))

	_, err := Generate(testOptions(TemplateMinimal, ConsoleNone, dir))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not empty")

	opts := testOptions(TemplateMinimal, ConsoleNone, dir)
	opts.Force = true
	_, err = Generate(opts)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "existing.txt"), "--force must not delete what is already there")
}

// The scaffolder's name rules must stay in step with the plugin-controller's, or
// it will happily generate a plugin that cannot be installed.
func TestNameLimitMatchesController(t *testing.T) {
	// plugin-controller/pkg/controller/resources.go: maxInstallationNameLen = 56,
	// i.e. 63 (the Kubernetes DNS-label limit) minus len("plugin-").
	assert.Equal(t, 63-len("plugin-"), maxPluginNameLen)
}

func TestGoTypeName(t *testing.T) {
	assert.Equal(t, "CertManager", goTypeName("cert-manager"))
	assert.Equal(t, "Demo", goTypeName("demo"))
	assert.Equal(t, "GatewayAPIEnvoy", goTypeName("gateway-aPI-envoy"))
	assert.Equal(t, "Plugin2fa", goTypeName("2fa"), "a leading digit is a valid DNS label but not a Go identifier")
}

// treeOf reads a directory into a path -> content map for comparison.
func treeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("relativise %q: %w", p, err)
		}
		content, err := os.ReadFile(p) //nolint:gosec // G304: p comes from walking a test directory.
		if err != nil {
			return fmt.Errorf("read %q: %w", p, err)
		}
		out[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	require.NoError(t, err)
	return out
}

func copyTree(src, dst string) error {
	//nolint:gosec // G301/G306/G304: golden trees are ordinary source files under testdata.
	err := filepath.WalkDir(src, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return fmt.Errorf("relativise %q: %w", p, err)
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %q: %w", p, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create %q: %w", filepath.Dir(target), err)
		}
		return os.WriteFile(target, content, 0o644)
	})
	if err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}
	return nil
}
