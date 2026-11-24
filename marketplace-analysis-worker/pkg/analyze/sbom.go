package analyze

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// runTrivy produces a CycloneDX SBOM of the image tarball. --format cyclonedx
// turns vulnerability scanning off, and the skip and offline flags keep the
// run from downloading a database or querying a package index: the scan works
// on the pulled bytes only, with no network calls of its own.
func (a *Analyzer) runTrivy(ctx context.Context, tarPath, cacheDir string) ([]byte, error) {
	out := filepath.Join(filepath.Dir(tarPath), "sbom.cdx.json")
	//nolint:gosec // the binary is configuration, the arguments are paths this worker created
	cmd := exec.CommandContext(ctx, a.cfg.TrivyPath,
		"image",
		"--input", tarPath,
		"--format", "cyclonedx",
		"--output", out,
		"--offline-scan",
		"--skip-db-update",
		"--skip-java-db-update",
		"--skip-version-check",
		"--cache-dir", cacheDir,
		"--quiet",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("trivy: %w", ctx.Err())
		}
		return nil, fmt.Errorf("trivy: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	sbom, err := os.ReadFile(out) //nolint:gosec // a path this worker created
	if err != nil {
		return nil, fmt.Errorf("reading trivy output: %w", err)
	}
	return sbom, nil
}

type cdxDocument struct {
	Components []cdxComponent `json:"components"`
}

type cdxComponent struct {
	Name       string         `json:"name"`
	Version    string         `json:"version"`
	Purl       string         `json:"purl"`
	Licenses   []cdxLicense   `json:"licenses"`
	Components []cdxComponent `json:"components"`
}

type cdxLicense struct {
	License *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"license"`
	Expression string `json:"expression"`
}

// parseCycloneDX keeps the components that are packages, the ones with a
// purl: Trivy also lists the operating system and each lock file or binary
// as a component, which are containers of packages rather than packages.
func parseCycloneDX(sbom []byte) ([]Package, error) {
	var doc cdxDocument
	if err := json.Unmarshal(sbom, &doc); err != nil {
		return nil, fmt.Errorf("decoding CycloneDX: %w", err)
	}

	packages := []Package{}
	var walk func(components []cdxComponent)
	walk = func(components []cdxComponent) {
		for i := range components {
			c := &components[i]
			if c.Purl != "" {
				packages = append(packages, Package{
					Purl:     c.Purl,
					Name:     c.Name,
					Version:  c.Version,
					Type:     purlType(c.Purl),
					Licenses: licenseNames(c.Licenses),
				})
			}
			walk(c.Components)
		}
	}
	walk(doc.Components)

	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name != packages[j].Name {
			return packages[i].Name < packages[j].Name
		}
		return packages[i].Purl < packages[j].Purl
	})
	return packages, nil
}

// purlType is the ecosystem, e.g. "deb" for pkg:deb/debian/openssl@3.0.
func purlType(purl string) string {
	rest, ok := strings.CutPrefix(purl, "pkg:")
	if !ok {
		return ""
	}
	typ, _, _ := strings.Cut(rest, "/")
	return typ
}

func licenseNames(licenses []cdxLicense) []string {
	names := []string{}
	for _, l := range licenses {
		switch {
		case l.Expression != "":
			names = append(names, l.Expression)
		case l.License != nil && l.License.ID != "":
			names = append(names, l.License.ID)
		case l.License != nil && l.License.Name != "":
			names = append(names, l.License.Name)
		}
	}
	return names
}
