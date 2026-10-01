package main

import (
	"os"
	"path/filepath"
	"testing"
)

// host.json goes into .semaps/, which ignores itself like the MCP log folder.
func TestHostFileFolderIgnoresItself(t *testing.T) {
	root := t.TempDir()
	s := &modelService{key: "k"}
	if err := s.hostFile(root, 1234); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".semaps", ".gitignore"))
	if err != nil || string(got) != "*\n" {
		t.Fatalf("no self-ignore beside host.json: %q, %v", got, err)
	}
}
