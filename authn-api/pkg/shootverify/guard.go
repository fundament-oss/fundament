package shootverify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Guard defaults. A review is one round trip to a shoot API server; the
// admin-kubeconfig fetch behind a cache miss is bounded separately by the
// cache. Three consecutive unavailable results open a cluster's circuit for
// the cooldown, after which a single probe is let through at a time.
const (
	DefaultTimeout          = 5 * time.Second
	DefaultFailureThreshold = 3
	DefaultCooldown         = 30 * time.Second
)

// Guarded wraps a Verifier with a per-call timeout and a per-cluster circuit
// breaker so a dead or slow shoot cannot pin authn-api's request handlers
// (FUN-22, "Abuse of the exchange"). Only ErrUnavailable trips the breaker:
// a rejected token is a healthy cluster doing its job. The review runs
// detached from the caller's context, under our timeout alone: a caller that
// cancels or sends a short deadline cannot turn a healthy cluster's answer
// into a failure, or an anonymous caller could hold a victim cluster's
// circuit open by cutting its own requests short.
type Guarded struct {
	inner     Verifier
	timeout   time.Duration
	threshold int
	cooldown  time.Duration
	now       func() time.Time

	mu       sync.Mutex
	circuits map[string]*circuit
}

type circuit struct {
	failures  int
	openUntil time.Time
	probing   bool
}

// NewGuarded wraps inner with the default timeout, threshold and cooldown.
func NewGuarded(inner Verifier) *Guarded {
	return &Guarded{
		inner:     inner,
		timeout:   DefaultTimeout,
		threshold: DefaultFailureThreshold,
		cooldown:  DefaultCooldown,
		now:       time.Now,
		circuits:  map[string]*circuit{},
	}
}

// Verify implements Verifier.
func (g *Guarded) Verify(ctx context.Context, clusterID, token, audience string) (*Subject, error) {
	probe, err := g.admit(clusterID)
	if err != nil {
		return nil, err
	}

	// A probe whose review never returns (a panic unwinding to the recovery
	// interceptor) must still free the half-open slot, or every later call
	// for the cluster is refused as "probe in flight" until a restart.
	settled := false
	if probe {
		defer func() {
			if !settled {
				g.releaseProbe(clusterID)
			}
		}()
	}

	reviewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.timeout)
	defer cancel()

	subject, err := g.inner.Verify(reviewCtx, clusterID, token, audience)
	settled = true
	switch {
	case err == nil:
		g.record(clusterID, true)
	case errors.Is(err, context.DeadlineExceeded):
		g.record(clusterID, false)
		return nil, fmt.Errorf("%w: review timed out after %s: %w", ErrUnavailable, g.timeout, err)
	case errors.Is(err, ErrUnavailable):
		g.record(clusterID, false)
	default:
		// A definite answer from the cluster: the circuit is healthy.
		g.record(clusterID, true)
	}
	if err != nil {
		return nil, fmt.Errorf("guarded verify: %w", err)
	}
	return subject, nil
}

// admit refuses calls to an open circuit. Once the cooldown has passed it
// lets one probe through, reporting it as the probe, and refuses the rest
// until the probe's outcome is recorded.
func (g *Guarded) admit(clusterID string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.circuits[clusterID]
	if !ok || c.openUntil.IsZero() {
		return false, nil
	}
	if g.now().Before(c.openUntil) {
		return false, fmt.Errorf("%w: circuit open for cluster %s until %s", ErrUnavailable, clusterID, c.openUntil.Format(time.RFC3339))
	}
	if c.probing {
		return false, fmt.Errorf("%w: circuit half-open for cluster %s, probe in flight", ErrUnavailable, clusterID)
	}
	c.probing = true
	return true, nil
}

// releaseProbe frees the half-open slot without recording an outcome, so the
// next caller probes.
func (g *Guarded) releaseProbe(clusterID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.circuits[clusterID]; ok {
		c.probing = false
	}
}

func (g *Guarded) record(clusterID string, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if ok {
		delete(g.circuits, clusterID)
		return
	}
	c, found := g.circuits[clusterID]
	if !found {
		c = &circuit{}
		g.circuits[clusterID] = c
	}
	c.failures++
	c.probing = false
	if c.failures >= g.threshold {
		c.openUntil = g.now().Add(g.cooldown)
	}
}
