package main

import "sync"

// mcpSettingsBox holds the `mcp:` section of the .semaps file in memory,
// shared by the HTTP graph endpoint, the MCP tool and the tool API
// (PLAN_20260928-7 step 2): PUT /api/setup writes the file, then calls Set
// here, so the running host uses the new format/list_cap/limit from the very
// next call on, in both places, without a restart. Read with Get, which
// copies out the (small, value-typed) struct under the lock.
type mcpSettingsBox struct {
	mu sync.RWMutex
	v  mcpSettings
}

func newMcpSettingsBox(v mcpSettings) *mcpSettingsBox {
	return &mcpSettingsBox{v: v.withDefaults()}
}

func (b *mcpSettingsBox) Get() mcpSettings {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.v
}

func (b *mcpSettingsBox) Set(v mcpSettings) {
	b.mu.Lock()
	b.v = v.withDefaults()
	b.mu.Unlock()
}
