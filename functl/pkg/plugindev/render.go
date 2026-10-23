package plugindev

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*
var templatesFS embed.FS

// RenderParams are the inputs for the generated dev files.
type RenderParams struct {
	Layout  Layout
	Target  Target
	OrgName string
}

// InstallationName is the PluginInstallation name for an org's plugin.
func InstallationName(orgName, pluginName string) string {
	return orgName + "--" + pluginName
}

// ImageName is the bare dev image name skaffold qualifies at build time.
func ImageName(pluginName string) string {
	return pluginName + "-plugin-dev"
}

type templateData struct {
	RenderParams
	FundamentModule  string
	InstallationName string
	ImageName        string
	Dockerfile       string
	// ProjectIgnore is the module's own .dockerignore, kept for standalone builds.
	ProjectIgnore string
	// ConsoleDir is the plugin's console/ relative to ModuleRoot.
	ConsoleDir string
	// Version is the normalized dev version the registry returned.
	Version string
}

// airDirs are the dirs air watches; sibling packages only matter in the monorepo.
func airDirs(l Layout) []string {
	if l.ModulePath == FundamentModule {
		return l.SourceDirs
	}
	return []string{"."}
}

func parseTemplates() (*template.Template, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{"airDirs": airDirs}).ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return tmpl, nil
}

// Render writes Dockerfile, Dockerfile.dockerignore and skaffold.yaml into
// outDir. Air's config is rendered inside the image.
func Render(outDir string, p RenderParams) error {
	if p.Layout.ModulePath == FundamentModule && len(p.Layout.SourceDirs) == 0 {
		return errors.New("render: SourceDirs not resolved")
	}
	tmpl, err := parseTemplates()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", outDir, err)
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", outDir, err)
	}
	// skaffold joins an absolute dockerfile onto the context, so pass it relative.
	dockerfile, err := filepath.Rel(p.Layout.ModuleRoot, filepath.Join(absOut, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("relate Dockerfile to module root: %w", err)
	}

	consoleDir := path.Join(strings.TrimPrefix(p.Layout.Package, "./"), "console")
	projectIgnore, err := os.ReadFile(filepath.Join(p.Layout.ModuleRoot, ".dockerignore"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read .dockerignore: %w", err)
	}

	data := templateData{
		RenderParams:     p,
		FundamentModule:  FundamentModule,
		InstallationName: InstallationName(p.OrgName, p.Layout.Name),
		ImageName:        ImageName(p.Layout.Name),
		Dockerfile:       filepath.ToSlash(dockerfile),
		ProjectIgnore:    strings.TrimSpace(string(projectIgnore)),
		ConsoleDir:       consoleDir,
	}
	return execute(tmpl, outDir, data, map[string]string{
		"Dockerfile.tmpl":              "Dockerfile",
		"Dockerfile.dockerignore.tmpl": "Dockerfile.dockerignore",
		"skaffold.yaml.tmpl":           "skaffold.yaml",
	})
}

// RenderInstallation writes installation.yaml for the published dev version.
func RenderInstallation(outDir string, p RenderParams, version string) error {
	tmpl, err := parseTemplates()
	if err != nil {
		return err
	}
	data := templateData{
		RenderParams:     p,
		InstallationName: InstallationName(p.OrgName, p.Layout.Name),
		ImageName:        ImageName(p.Layout.Name),
		Version:          version,
	}
	return execute(tmpl, outDir, data, map[string]string{"installation.yaml.tmpl": "installation.yaml"})
}

func execute(tmpl *template.Template, outDir string, data templateData, files map[string]string) error {
	for tmplName, outName := range files {
		var sb strings.Builder
		if err := tmpl.ExecuteTemplate(&sb, tmplName, data); err != nil {
			return fmt.Errorf("render %s: %w", tmplName, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, outName), []byte(sb.String()+"\n"), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outName, err)
		}
	}
	return nil
}
