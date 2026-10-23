package plugindev

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

// FundamentModule is the module path of the fundament monorepo.
const FundamentModule = "github.com/fundament-oss/fundament"

// Layout describes where a plugin lives and how it is built.
type Layout struct {
	PluginDir  string
	ModuleRoot string
	ModulePath string
	// Package is "." or "./plugins/<path>", relative to ModuleRoot.
	Package string
	Vite    bool
	// SDKReplace is the absolute local dir of a FundamentModule replace, or "".
	SDKReplace string
	Name       string
	Version    string
	// SourceDirs are the module dirs a FundamentModule plugin build needs,
	// relative to ModuleRoot; set by ResolveSourceDirs. They bound the build
	// context, and with it what syncs into the pod.
	SourceDirs []string
}

// Detect inspects dir, which must hold definition.yaml and sit in a Go module.
func Detect(dir string) (Layout, error) {
	pluginDir, err := filepath.Abs(dir)
	if err != nil {
		return Layout{}, fmt.Errorf("resolve plugin dir: %w", err)
	}

	if _, err := os.Stat(filepath.Join(pluginDir, "definition.yaml")); err != nil {
		return Layout{}, fmt.Errorf("read definition.yaml in %s: %w", pluginDir, err)
	}

	moduleRoot, err := findModuleRoot(pluginDir)
	if err != nil {
		return Layout{}, err
	}

	goMod, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return Layout{}, fmt.Errorf("read go.mod: %w", err)
	}
	mf, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil {
		return Layout{}, fmt.Errorf("parse go.mod: %w", err)
	}
	if mf.Module == nil {
		return Layout{}, fmt.Errorf("%s has no module directive", filepath.Join(moduleRoot, "go.mod"))
	}

	rel, err := filepath.Rel(moduleRoot, pluginDir)
	if err != nil {
		return Layout{}, fmt.Errorf("relate plugin dir to module root: %w", err)
	}
	pkg := "."
	if rel != "." {
		pkg = "./" + filepath.ToSlash(rel)
	}

	def, err := os.ReadFile(filepath.Join(pluginDir, "definition.yaml"))
	if err != nil {
		return Layout{}, fmt.Errorf("read definition.yaml in %s: %w", pluginDir, err)
	}
	var parsed struct {
		Metadata struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal(def, &parsed); err != nil {
		return Layout{}, fmt.Errorf("parse definition.yaml: %w", err)
	}

	l := Layout{
		PluginDir:  pluginDir,
		ModuleRoot: moduleRoot,
		ModulePath: mf.Module.Mod.Path,
		Package:    pkg,
		SDKReplace: sdkReplace(mf, moduleRoot),
		Name:       parsed.Metadata.Name,
		Version:    parsed.Metadata.Version,
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "console-ui", "package.json")); err == nil {
		l.Vite = true
	}
	return l, nil
}

func findModuleRoot(start string) (string, error) {
	for dir := start; ; {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod found in " + start + " or any parent directory")
		}
		dir = parent
	}
}

// sdkReplace returns the absolute local target of a FundamentModule replace.
// Module-version targets are not local paths and yield "".
func sdkReplace(mf *modfile.File, moduleRoot string) string {
	for _, r := range mf.Replace {
		if r.Old.Path != FundamentModule || r.New.Version != "" {
			continue
		}
		if filepath.IsAbs(r.New.Path) {
			return filepath.Clean(r.New.Path)
		}
		return filepath.Join(moduleRoot, r.New.Path)
	}
	return ""
}

// ResolveSourceDirs sets SourceDirs from the plugin's in-module package deps.
// It is a no-op outside the fundament monorepo.
func (l *Layout) ResolveSourceDirs() error {
	if l.ModulePath != FundamentModule {
		return nil
	}
	cmd := exec.Command("go", "list", "-deps",
		"-f", `{{with .Module}}{{if eq .Path "`+FundamentModule+`"}}{{$.Dir}}{{end}}{{end}}`,
		l.Package)
	cmd.Dir = l.ModuleRoot
	// Match the image's build, not the host's.
	cmd.Env = append(os.Environ(), "GOOS=linux", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return fmt.Errorf("go list %s: %w\n%s", l.Package, err, exitErr.Stderr)
		}
		return fmt.Errorf("go list %s: %w", l.Package, err)
	}
	var pkgDirs []string
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(l.ModuleRoot, line)
		if err != nil {
			return fmt.Errorf("relate %s to module root: %w", line, err)
		}
		pkgDirs = append(pkgDirs, filepath.ToSlash(rel))
	}
	l.SourceDirs = sourceDirs(strings.TrimPrefix(l.Package, "./"), pkgDirs)
	return nil
}

// sourceDirs reduces package dirs to the plugin dir plus the top-level dir of
// every dep outside it, dropping dirs nested in another.
func sourceDirs(pluginDir string, pkgDirs []string) []string {
	dirs := []string{pluginDir}
	for _, d := range pkgDirs {
		if !within(d, pluginDir) {
			dirs = append(dirs, strings.SplitN(d, "/", 2)[0])
		}
	}
	slices.Sort(dirs)
	dirs = slices.Compact(dirs)
	return slices.DeleteFunc(dirs, func(d string) bool {
		return slices.ContainsFunc(dirs, func(o string) bool { return o != d && within(d, o) })
	})
}

// within reports whether dir is parent or lies under it.
func within(dir, parent string) bool {
	return dir == parent || strings.HasPrefix(dir, path.Clean(parent)+"/")
}
