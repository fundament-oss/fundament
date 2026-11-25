package analyze

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSBOM = `{
  "bomFormat": "CycloneDX",
  "components": [
    {"type": "operating-system", "name": "debian", "version": "12.5"},
    {"type": "library", "name": "openssl", "version": "3.0.13",
     "purl": "pkg:deb/debian/openssl@3.0.13?distro=debian-12.5",
     "licenses": [{"license": {"name": "Apache-2.0"}}]},
    {"type": "application", "name": "usr/bin/plugin",
     "components": [
       {"type": "library", "name": "golang.org/x/net", "version": "v0.30.0",
        "purl": "pkg:golang/golang.org/x/net@v0.30.0",
        "licenses": [{"expression": "BSD-3-Clause"}]}
     ]}
  ]
}`

type tarEntry struct {
	name     string
	body     string
	mode     int64
	typeflag byte
	link     string
}

func layerOf(t *testing.T, entries ...tarEntry) v1.Layer {
	t.Helper()

	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		require.NoError(t, w.WriteHeader(&tar.Header{
			Name: e.name, Mode: mode, Size: int64(len(e.body)), Typeflag: typeflag, Linkname: e.link,
		}))
		_, err := w.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())

	data := buf.Bytes()
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
	require.NoError(t, err)
	return layer
}

func imageOf(t *testing.T, platform *v1.Platform, layers ...v1.Layer) v1.Image {
	t.Helper()

	img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)
	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	cfg = cfg.DeepCopy()
	cfg.OS, cfg.Architecture = platform.OS, platform.Architecture
	img, err = mutate.ConfigFile(img, cfg)
	require.NoError(t, err)
	return img
}

