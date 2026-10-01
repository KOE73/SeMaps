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
	// onChange, when set (registerMCPHTTP, PLAN_20260928-7 step 4), is
	// called after every Set with the new settings, outside the lock: the
	// MCP server uses it to rebuild its graph tool list and notify
	// connected clients when tools/description changed. nil in tests that
	// only care about format/list_cap/limit (step 2).
	onChange func(mcpSettings)
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
	onChange := b.onChange
	cur := b.v
	b.mu.Unlock()
	if onChange != nil {
		onChange(cur)
	}
}

// OnChange installs the settings-changed callback (registerMCPHTTP calls
// this once, right after building the server, so the callback can close
// over it).
func (b *mcpSettingsBox) OnChange(f func(mcpSettings)) {
	b.mu.Lock()
	b.onChange = f
	b.mu.Unlock()
}
