package cli

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

const (
	testSVG   = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><path d="M0 0h8v8z"/></svg>`
	testImage = "localhost:5112/demo@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))))
	return buf.Bytes()
}

func iconManifest(icon, iconDark, orgIcon, projectIcon string) []byte {
	var b strings.Builder
	b.WriteString("apiVersion: fundament.io/v1\nkind: PluginDefinition\nmetadata:\n  name: demo\n  version: v1.0.0\n")
	b.WriteString("  icon: " + icon + "\n")
	if iconDark != "" {
		b.WriteString("  iconDark: " + iconDark + "\n")
	}
	b.WriteString("spec:\n  menu:\n    organization:\n      - crd: a.example.io\n        icon: " + orgIcon + "\n")
	b.WriteString("    project:\n      - crd: b.example.io\n        icon: " + projectIcon + "\n")
	return []byte(b.String())
}

func writeIconFile(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
}

func TestResolveIconFiles_RewritesToDigests(t *testing.T) {
	dir := t.TempDir()
	logo, dark := testPNG(t), []byte(testSVG+"\n")
	org, project := []byte(testSVG+" "), []byte(testSVG+"  ")
	writeIconFile(t, dir, "logo.png", logo)
	writeIconFile(t, dir, "dark.svg", dark)
	writeIconFile(t, dir, "org.svg", org)
	writeIconFile(t, dir, "project.svg", project)

	out, files, paths, err := resolveIconFiles(iconManifest("logo.png", "dark.svg", "org.svg", "project.svg"), dir)
	require.NoError(t, err)

	var def pluginruntime.PluginDefinition
	require.NoError(t, yaml.Unmarshal(out, &def))
	assert.Equal(t, pluginruntime.IconDigest(logo), def.Metadata.Icon)
	assert.Equal(t, pluginruntime.IconDigest(dark), def.Metadata.IconDark)
	assert.Equal(t, pluginruntime.IconDigest(org), def.Spec.Menu.Organization[0].Icon)
	assert.Equal(t, pluginruntime.IconDigest(project), def.Spec.Menu.Project[0].Icon)
	assert.Equal(t, "logo.png", paths[pluginruntime.IconDigest(logo)])
	assert.Equal(t, "org.svg", paths[pluginruntime.IconDigest(org)])
	assert.Equal(t, logo, files[pluginruntime.IconDigest(logo)])
	assert.Equal(t, dark, files[pluginruntime.IconDigest(dark)])
	assert.Equal(t, org, files[pluginruntime.IconDigest(org)])
	assert.Equal(t, project, files[pluginruntime.IconDigest(project)])
}

func TestResolveIconFiles_LeavesNamesAlone(t *testing.T) {
	out, files, paths, err := resolveIconFiles(iconManifest("globe", "", "box", "cog"), t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, files)
	assert.Empty(t, paths)
	var def pluginruntime.PluginDefinition
	require.NoError(t, yaml.Unmarshal(out, &def))
	assert.Equal(t, "globe", def.Metadata.Icon)
	assert.Equal(t, "box", def.Spec.Menu.Organization[0].Icon)
	assert.Equal(t, "cog", def.Spec.Menu.Project[0].Icon)
}

func TestResolveIconFiles_SharedFile(t *testing.T) {
	dir := t.TempDir()
	writeIconFile(t, dir, "i.svg", []byte(testSVG))
	_, files, _, err := resolveIconFiles(iconManifest("globe", "", "i.svg", "i.svg"), dir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

func TestResolveIconFiles_MissingFile(t *testing.T) {
	_, _, _, err := resolveIconFiles(iconManifest("nope.svg", "", "box", "cog"), t.TempDir())
	assert.ErrorContains(t, err, "nope.svg")
}

func TestResolveIconFiles_PathEscape(t *testing.T) {
	outside := t.TempDir()
	writeIconFile(t, outside, "x.svg", []byte(testSVG))
	dir := t.TempDir()
	// Both temp dirs share a parent, so every ref names the real outside file.
	refs := []string{filepath.Join("..", filepath.Base(outside), "x.svg"), filepath.Join(outside, "x.svg")}
	// Creating a symlink needs extra rights on Windows.
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(outside, "x.svg"), filepath.Join(dir, "link.svg")))
		refs = append(refs, "link.svg")
	}

	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			_, _, _, err := resolveIconFiles(iconManifest(ref, "", "box", "cog"), dir)
			require.Error(t, err)
			assert.NotErrorIs(t, err, fs.ErrNotExist)
		})
	}
}

func TestPrepareManifest_ValidatesIcons(t *testing.T) {
	dir := t.TempDir()
	writeIconFile(t, dir, "big.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg">`+strings.Repeat(" ", pluginruntime.MaxLogoBytes)+`</svg>`))
	_, _, err := prepareManifest(iconManifest("big.svg", "", "box", "cog"), dir, testImage, false)
	assert.ErrorContains(t, err, "exceeds")
	assert.ErrorContains(t, err, "big.svg")
	assert.NotContains(t, err.Error(), "sha256:")
}

func TestPrepareManifest_NamesUnsafeSVGByPath(t *testing.T) {
	dir := t.TempDir()
	writeIconFile(t, dir, "evil.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	_, _, err := prepareManifest(iconManifest("globe", "", "evil.svg", "cog"), dir, testImage, false)
	assert.ErrorContains(t, err, "evil.svg")
	assert.ErrorContains(t, err, "must be an SVG")
	assert.ErrorContains(t, err, "<script> is not allowed")
	assert.NotContains(t, err.Error(), "sha256:")
}

func TestPrepareManifest_HashChangesWithIcon(t *testing.T) {
	dir := t.TempDir()
	src := iconManifest("logo.svg", "", "box", "cog")

	writeIconFile(t, dir, "logo.svg", []byte(testSVG))
	first, _, err := prepareManifest(src, dir, testImage, false)
	require.NoError(t, err)
	assert.Contains(t, first.IconFiles, pluginruntime.IconDigest([]byte(testSVG)))

	writeIconFile(t, dir, "logo.svg", []byte(testSVG+" "))
	second, _, err := prepareManifest(src, dir, testImage, false)
	require.NoError(t, err)

	assert.NotEqual(t, pluginruntime.HashManifest(first.Bytes), pluginruntime.HashManifest(second.Bytes))
}
