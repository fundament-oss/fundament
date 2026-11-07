package cli

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// resolveIconFiles replaces icon file paths in the manifest with the digests of
// the files, read from under baseDir, and returns the files and the manifest
// path each came from, both keyed by digest. Built-in icon names are left alone.
func resolveIconFiles(src []byte, baseDir string) ([]byte, map[string][]byte, map[string]string, error) {
	var doc yaml.Node
	metadata, err := topLevelMapping(src, &doc, "metadata")
	if err != nil {
		return nil, nil, nil, err
	}

	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open icon directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	files := map[string][]byte{}
	paths := map[string]string{}
	resolve := func(m *yaml.Node, key string) error {
		node := mapValue(m, key)
		if node == nil || node.Kind != yaml.ScalarNode || !pluginruntime.IsIconFileRef(node.Value) {
			return nil
		}
		path := node.Value
		content, err := readIconFile(root, path)
		if err != nil {
			return err
		}
		digest := pluginruntime.IconDigest(content)
		files[digest] = content
		paths[digest] = path
		setMapValue(m, key, digest)
		return nil
	}

	for _, key := range []string{"icon", "iconDark"} {
		if err := resolve(metadata, key); err != nil {
			return nil, nil, nil, err
		}
	}

	var menu *yaml.Node
	if spec := mapValue(doc.Content[0], "spec"); spec != nil && spec.Kind == yaml.MappingNode {
		menu = mapValue(spec, "menu")
	}
	if menu != nil && menu.Kind == yaml.MappingNode {
		for _, section := range []string{"organization", "project"} {
			entries := mapValue(menu, section)
			if entries == nil || entries.Kind != yaml.SequenceNode {
				continue
			}
			for _, entry := range entries.Content {
				if entry.Kind != yaml.MappingNode {
					continue
				}
				if err := resolve(entry, "icon"); err != nil {
					return nil, nil, nil, err
				}
			}
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return out, files, paths, nil
}

// readIconFile reads path from root, which refuses paths and symlinks that
// leave it. Reads stop just past the largest allowed size; validation reports
// oversized files.
func readIconFile(root *os.Root, path string) ([]byte, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("icon file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(io.LimitReader(f, pluginruntime.MaxLogoBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read icon file %q: %w", path, err)
	}
	return content, nil
}
