package core

import (
	"os"
	"path/filepath"
	"testing"
)

// The loader reads only contract 5: another project version and a view of the
// old shape are refused with the file, the field and the version named.
func TestLoaderRefusesTheOldShape(t *testing.T) {
	ws := editWorkspace(t)
	manifest := filepath.Join(ws, "projects", "p", "project.json")
	for body, want := range map[string]string{
		`{"id":"p","contractVersion":3}`: "project.json: contractVersion 3 — форма контракта 3, нужен 5 (`semaps migrate`, ADR_20260927-3)",
		`{"id":"p","contractVersion":6}`: "contractVersion 6",
		`{"id":"p"}`:                     "project.json: contractVersion is missing",
	} {
		writeFile(t, manifest, body)
		_, err := LoadModel(ws, "p", defaultKindsJSON(t))
		refused(t, err, want)
	}
	writeFile(t, manifest, `{"id":"p","contractVersion":5}`)
	containers := filepath.Join(ws, "projects", "p", "containers.json")
	writeFile(t, containers, `{"containers":[]}`)
	_, err := LoadModel(ws, "p", defaultKindsJSON(t))
	refused(t, err, "containers.json — контейнер это сущность")
	if err := os.Remove(containers); err != nil {
		t.Fatal(err)
	}
	// relation types are the dictionary's: a project that still has the file is named
	types := filepath.Join(ws, "projects", "p", RelationTypesFile)
	writeFile(t, types, `{"contractVersion":5,"relationTypes":[]}`)
	_, err = LoadModel(ws, "p", defaultKindsJSON(t))
	refused(t, err, "relation-types.json — тип связи описан в словаре")
	if err := os.Remove(types); err != nil {
		t.Fatal(err)
	}
	view := filepath.Join(ws, "projects", "p", "views", "main.view.json")
	for body, want := range map[string]string{
		`{"id":"v_main","zones":[],"placements":[]}`:                        "views/main.view.json: `zones` — форма контракта 3, нужен 5",
		`{"id":"v_main","nodes":[]}`:                                        "`nodes`",
		`{"id":"v_main","placements":[{"entity":"e_a","zone":null}]}`:       "placements[0]: `zone`",
		`{"id":"v_main","placements":[{"id":"e_a","x":0,"y":0}]}`:           "placements[0]: `id` вместо `entity`",
		`{"id":"v_main","placements":[{"entity":"e_a","container":"c_x"}]}`: "placements[0]: `container`",
	} {
		writeFile(t, view, body)
		m, err := LoadModel(ws, "p", defaultKindsJSON(t))
		if err != nil {
			t.Fatal(err)
		}
		_, err = m.View("v_main")
		refused(t, err, want)
		_, err = m.Apply([]Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":null,"x":0,"y":0}`)}, "human")
		refused(t, err, want)
	}
}

// One op kind for blocks and containers: placement, addressed by the entity;
// null removes it. zone and node are no op kinds any more.
func TestPlacementOps(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	refs, err := m.Apply([]Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":"e_core","x":40,"y":40}`)}, "human")
	if err != nil || len(refs) != 1 || refs[0].Kind != "placement" || refs[0].ID != "e_b" || refs[0].View != "v_main" {
		t.Fatalf("refs %+v, %v", refs, err)
	}
	doc, _ := m.view("v_main")
	items := viewItems(doc, "placements")
	if len(items) != 3 || items[2].str("entity") != "e_b" {
		t.Fatalf("a new placement is appended: %s", objString(doc))
	}
	// an edit keeps the place in the array
	if _, err := m.Apply([]Op{modelOp("placement", "e_core", "v_main", "", `{"entity":"e_core","parent":null,"x":10,"y":0,"width":500,"height":500}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	doc, _ = m.view("v_main")
	if items = viewItems(doc, "placements"); items[0].str("entity") != "e_core" || items[0].numOr("x", 0) != 10 {
		t.Fatalf("order changed: %s", objString(doc))
	}
	// a container with content cannot go alone
	_, err = m.Apply([]Op{modelOp("placement", "e_core", "v_main", "", `null`)}, "human")
	refused(t, err, "e_b on v_main still lies in e_core")
	if _, err := m.Apply([]Op{modelOp("placement", "e_b", "v_main", "", `null`), modelOp("placement", "e_core", "v_main", "", `null`)}, "human"); err != nil {
		t.Fatal(err)
	}
	doc, _ = m.view("v_main")
	if items = viewItems(doc, "placements"); len(items) != 1 {
		t.Fatalf("null removes: %s", objString(doc))
	}
	for _, kind := range []string{"zone", "node"} {
		_, err = m.Apply([]Op{modelOp(kind, "e_b", "v_main", "", `{"entity":"e_b","parent":null,"x":0,"y":0}`)}, "human")
		refused(t, err, "unknown operation kind")
	}
	for _, key := range []string{"placements", "zones", "nodes"} {
		_, err = m.Apply([]Op{modelOp("view", "v_main", "v_main", "", `{"id":"v_main","`+key+`":[]}`)}, "human")
		refused(t, err, "cannot carry `"+key+"`")
	}
	_, err = m.Apply([]Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_a","parent":null,"x":0,"y":0}`)}, "human")
	refused(t, err, "differs from operation id")
	// the project op keeps contractVersion when the editor leaves it out
	if _, err := m.Apply([]Op{modelOp("project", "p", "", "", `{"id":"p","title":"T"}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	if err := checkContractVersion(m.manifest); err != nil {
		t.Fatal(err)
	}
	_, err = m.Apply([]Op{modelOp("project", "p", "", "", `{"id":"p","contractVersion":3}`)}, "human")
	refused(t, err, "contractVersion 3")
}

func TestPlacementRules(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, op, want string }{
		{"e_b", `{"entity":"e_b","x":0,"y":0}`, "parent is required"},
		{"e_b", `{"entity":"e_b","parent":"e_zzz","x":0,"y":0}`, "no container e_zzz on v_main"},
		{"e_b", `{"entity":"e_b","parent":"e_a","x":0,"y":0}`, "e_a is not a container"},
		{"e_b", `{"entity":"e_b","parent":"e_core","override":{"radius":9}}`, `"radius" cannot be overridden on a placement`},
		{"e_nobody", `{"entity":"e_nobody","parent":null,"x":0,"y":0}`, "no entity e_nobody"},
		{"e_b", `{"entity":"e_b","parent":"e_core","zone":"z_x","x":0,"y":0}`, "`zone`"},
		{"e_b", `{"entity":"e_b","parent":"e_core","override":{"header":{"x":1}}}`, `"header.x" cannot be overridden`},
	} {
		_, err := m.Apply([]Op{modelOp("placement", tc.id, "v_main", "", tc.op)}, "human")
		refused(t, err, tc.want)
	}
	// a parent that is a block: place e_a in e_b
	if _, err := m.Apply([]Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":null,"x":0,"y":0}`)}, "human"); err != nil {
		t.Fatal(err)
	}
	_, err = m.Apply([]Op{modelOp("placement", "e_a", "v_main", "", `{"entity":"e_a","parent":"e_b","x":0,"y":0}`)}, "human")
	refused(t, err, "e_b is not a container")
	// an allowed override passes
	if _, err := m.Apply([]Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":null,"x":0,"y":0,"override":{"fill":"#fff","border":{"color":"#000","dash":"6,4"},"header":{"fill":"#eee"},"icon":{"glyph":"box"}}}`)}, "human"); err != nil {
		t.Fatal(err)
	}
}

func TestTextKeysOfContainersAreGone(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"c_core", "z_core"} {
		refused(t, m.SetText("ru", key, "name", "Ядро", "human"), "expected a prefix e_, r_ or v_")
	}
}
