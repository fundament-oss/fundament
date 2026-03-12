package prometheus

import (
	"context"
	"time"
)

// StubClient is a no-op Client used when Prometheus is not configured.
// All methods return empty results without errors.
type StubClient struct{}

func (StubClient) Query(_ context.Context, _ string, _ time.Time) ([]Sample, error) {
	return nil, nil
}

func (StubClient) QueryRange(_ context.Context, _ string, _, _ time.Time, _ time.Duration) ([]TimeSeries, error) {
	return nil, nil
}
