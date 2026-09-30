package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"semaps/core"
)

// `semaps migrate` rewrites a contract-3 workspace in place; the result is
// loaded by the host's own model and passes `semaps check`; a second run does
// nothing; --dry-run writes nothing and says so with exit code 1.
func TestRunMigrate(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json":        `{"id":"p","title":"P","contractVersion":3,"defaultAxis":"axis_a","languages":["ru"]}`,
		"entities.json":       `{"entities":[{"id":"e_a","name":"A","kind":"class"}]}`,
		"relations.json":      `{"relations":[]}`,
		"relation-types.json": `{"relationTypes":[]}`,
		"text.ru.json":        `{"contractVersion":3,"language":"ru","entries":{"v_main":{"name":{"v":"Главный","at":"2026-09-24T00:00:00Z","origin":"authored"}},"z_g":{"name":{"v":"Группа","at":"2026-09-24T00:00:00Z","origin":"authored"}}}}`,
		"views/v.view.json": `{"id":"v_main","project":"p","axis":"axis_a",
			"zones":[{"id":"z_g","container":null,"parent":null,"x":0,"y":0,"width":300,"height":200,"styleId":"zone.red"}],
			"nodes":[{"entity":"e_a","zone":"z_g","x":20,"y":50}]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := core.LoadModel(ws, "p", defaultKinds()); err == nil || !strings.Contains(err.Error(), "semaps migrate") {
		t.Fatalf("the loader must name the command: %v", err)
	}

	var out bytes.Buffer
	if code := runMigrate(&out, ws, true); code != 1 || !strings.Contains(out.String(), "--dry-run") {
		t.Fatalf("dry run: %d\n%s", code, out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "project.json")); !strings.Contains(string(b), `"contractVersion":3`) {
		t.Fatalf("dry run wrote: %s", b)
	}

	out.Reset()
	if code := runMigrate(&out, ws, false); code != 0 {
		t.Fatalf("migrate: %d\n%s", code, out.String())
	}
	m, err := core.LoadModel(ws, "p", defaultKinds())
	if err != nil {
		t.Fatalf("the migrated project does not load: %v", err)
	}
	m.SetCanvas(loadCanvas(ws))
	info, err := m.GetView("v_main")
	if err != nil || len(info.Placements) != 1 || info.Placements[0].Name != "Группа" || !info.Placements[0].Container ||
		len(info.Placements[0].Children) != 1 || info.Placements[0].Children[0].Entity != "e_a" {
		t.Fatalf("view after migrate: %+v %v", info.Placements, err)
	}
	if findings := core.Check(ws, ws, defaultKinds()); len(findings) != 0 {
		t.Fatalf("check after migrate: %v", findings)
	}

	out.Reset()
	if code := runMigrate(&out, ws, false); code != 0 || !strings.Contains(out.String(), "Уже контракт v5") {
		t.Fatalf("second run: %d\n%s", code, out.String())
	}
}
