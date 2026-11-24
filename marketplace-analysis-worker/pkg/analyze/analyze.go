// Package analyze pulls a plugin version's container image and records what it
// contains: the flattened filesystem and, through Trivy, its packages (FUN-24).
package analyze

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// Config bounds what one analysis may pull and where it may pull it from.
type Config struct {
	// Platform picked from an image index. Plugins run on linux/amd64 nodes.
	Platform v1.Platform
	// MaxImageBytes caps both the compressed layers the manifest declares and
	// the bytes actually decompressed, so a small gzip bomb is caught too.
	MaxImageBytes int64
	// MaxFiles caps the recorded file list; the rest is reported as truncated.
	MaxFiles int
	// RegistryRewrites maps a registry host in a stored reference onto the one
	// the worker should pull from, e.g. localhost:5112 onto the in-cluster
	// name of the local registry. containerd's mirror config does this for
	// kubelet, but not for a client in a pod.
	RegistryRewrites map[string]string
	// InsecureRegistries may be reached over plain http.
	InsecureRegistries []string
	// AllowedPrivateRegistries may resolve to non-public addresses.
	AllowedPrivateRegistries []string
	// TrivyPath is the trivy binary.
	TrivyPath string
	// WorkDir holds the image tarball and Trivy's cache while a job runs.
	WorkDir string
}

// Result is what one analysis found.
type Result struct {
	// Digest of the platform manifest that was analyzed.
	ImageDigest string
	Platform    string
	// Platforms an index offered; empty for a single-platform image.
	Platforms      []string
	Packages       []Package
	Files          []File
	FilesTruncated bool
	SBOM           []byte
}

// Package is one SBOM component, normalized.
type Package struct {
	Purl     string   `json:"purl"`
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Type     string   `json:"type"`
	Licenses []string `json:"licenses"`
}

// File is one entry of the flattened filesystem. Directories are left out.
type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Mode int64  `json:"mode"`
	// Empty for a symlink, which carries its target in Link instead.
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

// PermanentError is a failure that retrying will not fix. Reason is worded for
// the reviewer.
type PermanentError struct {
	Reason    string
	Platforms []string
	Err       error
}

func (e *PermanentError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Reason, e.Err)
	}
	return e.Reason
}

func (e *PermanentError) Unwrap() error { return e.Err }

// Analyzer runs analyses with one Config.
type Analyzer struct {
	cfg       Config
	transport http.RoundTripper
	// sbom runs Trivy; a field so tests can stub it.
	sbom func(ctx context.Context, tarPath, cacheDir string) ([]byte, error)
}

// New returns an Analyzer for a copy of cfg.
func New(cfg *Config) *Analyzer {
	own := *cfg
	if own.Platform.OS == "" {
		own.Platform = v1.Platform{OS: "linux", Architecture: "amd64"}
	}
	a := &Analyzer{
		cfg:       own,
		transport: newTransport(own.AllowedPrivateRegistries, own.InsecureRegistries),
	}
	a.sbom = a.runTrivy
	return a
}

