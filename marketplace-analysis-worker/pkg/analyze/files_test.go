package analyze

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenLayer is a layer whose contents cannot be read, like a blob download
// that fails halfway.
type brokenLayer struct{ v1.Layer }

func (brokenLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, errors.New("connection reset by peer")
}

func TestListFilesReportsLayerFailureAfterTrailer(t *testing.T) {
	// Extract walks the top layer first, so the walk sees /top, then the tar
	// trailer, and only then the lower layer's failure.
	img := imageOf(t, &amd64,
		brokenLayer{layerOf(t, tarEntry{name: "lower", body: "x"})},
		layerOf(t, tarEntry{name: "top", body: "x"}),
	)

	_, _, err := listFiles(context.Background(), img, 1<<20, 100)

	require.ErrorContains(t, err, "connection reset by peer")
}

func TestListFilesStopsWhenCancelled(t *testing.T) {
	img := imageOf(t, &amd64, layerOf(t, tarEntry{name: "a", body: "x"}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := listFiles(ctx, img, 1<<20, 100)

	require.ErrorIs(t, err, context.Canceled)
}

// cancellingLayer cancels the walk once its contents are first read.
type cancellingLayer struct {
	v1.Layer
	cancel context.CancelFunc
}

func (l cancellingLayer) Uncompressed() (io.ReadCloser, error) {
	rc, err := l.Layer.Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("reading layer: %w", err)
	}
	l.cancel()
	return rc, nil
}

func TestListFilesCountsEntriesExtractSkips(t *testing.T) {
	big := tarEntry{name: "big", body: strings.Repeat("\x00", 4<<20)}
	for name, top := range map[string]tarEntry{
		"whited out": {name: ".wh.big"},
		"replaced":   {name: "big", body: "small"},
	} {
		t.Run(name, func(t *testing.T) {
			img := imageOf(t, &amd64, layerOf(t, big), layerOf(t, top))

			_, _, err := listFiles(context.Background(), img, 1<<20, 100)

			var permanent *PermanentError
			require.ErrorAs(t, err, &permanent)
			assert.Equal(t, "The image is over the 1.0 MiB analysis limit once decompressed", permanent.Reason)
		})
	}
}

func TestListFilesStopsWhenCancelledInSkippedEntries(t *testing.T) {
	// Nothing of the lower layer reaches the walk: the top layer whites it out.
	ctx, cancel := context.WithCancel(context.Background())
	img := imageOf(t, &amd64,
		cancellingLayer{Layer: layerOf(t, tarEntry{name: "big", body: strings.Repeat("\x00", 4<<20)}), cancel: cancel},
		layerOf(t, tarEntry{name: ".wh.big"}),
	)

	_, _, err := listFiles(ctx, img, 1<<30, 100)

	require.ErrorIs(t, err, context.Canceled)
}

func TestAnalyzeSinglePlatformImageForAnotherPlatformFailsPermanently(t *testing.T) {
	host := testRegistry(t, nil)
	arm := v1.Platform{OS: "linux", Architecture: "arm64"}
	ref := push(t, host, imageOf(t, &arm, layerOf(t, tarEntry{name: "a", body: "x"})))

	_, err := newTestAnalyzer(t, host, &Config{}).Analyze(context.Background(), ref)

	var permanent *PermanentError
	require.ErrorAs(t, err, &permanent)
	assert.Equal(t, "The image is built for linux/arm64, not linux/amd64", permanent.Reason)
	assert.Equal(t, []string{"linux/arm64"}, permanent.Platforms)
}
