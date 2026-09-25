package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The log of `semaps mcp` calls: one JSON line per tool call in
// <project root>/.semaps/logs/mcp-<date>.jsonl, and a short line on stderr,
// which MCP clients keep in their server logs. What an agent asked, how long
// it took and what went wrong — for finding out why a client refused an
// answer or a tool misbehaved. The folder ignores itself in git.

const mcpLogKeepDays = 14

type mcpLogEntry struct {
	At      string          `json:"at"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args,omitempty"`
	Ms      int64           `json:"ms"`
	IsError bool            `json:"isError,omitempty"`
	Error   string          `json:"error,omitempty"` // a tool error's text, or a protocol error
}

var mcpLogMu sync.Mutex

// callLog is a receiving middleware that records every tools/call. root is
// the folder of the .semaps file; empty — no file log. echo gets one line per
// call; nil — none.
func callLog(root string, echo io.Writer) mcp.Middleware {
	dir := ""
	if root != "" {
		dir = filepath.Join(root, ".semaps", "logs")
		pruneLogs(dir)
	}
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if method != "tools/call" || !ok {
				return next(ctx, method, req)
			}
			start := time.Now()
			res, err := next(ctx, method, req)
			e := mcpLogEntry{At: start.UTC().Format(time.RFC3339), Tool: call.Params.Name, Ms: time.Since(start).Milliseconds()}
			if raw := call.Params.Arguments; len(raw) > 0 && string(raw) != "{}" && string(raw) != "null" {
				e.Args = raw
			}
			switch r, _ := res.(*mcp.CallToolResult); {
			case err != nil:
				e.IsError, e.Error = true, err.Error()
			case r != nil && r.IsError:
				e.IsError, e.Error = true, toolText(r)
			}
			if echo != nil {
				status := "ok"
				if e.IsError {
					status = "ERROR " + e.Error
				}
				fmt.Fprintf(echo, "semaps mcp: %s %dms %s\n", e.Tool, e.Ms, status)
			}
			if dir != "" {
				writeLog(dir, start, e)
			}
			return res, err
		}
	}
}

func toolText(r *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

func writeLog(dir string, at time.Time, e mcpLogEntry) {
	mcpLogMu.Lock()
	defer mcpLogMu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	// .semaps/ is machine state: keep it out of git without touching the repo's .gitignore.
	ignore := filepath.Join(filepath.Dir(dir), ".gitignore")
	if _, err := os.Stat(ignore); err != nil {
		_ = os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
	f, err := os.OpenFile(filepath.Join(dir, "mcp-"+at.Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	line, _ := json.Marshal(e)
	_, _ = f.Write(append(line, '\n'))
}

// pruneLogs drops call logs older than mcpLogKeepDays.
func pruneLogs(dir string) {
	files, _ := filepath.Glob(filepath.Join(dir, "mcp-*.jsonl"))
	cut := time.Now().AddDate(0, 0, -mcpLogKeepDays).Format("2006-01-02")
	for _, f := range files {
		day := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "mcp-"), ".jsonl")
		if day < cut {
			_ = os.Remove(f)
		}
	}
}