// testRegistry serves an in-memory registry. wrap, when set, sits in front of
// it to inject failures.
func testRegistry(t *testing.T, wrap func(http.Handler) http.Handler) string {
	t.Helper()

	handler := registry.New()
	if wrap != nil {
		handler = wrap(handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	return u.Host
}

func push(t *testing.T, host string, img v1.Image) string {
	t.Helper()

	digest, err := img.Digest()
	require.NoError(t, err)
	ref, err := name.NewDigest(host+"/plugin@"+digest.String(), name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))
	return ref.String()
}

type platformImage struct {
	platform v1.Platform
	image    v1.Image
}

func pushIndex(t *testing.T, host string, images ...platformImage) string {
	t.Helper()

	var index v1.ImageIndex = empty.Index
	for i := range images {
		index = mutate.AppendManifests(index, mutate.IndexAddendum{
			Add:        images[i].image,
			Descriptor: v1.Descriptor{Platform: &images[i].platform},
		})
	}
	index = mutate.IndexMediaType(index, types.OCIImageIndex)

	digest, err := index.Digest()
	require.NoError(t, err)
	ref, err := name.NewDigest(host+"/plugin@"+digest.String(), name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(ref, index))
	return ref.String()
}

// newTestAnalyzer may reach the loopback test registry over http, and stubs
// Trivy out.
func newTestAnalyzer(t *testing.T, host string, cfg *Config) *Analyzer {
	t.Helper()

	if cfg.MaxImageBytes == 0 {
		cfg.MaxImageBytes = 64 << 20
	}
	if cfg.MaxFiles == 0 {
		cfg.MaxFiles = 1000
	}
	if cfg.AllowedPrivateRegistries == nil {
		cfg.AllowedPrivateRegistries = []string{host}
	}
	if cfg.InsecureRegistries == nil {
		cfg.InsecureRegistries = []string{host}
	}
	cfg.WorkDir = t.TempDir()

	a := New(cfg)
	a.sbom = func(context.Context, string, string) ([]byte, error) { return []byte(testSBOM), nil }
	return a
}

var amd64 = v1.Platform{OS: "linux", Architecture: "amd64"}

func TestAnalyzeFlattensFilesystem(t *testing.T) {
	host := testRegistry(t, nil)
	img := imageOf(t, &amd64,
		layerOf(t,
			tarEntry{name: "etc/", typeflag: tar.TypeDir, mode: 0o755},
			tarEntry{name: "etc/config", body: "old"},
			tarEntry{name: "etc/removed", body: "gone soon"},
			tarEntry{name: "usr/bin/plugin", body: "binary", mode: 0o755},
		),
		layerOf(t,
			tarEntry{name: "etc/config", body: "new"},
			tarEntry{name: "etc/.wh.removed"},
			tarEntry{name: "usr/bin/run", typeflag: tar.TypeSymlink, link: "plugin", mode: 0o777},
		),
	)
	ref := push(t, host, img)

	result, err := newTestAnalyzer(t, host, &Config{}).Analyze(context.Background(), ref)
	require.NoError(t, err)

	digest, err := img.Digest()
	require.NoError(t, err)
	assert.Equal(t, digest.String(), result.ImageDigest)
	assert.Equal(t, "linux/amd64", result.Platform)
	assert.Empty(t, result.Platforms)
	assert.False(t, result.FilesTruncated)
	assert.Equal(t, []File{
		{Path: "/etc/config", Size: 3, Mode: 0o644, SHA256: sha256Hex("new")},
		{Path: "/usr/bin/plugin", Size: 6, Mode: 0o755, SHA256: sha256Hex("binary")},
		{Path: "/usr/bin/run", Mode: 0o777, Link: "plugin"},
	}, result.Files)

	assert.Equal(t, []Package{
		{Purl: "pkg:golang/golang.org/x/net@v0.30.0", Name: "golang.org/x/net", Version: "v0.30.0", Type: "golang", Licenses: []string{"BSD-3-Clause"}},
		{Purl: "pkg:deb/debian/openssl@3.0.13?distro=debian-12.5", Name: "openssl", Version: "3.0.13", Type: "deb", Licenses: []string{"Apache-2.0"}},
	}, result.Packages)
	assert.JSONEq(t, testSBOM, string(result.SBOM))
}

func TestAnalyzeTruncatesFileList(t *testing.T) {
	host := testRegistry(t, nil)
	ref := push(t, host, imageOf(t, &amd64, layerOf(t,
		tarEntry{name: "a", body: "1"}, tarEntry{name: "b", body: "2"}, tarEntry{name: "c", body: "3"},
	)))

	result, err := newTestAnalyzer(t, host, &Config{MaxFiles: 2}).Analyze(context.Background(), ref)
	require.NoError(t, err)

	assert.Len(t, result.Files, 2)
	assert.True(t, result.FilesTruncated)
}

func TestAnalyzePicksPlatformFromIndex(t *testing.T) {
	host := testRegistry(t, nil)
	amdImage := imageOf(t, &amd64, layerOf(t, tarEntry{name: "amd", body: "x"}))
	arm := v1.Platform{OS: "linux", Architecture: "arm64"}
	armImage := imageOf(t, &arm, layerOf(t, tarEntry{name: "arm", body: "x"}))
	// buildx writes its attestation manifests with this platform.
	attestation := v1.Platform{OS: "unknown", Architecture: "unknown"}
	attestationImage := imageOf(t, &attestation, layerOf(t, tarEntry{name: "att", body: "x"}))
	ref := pushIndex(t, host,
		platformImage{arm, armImage}, platformImage{amd64, amdImage}, platformImage{attestation, attestationImage})

	result, err := newTestAnalyzer(t, host, &Config{}).Analyze(context.Background(), ref)
	require.NoError(t, err)

	digest, err := amdImage.Digest()
	require.NoError(t, err)
	assert.Equal(t, digest.String(), result.ImageDigest)
	assert.Equal(t, "linux/amd64", result.Platform)
	assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, result.Platforms)
	require.Len(t, result.Files, 1)
	assert.Equal(t, "/amd", result.Files[0].Path)
}

func TestAnalyzeIndexWithoutPlatformFailsPermanently(t *testing.T) {
	host := testRegistry(t, nil)
	arm := v1.Platform{OS: "linux", Architecture: "arm64"}
	ref := pushIndex(t, host, platformImage{arm, imageOf(t, &arm, layerOf(t, tarEntry{name: "a", body: "x"}))})

	_, err := newTestAnalyzer(t, host, &Config{}).Analyze(context.Background(), ref)

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
	assert.Equal(t, "The image has no linux/amd64 variant, only linux/arm64", permanent.Reason)
	assert.Equal(t, []string{"linux/arm64"}, permanent.Platforms)
}

func TestAnalyzePrivateImageFailsPermanently(t *testing.T) {
	host := testRegistry(t, func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	})

	_, err := newTestAnalyzer(t, host, &Config{}).Analyze(context.Background(),
		host+"/plugin@sha256:"+strings.Repeat("a", 64))

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
	assert.Equal(t, "Image is private or does not exist. Only public images can be analyzed.", permanent.Reason)
}

func TestAnalyzeServerErrorIsRetryable(t *testing.T) {
	host := testRegistry(t, func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
	})

	_, err := newTestAnalyzer(t, host, &Config{}).Analyze(context.Background(),
		host+"/plugin@sha256:"+strings.Repeat("a", 64))

	require.Error(t, err)
	var permanent *PermanentError
	assert.NotErrorAs(t, err, &permanent)
}

