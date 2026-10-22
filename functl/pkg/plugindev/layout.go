package plugindev

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
