package controller

import (
	"sync"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// definitionCache memoises parsed PluginDefinitions by their content hash.
//
// A published definition is immutable and hash-pinned, so the "sha256:..." pin
// is a stable, unique key: a cache hit is byte-identical to a fresh fetch. This
// lets the level-triggered reconcile — which runs reconcileChildren on every
// poll to repair out-of-band drift — serve an unchanged definition from memory
// instead of re-fetching it from organization-api. Steady-state reconciliation
// therefore does not depend on organization-api being reachable.
//
// Only pinned installs populate the cache; unpinned dev installs have no stable
// hash and are always fetched fresh, preserving hot-reload. A republish lands
// under a new hash (a new key), so a stale manifest is never served. Live keys
// are bounded by the number of distinct pinned definitions the controller has
// fetched, so the map is left unbounded — each entry is a few KB.
type definitionCache struct {
	mu      sync.RWMutex
	entries map[string]*pluginruntime.PluginDefinition
}

func newDefinitionCache() *definitionCache {
	return &definitionCache{entries: make(map[string]*pluginruntime.PluginDefinition)}
}

func (c *definitionCache) get(hash string) (*pluginruntime.PluginDefinition, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	def, ok := c.entries[hash]
	return def, ok
}

func (c *definitionCache) put(hash string, def *pluginruntime.PluginDefinition) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[hash] = def
}
