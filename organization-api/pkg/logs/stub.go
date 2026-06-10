package logs

import "context"

// StubClient is a no-op Client used when no log backend is configured.
// All methods return empty results without errors.
type StubClient struct{}

func (StubClient) Backend() Backend { return BackendNone }

func (StubClient) Query(_ context.Context, _ QueryParams) ([]Entry, error) {
	return nil, nil
}

func (StubClient) Tail(_ context.Context, _ QueryParams) (<-chan Entry, error) {
	ch := make(chan Entry)
	close(ch)
	return ch, nil
}

func (StubClient) Labels(_ context.Context, _, _ string) (Labels, error) {
	return Labels{}, nil
}
