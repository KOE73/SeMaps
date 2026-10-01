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
	writeFile(t, filepath.Join(proj, "relations.json"), `{"contractVersion":5,"relations":[
		{"id":"r_a_w_call","from":"e_a","to":"e_w","type":"call","origin":"authored"},
		{"id":"r_a_w_homemade","from":"e_a","to":"e_w","type":"homemade","origin":"authored"}]}`)
	writeFile(t, filepath.Join(proj, "relation-types.json"), `{"contractVersion":5,"relationTypes":[{"id":"call"}]}`)
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
		"edges":[{"id":"r_a_w_call","override":{"fill":"#f00"}},{"id":"r_ghost","routing":"bezier"},{"id":"r_a_w_call","routing":"bezier"},{"id":"r_a_w_homemade"}]}`)
	writeFile(t, filepath.Join(proj, "views", "v_old.view.json"), `{"id":"v_old","project":"p","zones":[],"nodes":[]}`)
	writeFile(t, filepath.Join(proj, "views", "v_oldedges.view.json"), `{"id":"v_oldedges","project":"p","axis":"axis_a","placements":[],
		"edges":[{"id":"r_a_w_call","from":"e_a","to":"e_w","type":"call"}]}`)
	name := `{"v":"%s","at":"2026-09-24T00:00:00Z","origin":"authored"}`
	writeFile(t, filepath.Join(proj, "text.ru.json"), `{"contractVersion":5,"language":"ru","entries":{
		"v_main":{"name":`+strings.Replace(name, "%s", "Главный", 1)+`},
		"v_bad":{"name":`+strings.Replace(name, "%s", "Плохой", 1)+`},
		"v_old":{"name":`+strings.Replace(name, "%s", "Старый", 1)+`},
		"v_oldedges":{"name":`+strings.Replace(name, "%s", "Старые линии", 1)+`},
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
		"p: views/v_oldedges.view.json: edges[0] (r_a_w_call): `from` — запись edges была полной копией связи",
		"text.ru.json: c_grp",
		"containers.json — контейнер это сущность",
		"relation-types.json — тип связи описан в словаре",
		"text.ru.json: rt_call — ключи rt_ упразднены",
	} {
		if !strings.Contains(byKind["форма контракта"], want) {
			t.Errorf("old shape: want %q in %q", want, byKind["форма контракта"])
		}
	}
	if got := byKind["тип не из словаря"]; got != `p: "weird": e_w, e_w2;` {
		t.Errorf("kinds outside the dictionary: %q", got)
	}
	if got := byKind["тип связи не из словаря"]; got != `p: "homemade": r_a_w_homemade;` {
		t.Errorf("relation types outside the dictionary: %q", got)
	}
	got := byKind["размещение"]
	for _, want := range []string{`v_bad.view.json: e_a: override: "shape"`, "e_w: parent e_a", "e_ghost: нет такой сущности", "e_ghost: нет поля parent", `связь r_a_w_call: override: "fill"`} {
		if !strings.Contains(got, want) {
			t.Errorf("placements: want %q in %q", want, got)
		}
	}
	lines := byKind["линия вида"]
	for _, want := range []string{"v_bad.view.json: r_ghost — такой связи нет", "v_bad.view.json: r_a_w_call записана дважды", "v_bad.view.json: r_a_w_homemade — записи нечего хранить"} {
		if !strings.Contains(lines, want) {
			t.Errorf("edge entries: want %q in %q", want, lines)
		}
	}
	if strings.Contains(lines, "v_main") {
		t.Errorf("a clean view reported: %q", lines)
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

// A broken code[].ref is reported, one of an external symbol (no ref) is not;
// a clean workspace is clean.
func TestCheckCodeRefs(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(proj, "project.json"), `{"id":"p","title":"P","contractVersion":5}`)
	writeFile(t, filepath.Join(proj, "entities.json"), `{"contractVersion":5,"entities":[
		{"id":"e_a","name":"A","kind":"class","code":[{"lang":"go","ref":"src/a.go#L3","symbol":"a.A"}]},
		{"id":"e_gone","name":"Gone","kind":"class","code":[{"ref":"src/Gone.cs"}]},
		{"id":"e_ext","name":"Writer","kind":"external","code":[{"lang":"go","symbol":"io.Writer"}]}]}`)
	writeFile(t, filepath.Join(ws, "src", "a.go"), "package a\n")
	if b := checkByKind(t, ws)["битая ссылка на код"]; b != "p: e_gone: src/Gone.cs;" {
		t.Errorf("broken refs: %q", b)
	}
}

// The shape of ADR_20260930-4/5 is what check reads: a top-level codeRef,
// symbol or via, an authored entity's name in entities.json and a malformed
// code[] entry are named; an authored entity without a name text is a gap only
// when no language has one.
func TestCheckCodeShapeAndAuthoredNames(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(proj, "project.json"), `{"id":"p","title":"P","contractVersion":5,"languages":["ru","en"]}`)
	writeFile(t, filepath.Join(proj, "entities.json"), `{"contractVersion":5,"entities":[
		{"id":"e_old","name":"Old","kind":"class","symbol":"N.Old","codeRef":"src/Old.cs"},
		{"id":"e_named","name":"Named in the file","kind":"group","origin":"authored"},
		{"id":"e_ru","kind":"group","origin":"authored"},
		{"id":"e_en","kind":"group","origin":"authored"},
		{"id":"e_none","kind":"group","origin":"authored"},
		{"id":"e_dead","name":"Dead","kind":"class","origin":"code"},
		{"id":"e_half","name":"Half","kind":"class","code":[{"lang":"go","ref":"a.go"},{"ref":"a.go","symbol":"a.A"}]},
		{"id":"e_twice","name":"Twice","kind":"class","code":[{"lang":"go","symbol":"a.A"},{"lang":"go","symbol":"a.B"}]}]}`)
	writeFile(t, filepath.Join(proj, "relations.json"), `{"contractVersion":5,"relations":[
		{"id":"r_old","from":"e_ru","to":"e_en","type":"holds.one","via":{"member":"x"},"evidence":[{"codeRef":"a.go"}]}]}`)
	name := `{"name":{"v":"%s","at":"2026-09-24T00:00:00Z","origin":"authored"}}`
	writeFile(t, filepath.Join(proj, "text.ru.json"), `{"contractVersion":5,"language":"ru","entries":{"e_ru":`+strings.Replace(name, "%s", "Русское", 1)+`,"e_dead":`+strings.Replace(name, "%s", "Мёртвое", 1)+`}}`)
	writeFile(t, filepath.Join(proj, "text.en.json"), `{"contractVersion":5,"language":"en","entries":{"e_en":`+strings.Replace(name, "%s", "English", 1)+`}}`)
	byKind := checkByKind(t, ws)
	for _, want := range []string{
		"entities.json: e_old: `codeRef` верхнего уровня",
		"entities.json: e_named: `name` у authored-сущности",
		"relations.json: r_old: `via` верхнего уровня",
	} {
		if !strings.Contains(byKind["форма контракта"], want) {
			t.Errorf("old shape: want %q in %q", want, byKind["форма контракта"])
		}
	}
	if got := byKind["недостача"]; !strings.Contains(got, "e_none: у нарисованной сущности нет имени ни в одном языке") ||
		strings.Contains(got, "e_ru") || strings.Contains(got, "e_en") {
		t.Errorf("a name in one language is enough, none in any is a gap: %q", got)
	}
	if got := byKind["лишнее имя"]; !strings.Contains(got, "e_dead@ru") || strings.Contains(got, "e_ru") {
		t.Errorf("a name text under the id of an entity from code is dead: %q", got)
	}
	got := byKind["реализация"]
	// the old relation has no valid evidence entry either; e_old in the old shape is named above, not here
	for _, want := range []string{"e_half: code[] entry needs lang and symbol together", "e_twice: two code[] entries of language go"} {
		if !strings.Contains(got, want) {
			t.Errorf("code[]: want %q in %q", want, got)
		}
	}
}
