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
	if code := runMigrate(&out, ws, true, false, nil); code != 1 || !strings.Contains(out.String(), "--dry-run") {
		t.Fatalf("dry run: %d\n%s", code, out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "project.json")); !strings.Contains(string(b), `"contractVersion":3`) {
		t.Fatalf("dry run wrote: %s", b)
	}

	out.Reset()
	if code := runMigrate(&out, ws, false, false, nil); code != 0 {
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
	if code := runMigrate(&out, ws, false, false, nil); code != 0 || !strings.Contains(out.String(), "Уже контракт v5") {
		t.Fatalf("second run: %d\n%s", code, out.String())
	}
}

// A workspace of the earlier form of contract 5 (a `codeRef` + `symbol`, the name of an authored
// entity in entities.json) is brought to the current shape, the language taken from the
// extractors of the .semaps file; --drop-untyped-styles also drops the styles that name no type
// (the shipped library then applies: styles.json is removed). Without a language the run asks.
func TestRunMigrateEarlierFormOfV5(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"styles.json":         `{"styles":[{"id":"class","appliesTo":"block"},{"id":"mine","appliesTo":"block","fill":"#fff"}]}`,
		"project.json":        `{"id":"p","title":"P","contractVersion":5,"defaultAxis":"axis_a","languages":["ru"]}`,
		"entities.json":       `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","codeRef":"src/A.cs","symbol":"N.A"},{"id":"e_g","name":"Группа","kind":"group","origin":"authored"}]}`,
		"relations.json":      `{"relations":[]}`,
		"relation-types.json": `{"relationTypes":[]}`,
		"text.ru.json":        `{"contractVersion":5,"language":"ru","entries":{"v_main":{"name":{"v":"Главный","at":"2026-09-24T00:00:00Z","origin":"authored"}}}}`,
		"views/v.view.json": `{"id":"v_main","project":"p","axis":"axis_a","placements":[
			{"entity":"e_g","parent":null,"x":0,"y":0,"width":300,"height":200},
			{"entity":"e_a","parent":"e_g","x":20,"y":50,"styleId":"mine"}]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if name == "styles.json" {
			p = filepath.Join(ws, name)
		}
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
	if code := runMigrate(&out, ws, false, true, nil); code != 1 {
		t.Fatalf("without a language the run must stop: %d\n%s", code, out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "entities.json")); !strings.Contains(string(b), `"symbol":"N.A"`) {
		t.Fatalf("a stopped run wrote: %s", b)
	}
	out.Reset()
	if code := runMigrate(&out, ws, false, true, []extractorConf{{ID: "cs", Language: "csharp", Project: "p"}}); code != 0 {
		t.Fatalf("migrate: %d\n%s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(ws, "styles.json")); err == nil {
		t.Error("styles.json must be removed: `class` is a shipped id and `mine` is dropped on request")
	}
	m, err := core.LoadModel(ws, "p", defaultKinds())
	if err != nil {
		t.Fatalf("the migrated project does not load: %v", err)
	}
	if got := m.EntityName("e_g"); got != "Группа" {
		t.Errorf("name of the authored entity: %q", got)
	}
	if !strings.Contains(out.String(), "«mine» — 1: v_main: размещение e_a") {
		t.Errorf("the report must say which placement lost its style:\n%s", out.String())
	}
	if findings := core.Check(ws, ws, defaultKinds()); len(findings) != 1 || findings[0].Kind != "битая ссылка на код" {
		t.Fatalf("check after migrate (only the source file of the demo is missing): %v", findings)
	}
	out.Reset()
	if code := runMigrate(&out, ws, false, true, nil); code != 0 || !strings.Contains(out.String(), "Уже контракт v5") {
		t.Fatalf("second run: %d\n%s", code, out.String())
	}
}
