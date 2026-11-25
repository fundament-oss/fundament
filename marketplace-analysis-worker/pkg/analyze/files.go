package analyze

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

// errTooLarge marks the decompressed stream passing the limit.
var errTooLarge = errors.New("decompressed image exceeds the size limit")

// budget bounds what the walk decompresses, counted where the layers are read
// rather than where mutate.Extract writes: Extract drops the entries an upper
// layer shadows or whites out, but still decompresses them. It also stops the
// reading once ctx is done, since nothing else on the way from the local
// tarball watches ctx. Only Extract's goroutine reads through it.
type budget struct {
	ctx   context.Context
	limit int64
	read  int64
}

// budgetedImage hands out its layers wrapped in budgetedLayer.
type budgetedImage struct {
	v1.Image
	budget *budget
}

func (i budgetedImage) Layers() ([]v1.Layer, error) {
	layers, err := i.Image.Layers()
	if err != nil {
		return nil, err //nolint:wrapcheck // Extract adds the context
	}
	wrapped := make([]v1.Layer, len(layers))
	for n, layer := range layers {
		wrapped[n] = budgetedLayer{Layer: layer, budget: i.budget}
	}
	return wrapped, nil
}

type budgetedLayer struct {
	v1.Layer
	budget *budget
}

func (l budgetedLayer) Uncompressed() (io.ReadCloser, error) {
	rc, err := l.Layer.Uncompressed()
	if err != nil {
		return nil, err //nolint:wrapcheck // Extract adds the context
	}
	return &budgetedReader{ReadCloser: rc, budget: l.budget}, nil
}

// budgetedReader fails once the budget is spent, rather than returning EOF the
// way io.LimitReader does: a truncated layer must not pass for a complete one.
type budgetedReader struct {
	io.ReadCloser
	budget *budget
}

func (r *budgetedReader) Read(p []byte) (int, error) {
	if err := r.budget.ctx.Err(); err != nil {
		return 0, err //nolint:wrapcheck // Extract adds the context
	}
	n, err := r.ReadCloser.Read(p)
	r.budget.read += int64(n)
	if r.budget.read > r.budget.limit {
		return n, errTooLarge
	}
	return n, err //nolint:wrapcheck // io.EOF must reach tar.Reader unwrapped
}

// listFiles walks the flattened filesystem (whiteouts applied, later layers
// winning) and records every regular file and symlink. The walk reads every
// layer in full, decompressed, so it is also where the size limit on the
// decompressed bytes is enforced. It stops once ctx is done, so the job's
// timeout bounds it like the pull and Trivy.
func listFiles(ctx context.Context, img v1.Image, maxBytes int64, maxFiles int) ([]File, bool, error) {
	// Cancelled on return too, so Extract's goroutine stops reading layers
	// when the walk ends early.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream := mutate.Extract(budgetedImage{Image: img, budget: &budget{ctx: ctx, limit: maxBytes}})
	defer func() { _ = stream.Close() }()

	reader := tar.NewReader(stream)
	var files []File
	truncated := false

	for {
		if err := ctx.Err(); err != nil {
			return nil, false, fmt.Errorf("listing files: %w", err)
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, sizeOrErr(err, maxBytes, "reading image layers")
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeSymlink, tar.TypeLink:
		default:
			// Directories, devices and fifos carry no content to compare.
			continue
		}
		if len(files) >= maxFiles {
			// The rest is still read, by tar.Reader skipping it, which keeps
			// Extract going to the end and the budget applying.
			truncated = true
			continue
		}

		var file File
		if header.Typeflag == tar.TypeReg {
			sum := sha256.New()
			// Bounded: the layers are read through the budget, which fails
			// past maxBytes.
			size, err := io.Copy(sum, reader) //nolint:gosec // G110: see limitedReader
			if err != nil {
				return nil, false, sizeOrErr(err, maxBytes, "reading "+header.Name)
			}
			file = File{Size: size, SHA256: hex.EncodeToString(sum.Sum(nil))}
		} else {
			file = File{Link: header.Linkname}
		}
		file.Path = cleanPath(header.Name)
		file.Mode = header.Mode & 0o7777
		files = append(files, file)
	}

	// mutate.Extract closes its tar stream, trailer included, before it hands
	// over the error that ended it: a layer that failed halfway reads as a
	// complete filesystem up to the trailer. The error arrives on the next read.
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return nil, false, sizeOrErr(err, maxBytes, "reading image layers")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, fmt.Errorf("listing files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, truncated, nil
}

func sizeOrErr(err error, maxBytes int64, what string) error {
	if errors.Is(err, errTooLarge) {
		return &PermanentError{Reason: fmt.Sprintf(
			"The image is over the %s analysis limit once decompressed", formatBytes(maxBytes))}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// cleanPath renders a tar entry name as an absolute path.
func cleanPath(name string) string {
	return path.Clean("/" + strings.TrimPrefix(name, "./"))
}
