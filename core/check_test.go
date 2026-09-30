package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func checkByKind(t *testing.T, ws string) map[string]string {
	t.Helper()
	byKind := map[string]string{}
	for _, f := range Check(ws, ws, defaultKindsJSON(t)) {
		byKind[f.Kind] += f.Project + ": " + f.Message + ";"
	}
	return byKind
}

// A container is an entity: named in entities.json, no text key of its own.
// A view of the old shape, a project of another version, a kind outside the
// dictionary, a parent that is no container and an override outside the table
// are reported; a clean view is not.
func TestCheckContainersKindsAndShape(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(proj, "project.json"),
		`{"id":"p","title":"P","contractVersion":5,"defaultAxis":"axis_a","languages":["ru"]}`)
	writeFile(t, filepath.Join(proj, "entities.json"), `{"contractVersion":5,"entities":[
		{"id":"e_grp","name":"Group","kind":"group"},
		{"id":"e_a","name":"A","kind":"class"},
		{"id":"e_w","name":"W","kind":"weird"},
		{"id":"e_w2","name":"W2","kind":"weird"}]}`)
	writeFile(t, filepath.Join(proj, "relations.json"), `{"contractVersion":5,"relations":[]}`)
	writeFile(t, filepath.Join(proj, "relation-types.json"), `{"contractVersion":5,"relationTypes":[{"id":"call"},{"id":"homemade","styleId":"edge.homemade"}]}`)
	writeFile(t, filepath.Join(proj, "containers.json"), `{"contractVersion":3,"containers":[]}`)
	writeFile(t, filepath.Join(proj, "views", "v_main.view.json"), `{
		"id":"v_main","project":"p","axis":"axis_a",
		"placements":[
			{"entity":"e_grp","parent":null,"x":0,"y":0,"width":400,"height":300},
			{"entity":"e_a","parent":"e_grp","x":20,"y":40,"override":{"border":{"color":"#f00"}}}
		]}`)
	writeFile(t, filepath.Join(proj, "views", "v_bad.view.json"), `{
		"id":"v_bad","project":"p","axis":"axis_a",
		"placements":[
			{"entity":"e_a","parent":null,"x":0,"y":0,"override":{"shape":"ellipse"}},
			{"entity":"e_w","parent":"e_a","x":0,"y":0},
			{"entity":"e_ghost","x":0,"y":0}
		],
		"edges":[{"id":"r_x","from":"e_a","to":"e_w","type":"call","override":{"fill":"#f00"}}]}`)
	writeFile(t, filepath.Join(proj, "views", "v_old.view.json"), `{"id":"v_old","project":"p","zones":[],"nodes":[]}`)
	name := `{"v":"%s","at":"2026-09-24T00:00:00Z","origin":"authored"}`
	writeFile(t, filepath.Join(proj, "text.ru.json"), `{"contractVersion":5,"language":"ru","entries":{
		"v_main":{"name":`+strings.Replace(name, "%s", "Главный", 1)+`},
		"v_bad":{"name":`+strings.Replace(name, "%s", "Плохой", 1)+`},
		"v_old":{"name":`+strings.Replace(name, "%s", "Старый", 1)+`},
		"rt_call":{"name":`+strings.Replace(name, "%s", "вызов", 1)+`},
		"rt_homemade":{"name":`+strings.Replace(name, "%s", "своя", 1)+`},
		"c_grp":{"name":`+strings.Replace(name, "%s", "Подпись контейнера", 1)+`}
	}}`)
	old := filepath.Join(ws, "projects", "q")
	writeFile(t, filepath.Join(old, "project.json"), `{"id":"q","title":"Q","contractVersion":3}`)

	byKind := checkByKind(t, ws)
	if byKind["недостача"] != "" {
		t.Errorf("a container needs no text name: %q", byKind["недостача"])
	}
	for _, want := range []string{
		"q: project.json: contractVersion 3 — форма контракта 3, нужен 5 (`semaps migrate`, ADR_20260927-3)",
		"p: views/v_old.view.json: `zones` — форма контракта 3, нужен 5",
		"text.ru.json: c_grp",
		"containers.json — контейнер это сущность",
		"relation-types.json: homemade: `styleId`",
	} {
		if !strings.Contains(byKind["форма контракта"], want) {
			t.Errorf("old shape: want %q in %q", want, byKind["форма контракта"])
		}
	}
	if got := byKind["тип не из словаря"]; got != `p: "weird": e_w, e_w2;` {
		t.Errorf("kinds outside the dictionary: %q", got)
	}
	if got := byKind["тип связи не из словаря"]; !strings.Contains(got, `"homemade"`) || strings.Contains(got, `"call"`) {
		t.Errorf("relation types outside the dictionary: %q", got)
	}
	got := byKind["размещение"]
	for _, want := range []string{`v_bad.view.json: e_a: override: "shape"`, "e_w: parent e_a", "e_ghost: нет такой сущности", "e_ghost: нет поля parent", `связь r_x: override: "fill"`} {
		if !strings.Contains(got, want) {
			t.Errorf("placements: want %q in %q", want, got)
		}
	}
	if strings.Contains(got, "v_main") {
		t.Errorf("a clean view reported: %q", got)
	}
}

