package pluginsdk

import "time"

type runConfig struct {
	metadataPort    int
	shutdownTimeout time.Duration
}

var defaultRunConfig = runConfig{
	metadataPort:    8080,
	shutdownTimeout: 30 * time.Second,
}

// RunOption configures the Run harness.
type RunOption func(*runConfig)

// WithMetadataPort sets the port for the HTTP server hosting health probes,
// metadata API, and console assets.
func WithMetadataPort(port int) RunOption {
	return func(c *runConfig) {
		c.metadataPort = port
	}
}

// WithShutdownTimeout sets the maximum duration for graceful shutdown.
func WithShutdownTimeout(d time.Duration) RunOption {
	return func(c *runConfig) {
		c.shutdownTimeout = d
	}
}
