package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Extractors are separate programs with one command line (EXTRACTOR.md §1):
// `--root <dir> [--include <path>]... [--exclude <glob>]...`, facts on stdout.
// semaps.exe only finds and calls them (ADR_20260924-3 §1).

// knownLanguages are the extractors SeMaps ships. Another language works
// through `command` in .semaps or `semaps-extract-<language>` on PATH.
var knownLanguages = []string{"csharp", "typescript"}

// runtimeInfo says whether the runtime an extractor needs is there.
type runtimeInfo struct {
	Name    string `json:"name"` // "dotnet SDK", "node"
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"` // what was found
	Hint    string `json:"hint,omitempty"`    // how to get it, when missing
}

// extractorTool is how an extractor of one entry gets started.
type extractorTool struct {
	Language string       `json:"language"`
	Found    bool         `json:"found"`
	Source   string       `json:"source,omitempty"` // command | bundled | path
	Where    string       `json:"where,omitempty"`  // shown to the human; the program or the command
	Runtime  *runtimeInfo `json:"runtime,omitempty"`
	Problem  string       `json:"problem,omitempty"`
	argv     []string
}

// ready: found and its runtime, if it needs one, is there.
func (t extractorTool) ready() bool { return t.Found && (t.Runtime == nil || t.Runtime.OK) }

// toolsDir is `extractors/` beside the running semaps.exe.
func toolsDir() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	if s, err := filepath.EvalSymlinks(self); err == nil {
		self = s
	}
	return filepath.Join(filepath.Dir(self), "extractors")
}

// resolveExtractor finds the program for an entry: its own `command`, then
// the one shipped beside semaps.exe, then `semaps-extract-<language>` on PATH.
func resolveExtractor(e extractorConf) extractorTool {
	t := extractorTool{Language: e.Language}
	if e.Command != "" {
		argv, err := splitCommand(e.Command)
		if err != nil || len(argv) == 0 {
			t.Problem = fmt.Sprintf("command: %v", err)
			return t
		}
		t.Source, t.Where = "command", e.Command
		if _, err := exec.LookPath(argv[0]); err != nil {
			t.Problem = fmt.Sprintf("%s: not found", argv[0])
			return t
		}
		t.Found, t.argv = true, argv
		t.Runtime = runtimeFor(e.Language)
		return t
	}
	if argv, where, ok := shippedExtractor(e.Language); ok {
		t.Found, t.Source, t.Where, t.argv = true, "bundled", where, argv
		t.Runtime = runtimeFor(e.Language)
		return t
	}
	name := "semaps-extract-" + e.Language
	if p, err := exec.LookPath(name); err == nil {
		t.Found, t.Source, t.Where, t.argv = true, "path", p, []string{p}
		t.Runtime = runtimeFor(e.Language)
		return t
	}
	t.Problem = fmt.Sprintf("no extractor for %q: not in %s, no %s on PATH, no `command` in .semaps", e.Language, toolsDir(), name)
	return t
}

func shippedExtractor(language string) (argv []string, where string, ok bool) {
	dir := filepath.Join(toolsDir(), language)
	switch language {
	case "csharp":
		exe := filepath.Join(dir, "semaps-extract-csharp")
		if runtime.GOOS == "windows" {
			exe += ".exe"
		}
		if fileExists(exe) {
			return []string{exe}, exe, true
		}
		if dll := filepath.Join(dir, "semaps-extract-csharp.dll"); fileExists(dll) {
			return []string{"dotnet", dll}, dll, true
		}
	case "typescript":
		if cli := filepath.Join(dir, "dist", "cli.js"); fileExists(cli) {
			return []string{"node", cli}, cli, true
		}
	}
	return nil, "", false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

var (
	runtimeMu    sync.Mutex
	runtimeCache = map[string]*runtimeInfo{}
)

// runtimeFor checks the runtime a language's extractor needs; nil for one it
// knows nothing about. Asked once per process.
func runtimeFor(language string) *runtimeInfo {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if r, ok := runtimeCache[language]; ok {
		return r
	}
	var r *runtimeInfo
	switch language {
	case "csharp":
		r = &runtimeInfo{Name: "dotnet SDK", Hint: "install the .NET 10 SDK: https://dotnet.microsoft.com/download"}
		if out, err := probe("dotnet", "--list-sdks"); err == nil && strings.TrimSpace(out) != "" {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			r.OK, r.Version, r.Hint = true, strings.Fields(lines[len(lines)-1])[0], ""
		}
	case "typescript":
		r = &runtimeInfo{Name: "node", Hint: "install Node.js 24 or newer: https://nodejs.org"}
		if out, err := probe("node", "--version"); err == nil {
			r.Version = strings.TrimSpace(out)
			var major int
			fmt.Sscanf(strings.TrimPrefix(r.Version, "v"), "%d", &major)
			if major >= 24 {
				r.OK, r.Hint = true, ""
			} else {
				r.Hint = "Node.js 24 or newer is needed: https://nodejs.org"
			}
		}
	}
	runtimeCache[language] = r
	return r
}

func probe(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// splitCommand splits a command line into words: spaces separate, double or
// single quotes group. No shell: nothing else is interpreted.
func splitCommand(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

// extractorArgs is the command line for one entry, `root` resolved against
// the project root.
func extractorArgs(t extractorTool, e extractorConf, projectRoot string) []string {
	args := append([]string{}, t.argv...)
	args = append(args, "--root", filepath.Join(projectRoot, filepath.FromSlash(e.Root)))
	for _, p := range e.Include {
		args = append(args, "--include", p)
	}
	for _, g := range e.Exclude {
		args = append(args, "--exclude", g)
	}
	if len(e.Edges) > 0 {
		// Join edge kinds with commas, no spaces (ADR_20260924-4 §5)
		var edges strings.Builder
		for i, edge := range e.Edges {
			if i > 0 {
				edges.WriteString(",")
			}
			edges.WriteString(edge)
		}
		args = append(args, "--edges", edges.String())
	}
	return args
}
