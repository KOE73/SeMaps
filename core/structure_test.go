package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStructuralRenamesKeepIdsAndTextPosition(t *testing.T) {
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p")
	text := `{"contractVersion":3,"language":"ru","entries":{"e_a":{"description":{"v":"A"}},"v_main":{"name":{"v":"Main"}},"e_b":{"description":{"v":"B"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "text.ru.json"), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RenameView(ws, "p", "v_main", "v_other"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "text.ru.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(b, []byte(`"e_a"`)) > bytes.Index(b, []byte(`"v_other"`)) || bytes.Index(b, []byte(`"v_other"`)) > bytes.Index(b, []byte(`"e_b"`)) {
		t.Fatalf("text key moved: %s", b)
	}
	if err := RenameProject(ws, "p", "renamed"); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(ws, "renamed")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(m.Manifest(), []byte(`"id":"renamed"`)) {
		t.Fatalf("manifest: %s", m.Manifest())
	}
	view, err := m.View("v_other")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(view, []byte(`"project":"renamed"`)) {
		t.Fatalf("view: %s", view)
	}
}