// The workspace files the contract changed: canvas.json (`zone` became
// `container`) and styles.json (`kinds` became `forKinds`, mandatory).
func TestCheckWorkspaceFiles(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "canvas.json"), `{"grid":10,"zone":{"minWidth":1},"gap":{"node":1,"zone":1}}`)
	writeFile(t, filepath.Join(ws, "styles.json"), `{"styles":[
		{"id":"default.node","appliesTo":"block"},
		{"id":"default.container","appliesTo":"container"},
		{"id":"default.edge","appliesTo":"edge"},
		{"id":"class","appliesTo":"block","forKinds":["class"]},
		{"id":"old","appliesTo":"block","kinds":["class"]},
		{"id":"typeless","appliesTo":"block"},
		{"id":"typeless.edge","appliesTo":"edge"},
		{"id":"weird","appliesTo":"block","forKinds":["nonsense"]},
		{"id":"frame","appliesTo":"block","forKinds":["group"]},
		{"id":"box","appliesTo":"container","forKinds":["class"]},
		{"id":"extends","appliesTo":"edge","forKinds":["extends"]},
		{"id":"wire","appliesTo":"edge","forKinds":["nonsense"]}]}`)
	byKind := checkByKind(t, ws)
	if !strings.Contains(byKind["форма контракта"], "canvas.json: `zone`") || !strings.Contains(byKind["форма контракта"], "old: `kinds`") {
		t.Errorf("old shape: %q", byKind["форма контракта"])
	}
	typeless := byKind["без типа"]
	for _, id := range []string{"old", "typeless", "typeless.edge"} {
		if !strings.Contains(typeless, "styles.json: "+id+" ") {
			t.Errorf("%s is without a type: %q", id, typeless)
		}
	}
	for _, id := range []string{"default.node", "default.container", "default.edge", "class", "extends"} {
		if strings.Contains(typeless, "styles.json: "+id+" ") {
			t.Errorf("%s must not be reported: %q", id, typeless)
		}
	}
	unknown := byKind["тип не из словаря"]
	if !strings.Contains(unknown, `weird: forKinds "nonsense"`) || !strings.Contains(unknown, `wire: forKinds "nonsense"`) {
		t.Errorf("unknown forKinds: %q", unknown)
	}
	mismatch := byKind["стиль и тип"]
	if !strings.Contains(mismatch, `frame: appliesTo "block", а тип "group" контейнер`) || !strings.Contains(mismatch, `box: appliesTo "container", а тип "class" не контейнер`) {
		t.Errorf("appliesTo against the kind: %q", mismatch)
	}
}

// A bare string instead of a value with provenance is the shape of contract 2:
// nobody reads it (CONTRACT §7), check names the file and the field.
func TestCheckBareTextStringIsTheOldShape(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(proj, "project.json"), `{"id":"p","title":"P","contractVersion":5,"languages":["ru"]}`)
	writeFile(t, filepath.Join(proj, "entities.json"), `{"contractVersion":5,"entities":[{"id":"e_a","name":"A","kind":"class"}]}`)
	writeFile(t, filepath.Join(proj, "text.ru.json"), `{"contractVersion":5,"language":"ru","entries":{"e_a":{"description":"голая строка"}}}`)
	if got := checkByKind(t, ws)["форма контракта"]; !strings.Contains(got, "text.ru.json: e_a.description — строка вместо значения с происхождением") {
		t.Errorf("bare string: %q", got)
	}
}

// A broken codeRef is reported; a clean workspace is clean.
func TestCheckCodeRefs(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(proj, "project.json"), `{"id":"p","title":"P","contractVersion":5}`)
	writeFile(t, filepath.Join(proj, "entities.json"), `{"contractVersion":5,"entities":[
		{"id":"e_a","name":"A","kind":"class","codeRef":"src/a.go#L3"},
		{"id":"e_gone","name":"Gone","kind":"class","codeRef":"src/Gone.cs"}]}`)
	writeFile(t, filepath.Join(ws, "src", "a.go"), "package a\n")
	if b := checkByKind(t, ws)["битый codeRef"]; b != "p: e_gone: src/Gone.cs;" {
		t.Errorf("broken refs: %q", b)
	}
}
