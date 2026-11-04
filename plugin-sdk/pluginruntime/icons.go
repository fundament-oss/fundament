package pluginruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"image/png"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	// MaxLogoBytes bounds a logo file (light or dark).
	MaxLogoBytes = 16 * 1024
	// MaxMenuIconBytes bounds a menu icon file.
	MaxMenuIconBytes = 8 * 1024
	// MaxIconFiles bounds the number of icon files in one plugin version.
	MaxIconFiles = 64
	// MaxPNGDimension bounds the width and height of a PNG logo, in pixels. A
	// small file can declare far larger dimensions than it is worth decoding.
	MaxPNGDimension = 1024
)

var (
	iconDigestRegex = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	iconNameRegex   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// cssURLRegex finds url( references in CSS or attribute values.
var cssURLRegex = regexp.MustCompile(`url\(\s*["']?\s*(.?)`)

// IsIconFileRef reports whether an icon value names a file or URL rather than
// a built-in icon name: it contains a slash or ends in .svg or .png.
func IsIconFileRef(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(value, "/") || strings.HasSuffix(lower, ".svg") || strings.HasSuffix(lower, ".png")
}

// IsIconDigest reports whether value is a sha256 icon digest.
func IsIconDigest(value string) bool {
	return iconDigestRegex.MatchString(value)
}

// IconDigest returns the digest of an icon file, in the format of HashManifest.
func IconDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ResolvedIcons holds validated icons as data URIs. MenuIcons is keyed by file
// digest and is never nil.
type ResolvedIcons struct {
	Logo      string
	LogoDark  string
	MenuIcons map[string]string
}

// ValidateIcons checks that every icon file reference in def is a digest backed
// by a matching, safe, size-limited file in files, and that every file is
// referenced. Any other non-empty value must be a built-in icon name, which
// passes through untouched.
func ValidateIcons(def *PluginDefinition, files map[string][]byte) (ResolvedIcons, error) {
	if len(files) > MaxIconFiles {
		return ResolvedIcons{}, fmt.Errorf("too many icon files: %d, maximum is %d", len(files), MaxIconFiles)
	}
	fileDigests := slices.Sorted(maps.Keys(files))
	for _, digest := range fileDigests {
		if IconDigest(files[digest]) != digest {
			return ResolvedIcons{}, fmt.Errorf("icon file content does not match its digest %q", digest)
		}
	}

	for _, value := range allIconValues(def) {
		switch {
		case strings.HasPrefix(value, "sha256:") && !IsIconDigest(value):
			return ResolvedIcons{}, fmt.Errorf("icon %q is not a valid digest (sha256:<64 lowercase hex chars>)", value)
		case !IsIconDigest(value) && IsIconFileRef(value):
			return ResolvedIcons{}, fmt.Errorf("icon %q is an unresolved file reference; upgrade functl and publish the plugin again", value)
		case value != "" && !IsIconDigest(value) && !iconNameRegex.MatchString(value):
			return ResolvedIcons{}, fmt.Errorf("icon %q is not a built-in icon name; icon files must end in .svg or .png", value)
		}
	}

	meta := def.Metadata
	var logoDigest, darkDigest string
	if IsIconDigest(meta.Icon) {
		logoDigest = meta.Icon
	}
	switch {
	case meta.IconDark == "":
	case logoDigest == "":
		return ResolvedIcons{}, errors.New("metadata.iconDark requires metadata.icon to be an icon file")
	case !IsIconDigest(meta.IconDark):
		return ResolvedIcons{}, fmt.Errorf("metadata.iconDark %q must be an icon file", meta.IconDark)
	default:
		darkDigest = meta.IconDark
	}

	menuDigests := map[string]bool{}
	for _, entries := range [][]MenuEntry{def.Spec.Menu.Organization, def.Spec.Menu.Project} {
		for _, entry := range entries {
			if IsIconDigest(entry.Icon) {
				menuDigests[entry.Icon] = true
			}
		}
	}

	used := maps.Clone(menuDigests)
	for _, digest := range []string{logoDigest, darkDigest} {
		if digest != "" {
			used[digest] = true
		}
	}

	dataURIs := map[string]string{}
	for _, digest := range slices.Sorted(maps.Keys(used)) {
		content, ok := files[digest]
		if !ok {
			return ResolvedIcons{}, fmt.Errorf("icon %q references no file", digest)
		}
		uri, err := validateIconFile(digest, content, menuDigests[digest])
		if err != nil {
			return ResolvedIcons{}, err
		}
		dataURIs[digest] = uri
	}
	for _, digest := range fileDigests {
		if !used[digest] {
			return ResolvedIcons{}, fmt.Errorf("icon file %q is not referenced by the manifest", digest)
		}
	}

	resolved := ResolvedIcons{Logo: dataURIs[logoDigest], LogoDark: dataURIs[darkDigest], MenuIcons: map[string]string{}}
	for digest := range menuDigests {
		resolved.MenuIcons[digest] = dataURIs[digest]
	}
	return resolved, nil
}

func allIconValues(def *PluginDefinition) []string {
	values := []string{def.Metadata.Icon, def.Metadata.IconDark}
	for _, entries := range [][]MenuEntry{def.Spec.Menu.Organization, def.Spec.Menu.Project} {
		for _, entry := range entries {
			values = append(values, entry.Icon)
		}
	}
	return values
}

// validateIconFile checks one file against the limits of its role. A file used
// as a menu icon anywhere is held to the menu rules (smaller, SVG only).
func validateIconFile(digest string, content []byte, asMenu bool) (string, error) {
	role, limit := "logo", MaxLogoBytes
	if asMenu {
		role, limit = "menu icon", MaxMenuIconBytes
	}
	if len(content) > limit {
		return "", fmt.Errorf("%s file %q exceeds %d bytes", role, digest, limit)
	}
	if config, err := png.DecodeConfig(bytes.NewReader(content)); err == nil && !asMenu {
		if config.Width > MaxPNGDimension || config.Height > MaxPNGDimension {
			return "", fmt.Errorf("logo file %q is %dx%d pixels, maximum is %dx%d", digest, config.Width, config.Height, MaxPNGDimension, MaxPNGDimension)
		}
		return dataURI("image/png", content), nil
	}
	if err := validateSVG(content); err != nil {
		if asMenu {
			return "", fmt.Errorf("menu icon file %q must be an SVG: %w", digest, err)
		}
		return "", fmt.Errorf("logo file %q must be an SVG or PNG: %w", digest, err)
	}
	return dataURI("image/svg+xml", content), nil
}

func dataURI(mime string, content []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(content)
}

const (
	svgNamespace = "http://www.w3.org/2000/svg"
	xmlNamespace = "http://www.w3.org/XML/1998/namespace"
)

// validateSVG walks the XML tokens and rejects anything that can run script or
// fetch external resources when the SVG is shown in a browser. Every element,
// including the root, must be in the SVG namespace. The output is only safe
// when displayed through img or as a CSS mask; it must never be inlined into
// HTML.
func validateSVG(content []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(content))
	depth := 0
	roots := 0
	inStyle := false
	var style strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid SVG: %w", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return errors.New("invalid SVG: DOCTYPE and other directives are not allowed")
		case xml.ProcInst:
			if t.Target != "xml" {
				return fmt.Errorf("invalid SVG: processing instruction %q is not allowed", t.Target)
			}
		case xml.Comment:
			if inStyle {
				return errors.New("invalid SVG: comments inside <style> are not allowed")
			}
			if c := string(t); strings.HasPrefix(c, ">") || strings.HasPrefix(c, "->") {
				return errors.New("invalid SVG: comment starting with > or -> is not allowed")
			}
		case xml.StartElement:
			if inStyle {
				return errors.New("invalid SVG: elements inside <style> are not allowed")
			}
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				roots++
				if roots > 1 || name != "svg" {
					return errors.New("invalid SVG: the root element must be a single <svg>")
				}
			}
			if err := validateSVGElement(t); err != nil {
				return err
			}
			inStyle = name == "style"
			depth++
		case xml.EndElement:
			if inStyle {
				if err := validateCSS("<style>", style.String()); err != nil {
					return err
				}
				style.Reset()
				inStyle = false
			}
			depth--
		case xml.CharData:
			switch {
			case inStyle:
				style.Write(t)
			case depth == 0 && len(bytes.TrimSpace(t)) > 0:
				return errors.New("invalid SVG: text outside the root element")
			}
		}
	}
	if roots == 0 {
		return errors.New("invalid SVG: no <svg> element")
	}
	return nil
}

