package core

import (
	"os"
	"path/filepath"
)

// EnsureSelfIgnore makes a `.semaps/` state folder (host key, MCP logs, the
// work journal) keep itself out of git: it writes a `.gitignore` holding `*`
// into the folder when there is none, so the repository's own .gitignore is
// never touched. Every writer of a `.semaps/` folder calls it after creating
// the folder. Best effort: a failure only means the folder is not ignored.
func EnsureSelfIgnore(stateDir string) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return
	}
	ignore := filepath.Join(stateDir, ".gitignore")
	if _, err := os.Stat(ignore); err != nil {
		_ = os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
}
