package core

import (
	"os"
	"path/filepath"
	"testing"
)

// The work journal lives in <workspace>/.semaps/work; writing it makes .semaps
// ignore itself, and an existing .gitignore there is left as it is.
func TestJournalFolderIgnoresItself(t *testing.T) {
	ws := editWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	entity := `{"id":"e_a","name":"A edited","kind":"class","origin":"code","code":[{"lang":"csharp","symbol":"N.A"}]}`
	if _, err := m.Apply([]Op{modelOp("entity", "e_a", "", "", entity)}, "human"); err != nil {
		t.Fatal(err)
	}
	ignore := filepath.Join(ws, ".semaps", ".gitignore")
	got, err := os.ReadFile(ignore)
	if err != nil || string(got) != "*\n" {
		t.Fatalf("no self-ignore in .semaps: %q, %v", got, err)
	}
	if err := os.WriteFile(ignore, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	EnsureSelfIgnore(filepath.Join(ws, ".semaps"))
	if got, _ := os.ReadFile(ignore); string(got) != "custom\n" {
		t.Fatalf("an existing .gitignore was overwritten: %q", got)
	}
}