var forbiddenSVGElements = map[string]bool{
	"script": true, "foreignobject": true, "iframe": true, "embed": true, "object": true,
}

var animatingSVGElements = map[string]bool{
	"set": true, "animate": true, "animatetransform": true, "animatemotion": true,
}

func validateSVGElement(el xml.StartElement) error {
	name := strings.ToLower(el.Name.Local)
	if strings.Contains(name, ":") {
		return fmt.Errorf("invalid SVG: element name %q is not allowed", el.Name.Local)
	}
	if el.Name.Space != svgNamespace {
		return fmt.Errorf("invalid SVG: <%s> must be in the SVG namespace", el.Name.Local)
	}
	if forbiddenSVGElements[name] {
		return fmt.Errorf("invalid SVG: <%s> is not allowed", name)
	}
	for _, attr := range el.Attr {
		attrName := strings.ToLower(attr.Name.Local)
		value := strings.TrimSpace(attr.Value)
		lowerValue := strings.ToLower(value)
		switch {
		case strings.Contains(attrName, ":"):
			return fmt.Errorf("invalid SVG: attribute name %q is not allowed", attr.Name.Local)
		case strings.HasPrefix(attrName, "on"):
			return fmt.Errorf("invalid SVG: event handler attribute %q is not allowed", attr.Name.Local)
		case attrName == "base" && (attr.Name.Space == "xml" || attr.Name.Space == xmlNamespace):
			return errors.New("invalid SVG: attribute xml:base is not allowed")
		case attrName == "href" && !strings.HasPrefix(value, "#"):
			return fmt.Errorf("invalid SVG: href %q must be a local fragment", value)
		case animatingSVGElements[name] && attrName == "attributename" &&
			(strings.HasSuffix(lowerValue, "href") || strings.HasPrefix(lowerValue, "on")):
			return fmt.Errorf("invalid SVG: <%s> must not animate %q", name, value)
		case strings.Contains(lowerValue, "javascript:"):
			return fmt.Errorf("invalid SVG: attribute %q contains a javascript: URL", attr.Name.Local)
		}
		if err := validateCSS("attribute "+strconv.Quote(attr.Name.Local), value); err != nil {
			return err
		}
	}
	return nil
}

var forbiddenCSSTokens = []string{"@import", "expression(", "src(", "image(", "image-set(", "cross-fade("}

// validateCSS rejects imports, escapes, resource-loading functions and any
// url() that is not a local fragment reference. where names the offending
// attribute or element in errors.
func validateCSS(where, css string) error {
	lower := strings.ToLower(css)
	if strings.Contains(lower, "\\") {
		return fmt.Errorf("invalid SVG: %s contains a backslash", where)
	}
	for _, token := range forbiddenCSSTokens {
		if strings.Contains(lower, token) {
			return fmt.Errorf("invalid SVG: %s contains %q", where, token)
		}
	}
	for _, m := range cssURLRegex.FindAllStringSubmatch(lower, -1) {
		if m[1] != "#" {
			return fmt.Errorf("invalid SVG: %s has a url() that is not a local fragment", where)
		}
	}
	return nil
}
