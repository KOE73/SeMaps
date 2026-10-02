package core

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteViewRemovesFileTextKeyAndDefault(t *testing.T) {
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p")
	text := `{"contractVersion":5,"language":"ru","entries":{"e_a":{"description":{"v":"A"}},"v_main":{"name":{"v":"Main"}},"e_b":{"description":{"v":"B"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "text.ru.json"), []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"id":"p","contractVersion":5,"defaultView":"v_main"}`), 0644); err != nil {
		t.Fatal(err)
	}
	entities, err := os.ReadFile(filepath.Join(dir, "entities.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteView(ws, "p", "v_main"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "views", "main.view.json")); !os.IsNotExist(err) {
		t.Fatalf("view file stays: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "text.ru.json"))
	if bytes.Contains(b, []byte("v_main")) || bytes.Index(b, []byte(`"e_a"`)) > bytes.Index(b, []byte(`"e_b"`)) {
		t.Fatalf("text: %s", b)
	}
	m, _ := os.ReadFile(filepath.Join(dir, "project.json"))
	if bytes.Contains(m, []byte("defaultView")) {
		t.Fatalf("defaultView stays: %s", m)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "entities.json"))
	if !bytes.Equal(entities, after) {
		t.Fatal("registry touched")
	}
	if err := DeleteView(ws, "p", "v_main"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	refused(t, DeleteView(ws, "p", "../x"), "invalid view id")
}

func TestDeleteViewKeepsOtherDefault(t *testing.T) {
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p")
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"id":"p","contractVersion":5,"defaultView":"v_other"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := DeleteView(ws, "p", "v_main"); err != nil {
		t.Fatal(err)
	}
	m, _ := os.ReadFile(filepath.Join(dir, "project.json"))
	if !bytes.Contains(m, []byte("v_other")) {
		t.Fatalf("manifest: %s", m)
	}
}

func TestStructuralRenamesKeepIdsAndTextPosition(t *testing.T) {
	ws := editWorkspace(t)
	dir := filepath.Join(ws, "projects", "p")
	text := `{"contractVersion":5,"language":"ru","entries":{"e_a":{"description":{"v":"A"}},"v_main":{"name":{"v":"Main"}},"e_b":{"description":{"v":"B"}}}}`
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
	m, err := LoadModel(ws, "renamed", defaultKindsJSON(t))
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