func TestAnalyzeEnforcesDecompressedSize(t *testing.T) {
	host := testRegistry(t, nil)
	// Compresses to a few KiB, well under the limit the manifest is checked
	// against, and decompresses to 4 MiB.
	ref := push(t, host, imageOf(t, &amd64, layerOf(t, tarEntry{name: "zeros", body: strings.Repeat("\x00", 4<<20)})))

	_, err := newTestAnalyzer(t, host, &Config{MaxImageBytes: 1 << 20}).Analyze(context.Background(), ref)

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
	assert.Equal(t, "The image is over the 1.0 MiB analysis limit once decompressed", permanent.Reason)
}

func TestAnalyzeRefusesPrivateAddress(t *testing.T) {
	host := testRegistry(t, nil)
	ref := push(t, host, imageOf(t, &amd64, layerOf(t, tarEntry{name: "a", body: "x"})))

	// The loopback registry is not allowlisted, as a publisher's would not be.
	_, err := newTestAnalyzer(t, host, &Config{AllowedPrivateRegistries: []string{}}).Analyze(context.Background(), ref)

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
	require.ErrorIs(t, err, ErrRefusedAddress)
	assert.Equal(t, "The image's registry resolves to an address the analysis may not reach", permanent.Reason)
}

func TestAnalyzeRefusesPlainHTTP(t *testing.T) {
	host := testRegistry(t, nil)
	ref := push(t, host, imageOf(t, &amd64, layerOf(t, tarEntry{name: "a", body: "x"})))

	_, err := newTestAnalyzer(t, host, &Config{InsecureRegistries: []string{}}).Analyze(context.Background(), ref)

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
	assert.ErrorIs(t, err, ErrInsecureScheme)
}

func TestAnalyzeRewritesRegistry(t *testing.T) {
	host := testRegistry(t, nil)
	ref := push(t, host, imageOf(t, &amd64, layerOf(t, tarEntry{name: "a", body: "x"})))
	stored := strings.Replace(ref, host, "localhost:5112", 1)

	result, err := newTestAnalyzer(t, host, &Config{
		RegistryRewrites: map[string]string{"localhost:5112": host},
	}).Analyze(context.Background(), stored)
	require.NoError(t, err)
	assert.Len(t, result.Files, 1)
}

func TestAnalyzeRequiresDigest(t *testing.T) {
	_, err := newTestAnalyzer(t, "example.com", &Config{}).Analyze(context.Background(), "example.com/plugin:latest")

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
}

func TestIsRefused(t *testing.T) {
	for addr, refused := range map[string]bool{
		"127.0.0.1":       true,
		"10.1.2.3":        true,
		"172.20.0.1":      true,
		"192.168.1.1":     true,
		"169.254.169.254": true,
		"100.64.0.1":      true,
		"0.0.0.0":         true,
		"::1":             true,
		"fd00::1":         true,
		"fe80::1":         true,
		"fe80::1%eth0":    true,
		"::1%lo":          true,
		"::ffff:10.0.0.1": true,
		// NAT64 (well-known and local-use), 6to4 and Teredo embed IPv4.
		"64:ff9b::a00:1":   true,
		"64:ff9b:1::a00:1": true,
		"2002:a00:1::1":    true,
		"2001:0:a00:1::1":  true,
		"140.82.121.33":    false,
		"2606:4700::6810":  false,
		"::ffff:8.8.8.8":   false,
	} {
		assert.Equal(t, refused, isRefused(netip.MustParseAddr(addr)), addr)
	}
}

// Runs the real Trivy over a real image when a trivy binary is on PATH (the
// worker image ships one); skipped otherwise.
func TestTrivyProducesSBOM(t *testing.T) {
	trivy, err := exec.LookPath("trivy")
	if err != nil {
		t.Skip("trivy not on PATH")
	}

	host := testRegistry(t, nil)
	ref := push(t, host, imageOf(t, &amd64, layerOf(t,
		tarEntry{name: "lib/apk/db/installed", body: "P:musl\nV:1.2.5-r0\nA:x86_64\nL:MIT\no:musl\n\n"},
		tarEntry{name: "etc/os-release", body: "ID=alpine\nVERSION_ID=3.20.0\n"},
	)))

	a := newTestAnalyzer(t, host, &Config{TrivyPath: trivy})
	a.sbom = a.runTrivy
	result, err := a.Analyze(context.Background(), ref)
	require.NoError(t, err)

	names := make([]string, 0, len(result.Packages))
	for _, p := range result.Packages {
		names = append(names, p.Name)
	}
	assert.Contains(t, names, "musl")
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