// Analyze pulls the digest-pinned image and records its files and packages.
// Errors that retrying will not fix are a *PermanentError.
func (a *Analyzer) Analyze(ctx context.Context, image string) (*Result, error) {
	ref, err := a.parseReference(image)
	if err != nil {
		return nil, err
	}

	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(a.transport),
		remote.WithAuth(authn.Anonymous),
	}

	result := &Result{}
	img, err := a.pick(ref, result, opts)
	if err != nil {
		return nil, classify(err)
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, classify(fmt.Errorf("reading image digest: %w", err))
	}
	result.ImageDigest = digest.String()

	if err := a.checkDeclaredSize(img); err != nil {
		return nil, classify(err)
	}

	jobDir, err := os.MkdirTemp(a.cfg.WorkDir, "job-")
	if err != nil {
		return nil, fmt.Errorf("creating job directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(jobDir) }()

	// Pulled once to disk: the file walk and Trivy both read the tarball.
	tarPath := filepath.Join(jobDir, "image.tar")
	if err := tarball.WriteToFile(tarPath, ref.Context().Digest(result.ImageDigest), img); err != nil {
		return nil, classify(fmt.Errorf("pulling image: %w", err))
	}

	local, err := tarball.ImageFromPath(tarPath, nil)
	if err != nil {
		return nil, fmt.Errorf("opening pulled image: %w", err)
	}
	result.Files, result.FilesTruncated, err = listFiles(ctx, local, a.cfg.MaxImageBytes, a.cfg.MaxFiles)
	if err != nil {
		return nil, err
	}

	sbom, err := a.sbom(ctx, tarPath, filepath.Join(a.cfg.WorkDir, "trivy-cache"))
	if err != nil {
		return nil, err
	}
	result.SBOM = sbom
	result.Packages, err = parseCycloneDX(sbom)
	if err != nil {
		return nil, &PermanentError{Reason: "Trivy produced an SBOM that could not be read", Err: err}
	}

	return result, nil
}

// parseReference applies the registry rewrites and insists on a digest: the
// registry only accepts digest-pinned definitions, and a tag could move
// between analysis and install.
func (a *Analyzer) parseReference(image string) (name.Digest, error) {
	for from, to := range a.cfg.RegistryRewrites {
		if rest, ok := strings.CutPrefix(image, from+"/"); ok {
			image = to + "/" + rest
			break
		}
	}

	parsed, err := name.NewDigest(image)
	if err != nil {
		return name.Digest{}, &PermanentError{Reason: "The image reference is not a valid digest-pinned reference", Err: err}
	}
	if newHostSet(a.cfg.InsecureRegistries).contains(parsed.RegistryStr()) {
		parsed, err = name.NewDigest(image, name.Insecure)
		if err != nil {
			return name.Digest{}, &PermanentError{Reason: "The image reference is not a valid digest-pinned reference", Err: err}
		}
	}
	return parsed, nil
}

// pick resolves the reference to one image, choosing the configured platform
// from an index.
func (a *Analyzer) pick(ref name.Digest, result *Result, opts []remote.Option) (v1.Image, error) {
	desc, err := remote.Get(ref, opts...)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	if !desc.MediaType.IsIndex() {
		img, err := desc.Image()
		if err != nil {
			return nil, fmt.Errorf("reading image: %w", err)
		}
		cfg, err := img.ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("reading image config: %w", err)
		}
		// The same rule as for an index: a version runs on the configured
		// platform. An image that declares none is given the benefit of the
		// doubt.
		platform := cfg.Platform()
		result.Platform = platformString(platform)
		if platform != nil && (platform.OS != a.cfg.Platform.OS || platform.Architecture != a.cfg.Platform.Architecture) {
			return nil, &PermanentError{
				Reason: fmt.Sprintf("The image is built for %s, not %s",
					result.Platform, platformString(&a.cfg.Platform)),
				Platforms: []string{result.Platform},
			}
		}
		return img, nil
	}

	index, err := desc.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("reading index: %w", err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("reading index manifest: %w", err)
	}

	var chosen *v1.Descriptor
	for i := range manifest.Manifests {
		m := &manifest.Manifests[i]
		if m.Platform == nil || !m.MediaType.IsImage() {
			// Attestation manifests and the like carry no runnable platform.
			continue
		}
		result.Platforms = append(result.Platforms, platformString(m.Platform))
		if chosen == nil && m.Platform.OS == a.cfg.Platform.OS && m.Platform.Architecture == a.cfg.Platform.Architecture {
			chosen = m
		}
	}
	slices.Sort(result.Platforms)
	result.Platforms = slices.Compact(result.Platforms)

	if chosen == nil {
		return nil, &PermanentError{
			Reason: fmt.Sprintf("The image has no %s variant, only %s",
				platformString(&a.cfg.Platform), strings.Join(result.Platforms, ", ")),
			Platforms: result.Platforms,
		}
	}
	result.Platform = platformString(chosen.Platform)

	img, err := remote.Image(ref.Context().Digest(chosen.Digest.String()), opts...)
	if err != nil {
		return nil, fmt.Errorf("fetching %s image: %w", result.Platform, err)
	}
	return img, nil
}

// checkDeclaredSize refuses an image whose compressed layers alone exceed the
// limit, before anything is downloaded. listFiles enforces the limit again on
// the decompressed bytes, which the manifest cannot vouch for.
func (a *Analyzer) checkDeclaredSize(img v1.Image) error {
	manifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("reading image manifest: %w", err)
	}
	var total int64
	for i := range manifest.Layers {
		total += manifest.Layers[i].Size
	}
	if total > a.cfg.MaxImageBytes {
		return &PermanentError{Reason: fmt.Sprintf(
			"The image is %s compressed, over the %s analysis limit",
			formatBytes(total), formatBytes(a.cfg.MaxImageBytes))}
	}
	return nil
}

// classify turns registry and transport errors into permanent ones where
// retrying cannot help.
func classify(err error) error {
	if _, ok := errors.AsType[*PermanentError](err); ok {
		return err
	}
	if errors.Is(err, ErrRefusedAddress) {
		return &PermanentError{Reason: "The image's registry resolves to an address the analysis may not reach", Err: err}
	}
	if errors.Is(err, ErrInsecureScheme) {
		return &PermanentError{Reason: "The image's registry is only reachable over plain http", Err: err}
	}
	if registryErr, ok := errors.AsType[*transport.Error](err); ok {
		switch registryErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return &PermanentError{
				Reason: "Image is private or does not exist. Only public images can be analyzed.",
				Err:    err,
			}
		}
	}
	return err
}

func platformString(p *v1.Platform) string {
	if p == nil {
		return ""
	}
	s := p.OS + "/" + p.Architecture
	if p.Variant != "" {
		s += "/" + p.Variant
	}
	return s
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
