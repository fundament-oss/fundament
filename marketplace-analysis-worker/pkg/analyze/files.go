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

// limitedReader fails once more than limit bytes were read, rather than
// returning EOF the way io.LimitReader does: a truncated stream must not pass
// for a complete one.
type limitedReader struct {
	r     io.Reader
	limit int64
	read  int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.read += int64(n)
	if l.read > l.limit {
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
	stream := mutate.Extract(img)
	defer func() { _ = stream.Close() }()

	limited := &limitedReader{r: stream, limit: maxBytes}
	reader := tar.NewReader(limited)
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
			// The rest is still read, by tar.Reader skipping it, so the size
			// limit keeps applying.
			truncated = true
			continue
		}

		var file File
		if header.Typeflag == tar.TypeReg {
			sum := sha256.New()
			// Bounded: reader sits on limitedReader, which fails past maxBytes.
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
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return nil, false, sizeOrErr(err, maxBytes, "reading image layers")
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
