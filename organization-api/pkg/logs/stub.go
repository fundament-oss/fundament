package logs

import (
	"context"
	"time"
)

// StubClient is a no-op Client used when no log backend is configured.
// All methods return empty results without errors.
type StubClient struct{}

func (StubClient) Backend() Backend { return BackendNone }

func (StubClient) Query(_ context.Context, _ *QueryParams) ([]Entry, error) {
	return nil, nil
}

func (StubClient) Tail(_ context.Context, _ *QueryParams) (<-chan TailEvent, error) {
	ch := make(chan TailEvent)
	close(ch)
	return ch, nil
}

func (StubClient) Labels(_ context.Context, _, _ string, _, _ time.Time) (Labels, error) {
	return Labels{}, nil
}
