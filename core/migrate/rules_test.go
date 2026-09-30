package migrate

import (
	"reflect"
	"strings"
	"testing"
)

func TestEntityIDs(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/entities.json": `{"entities":[{"id":"e_llm_core","name":"x","kind":"class"}]}`,
		"projects/p/views/v1.view.json": `{"id":"v1","project":"p","zones":[
			{"id":"z_LLM Core!","x":0,"y":0},{"id":"z_llm_core","x":1,"y":1},
			{"id":"z_Ядро (core)","x":2,"y":2},{"id":"z_---","x":3,"y":3}],"nodes":[]}`,
		"projects/p/views/v2.view.json": `{"id":"v2","project":"p","zones":[{"id":"z_llm_core","x":5,"y":5}],"nodes":[]}`,
	}))
	rep := run(t, ws, Options{})
	got := entityIDs(t, ws, "p")
	want := []string{"e_llm_core", "e_llm_core_2", "e_llm_core_3", "e_ядро_core", "e_group"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entities %v, want %v", got, want)
	}
	// The same zone id on two views is one entity.
	v2 := readTree(t, ws, "projects/p/views/v2.view.json")
	if e := str(t, v2, "placements", 0, "entity"); e != "e_llm_core_3" {
		t.Errorf("v2 zone entity %s", e)
	}
	if rep.Projects[0].Zones != 5 || rep.Projects[0].ZoneEntities != 4 {
		t.Errorf("counts %+v", *rep.Projects[0])
	}
	if got := js(t, v2, "placements"); got != `[{"entity":"e_llm_core_3","parent":null,"x":5,"y":5}]` {
		t.Errorf("v2 placements %s", got)
	}
	// Names fall back to the id tail, and are reported.
	if n := str(t, entityByID(t, ws, "p", "e_llm_core_2"), "name"); n != "LLM Core!" {
		t.Errorf("name %q", n)
	}
	if len(rep.Projects[0].NoName) != 4 {
		t.Errorf("no-name %v", rep.Projects[0].NoName)
	}
}

func TestNameChoiceAcrossLanguages(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/project.json": `{"id":"p","title":"P","contractVersion":3,"languages":["en","ru"]}`,
		"projects/p/text.ru.json": `{"contractVersion":3,"language":"ru","entries":{"z_x":{"name":{"v":"Икс","at":"t","origin":"authored"}},"z_y":{"name":{"v":"Игрек","at":"t","origin":"authored"}}}}`,
		"projects/p/text.en.json": `{"contractVersion":3,"language":"en","entries":{"c_x":{"name":{"v":"X","at":"t","origin":"authored"}},"c_y":{"name":{"v":"Игрек","at":"t","origin":"authored"}}}}`,
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[
			{"id":"z_x","container":"c_x","x":0,"y":0},
			{"id":"z_y","container":null,"x":0,"y":0}],"nodes":[]}`,
	}))
	rep := run(t, ws, Options{})
	// z_x: en is first in languages, and the c_ key of the container holds the name.
	if n := str(t, entityByID(t, ws, "p", "e_x"), "name"); n != "X" {
		t.Errorf("name of e_x %q", n)
	}
	// z_y: no en text under z_y; the fallback key c_y (tail of z_) is searched too.
	if n := str(t, entityByID(t, ws, "p", "e_y"), "name"); n != "Игрек" {
		t.Errorf("name of e_y %q", n)
	}
	p := rep.Projects[0]
	if len(p.LostNames) != 1 || !strings.Contains(p.LostNames[0], "ru z_x: «Икс»") {
		t.Errorf("lost names %v", p.LostNames)
	}
	if len(p.NoName) != 0 {
		t.Errorf("no-name %v", p.NoName)
	}
}

func TestTextsMovedAndKeysRemoved(t *testing.T) {
	prov := func(v string) string { return `{"v":"` + v + `","at":"2026-01-01T00:00:00Z","origin":"authored"}` }
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/text.ru.json": `{"contractVersion":3,"language":"ru","entries":{
			"z_grp":{"name":` + prov("Группа") + `,"description":` + prov("Z-описание") + `},
			"c_grp":{"description":` + prov("C-описание") + `,"doc":` + prov("C-док") + `},
			"e_grp":{"description":` + prov("E-описание") + `},
			"c_other":{"name":` + prov("Чужой") + `},
			"e_a":{"description":` + prov("A") + `},
			"r_x":{"name":` + prov("связь") + `}}}`,
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[{"id":"z_grp","container":"c_grp","x":0,"y":0}],"nodes":[]}`,
	}))
	rep := run(t, ws, Options{})
	p := rep.Projects[0]
	text := readTree(t, ws, "projects/p/text.ru.json")
	keys := keysOf(t, text, "entries")
	if !reflect.DeepEqual(keys, []string{"e_grp", "e_a", "r_x"}) {
		t.Fatalf("entries %v", keys)
	}
	// The existing entity text wins, the missing field comes from the zone, then the container.
	if got := str(t, text, "entries", "e_grp", "description", "v"); got != "E-описание" {
		t.Errorf("description %q", got)
	}
	if got := str(t, text, "entries", "e_grp", "doc", "v"); got != "C-док" {
		t.Errorf("doc %q", got)
	}
	if p.TextsMoved != 1 || p.TextsRemoved != 3 {
		t.Errorf("moved/removed %d/%d", p.TextsMoved, p.TextsRemoved)
	}
	if len(p.LostText) != 2 {
		t.Errorf("lost text %v", p.LostText)
	}
	if len(p.UnconsumedText) != 1 || !strings.Contains(p.UnconsumedText[0], "c_other «Чужой»") {
		t.Errorf("unconsumed %v", p.UnconsumedText)
	}
	// The entity was minted from the container.
	if n := str(t, entityByID(t, ws, "p", "e_grp"), "name"); n != "Группа" {
		t.Errorf("name %q", n)
	}
}

func TestLegacySpellingsAndPlacementsKey(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/views/v.view.json": `{"id":"v","project":"p","placements":[
			{"id":"e_a","container":"g1","x":1,"y":2},
			{"id":"e_b","container":"nope","x":3,"y":4},
			{"entity":"e_c","zone":null,"container":"g1"},
			{"entity":"e_d","id":"ignored","zone":"g1","x":7}],
			"zones":[{"id":"g1","x":0,"y":0,"width":9,"height":9}],"tail":true}`,
	}))
	rep := run(t, ws, Options{})
	v := readTree(t, ws, "projects/p/views/v.view.json")
	if got := strings.Join(v.keys, ","); got != "id,project,placements,tail" {
		t.Errorf("keys %s", got)
	}
	want := `[{"entity":"e_g1","parent":null,"x":0,"y":0,"width":9,"height":9},` +
		`{"entity":"e_a","parent":"e_g1","x":1,"y":2},` +
		`{"entity":"e_b","parent":null,"x":3,"y":4},` +
		`{"entity":"e_c","parent":null},` +
		`{"entity":"e_d","parent":"e_g1","x":7}]`
	if got := js(t, v, "placements"); got != want {
		t.Errorf("placements\n got %s\nwant %s", got, want)
	}
	if len(rep.Projects[0].UnknownZones) != 1 || !strings.Contains(rep.Projects[0].UnknownZones[0], "«nope»") {
		t.Errorf("unknown zones %v", rep.Projects[0].UnknownZones)
	}
}

func TestNodesAndPlacementsBothPresent(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[],"nodes":[{"entity":"e_a"}],"placements":[{"id":"e_b"}]}`,
	}))
	run(t, ws, Options{})
	v := readTree(t, ws, "projects/p/views/v.view.json")
	if got := js(t, v, "placements"); got != `[{"entity":"e_a","parent":null},{"entity":"e_b","parent":null}]` {
		t.Errorf("placements %s", got)
	}
}

func TestZoneNesting(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[
			{"id":"z_a","x":0,"y":0,"width":100,"height":100},
			{"id":"z_b","parent":"z_a","x":10,"y":10,"width":20,"height":20},
			{"id":"z_c","parent":"z_missing","x":10,"y":50,"width":20,"height":20},
			{"id":"z_d","container":"z_a","x":40,"y":10,"width":20,"height":20},
			{"id":"z_e","parent":null,"x":11,"y":11,"width":5,"height":5}],"nodes":[]}`,
	}))
	rep := run(t, ws, Options{})
	v := readTree(t, ws, "projects/p/views/v.view.json")
	parents := []string{}
	for i := 0; i < 5; i++ {
		p := at(t, v, "placements", i, "parent")
		s, _ := asStr(p)
		parents = append(parents, s)
	}
	// z_e lies inside z_a by geometry, but nesting is never inferred.
	// (e_a is taken by an existing entity, so the zone z_a is e_a_2.)
	if !reflect.DeepEqual(parents, []string{"", "e_a_2", "", "e_a_2", ""}) {
		t.Errorf("parents %v", parents)
	}
	if len(rep.Projects[0].UnknownZones) != 1 || !strings.Contains(rep.Projects[0].UnknownZones[0], "zones[2].parent") {
		t.Errorf("unknown zones %v", rep.Projects[0].UnknownZones)
	}
	// z_d had `container` naming a zone: its own entity, not a container link.
	if n := len(entityIDs(t, ws, "p")); n != 6 {
		t.Errorf("entities %d", n)
	}
	if has(at(t, v, "placements", 3), "container") {
		t.Error("container key must be dropped")
	}
}

const containersFixture = `{"contractVersion":3,"containers":[
	{"id":"c_p","parent":null},{"id":"c_c","parent":"c_p"}],"overrides":{"e_a":"c_c"}}`

func nestedView(extra string) string {
	return `{"id":"v","project":"p",` + extra + `"zones":[
		{"id":"z1","container":"c_p","x":0,"y":0,"width":9,"height":9},
		{"id":"z2","container":"c_c","parent":"z1","x":1,"y":1,"width":5,"height":5}],
		"nodes":[{"entity":"e_a","zone":"z2","x":2,"y":2}]}`
}

func TestContainsRelationsAndExceptions(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/containers.json":     containersFixture,
		"projects/p/relation-types.json": `{"contractVersion":3,"relationTypes":[{"id":"contains","origin":"code"}]}`,
		"projects/p/relations.json":      `{"relations":[{"id":"r_p_c_contains","from":"e_a","to":"e_a","type":"uses"}]}`,
		"projects/p/views/v_a.view.json": strings.Replace(nestedView(""), `"id":"v"`, `"id":"v_a"`, 1),
		"projects/p/views/v_b.view.json": strings.Replace(nestedView(`"edges":[],`), `"id":"v"`, `"id":"v_b"`, 1),
		"projects/p/views/v_c.view.json": strings.Replace(nestedView(`"relations":{"default":"hidden"},`), `"id":"v"`, `"id":"v_c"`, 1),
		"projects/p/views/v_d.view.json": `{"id":"v_d","project":"p","zones":[{"id":"z1","container":"c_p","x":0,"y":0}],"nodes":[{"entity":"e_a","zone":"z1"}]}`,
		"projects/p/views/v_e.view.json": strings.Replace(nestedView(`"relations":{"default":"visible","except":["r_x"]},`), `"id":"v"`, `"id":"v_e"`, 1),
		"projects/p/views/v_f.view.json": strings.Replace(nestedView(`"relations":{"default":"visible"},`), `"id":"v"`, `"id":"v_f"`, 1),
	}))
	rep := run(t, ws, Options{})
	p := rep.Projects[0]
	if p.ContainsAdded != 2 {
		t.Fatalf("contains added %d", p.ContainsAdded)
	}
	rels := readTree(t, ws, "projects/p/relations.json")
	if got := js(t, rels, "relations", 1); got != `{"id":"r_p_c_contains_2","from":"e_p","to":"e_c","type":"contains","origin":"authored"}` {
		t.Errorf("relation 1: %s", got)
	}
	if got := js(t, rels, "relations", 2); got != `{"id":"r_c_a_contains","from":"e_c","to":"e_a","type":"contains","origin":"authored"}` {
		t.Errorf("relation 2: %s", got)
	}
	// An existing `contains` type stays as it is.
	rt := readTree(t, ws, "projects/p/relation-types.json")
	if arrLen(t, rt, "relationTypes") != 1 {
		t.Errorf("relation types %s", js(t, rt))
	}
	except := `["r_p_c_contains_2","r_c_a_contains"]`
	v := readTree(t, ws, "projects/p/views/v_a.view.json")
	if got := strings.Join(v.keys, ","); got != "id,project,relations,placements" {
		t.Errorf("v_a keys %s", got)
	}
	if got := js(t, v, "relations"); got != `{"default":"visible","except":`+except+`}` {
		t.Errorf("v_a relations %s", got)
	}
	if has(readTree(t, ws, "projects/p/views/v_b.view.json"), "relations") {
		t.Error("a view with its own edges must not get relations")
	}
	if got := js(t, readTree(t, ws, "projects/p/views/v_c.view.json"), "relations"); got != `{"default":"hidden"}` {
		t.Errorf("hidden view: %s", got)
	}
	if has(readTree(t, ws, "projects/p/views/v_d.view.json"), "relations") {
		t.Error("a view that places one end only must not get relations")
	}
	if got := js(t, readTree(t, ws, "projects/p/views/v_e.view.json"), "relations"); got != `{"default":"visible","except":["r_x","r_p_c_contains_2","r_c_a_contains"]}` {
		t.Errorf("v_e relations %s", got)
	}
	if got := js(t, readTree(t, ws, "projects/p/views/v_f.view.json"), "relations"); got != `{"default":"visible","except":`+except+`}` {
		t.Errorf("v_f relations %s", got)
	}
	// Second run: nothing.
	if run(t, ws, Options{}).Changed() {
		t.Error("not idempotent")
	}
}

func TestContainsTypeVisibility(t *testing.T) {
	mk := func(rt string) string {
		return mkws(t, baseProject(map[string]string{
			"projects/p/containers.json":     containersFixture,
			"projects/p/relation-types.json": rt,
			"projects/p/views/v.view.json":   nestedView(`"relations":{"default":"hidden"},`),
		}))
	}
	// A hidden type: no exception even if the view says nothing.
	ws := mk(`{"relationTypes":[{"id":"contains","origin":"code","visibility":"hidden"}]}`)
	run(t, ws, Options{})
	if got := js(t, readTree(t, ws, "projects/p/views/v.view.json"), "relations"); got != `{"default":"hidden"}` {
		t.Errorf("hidden type: %s", got)
	}
	// A visible type beats the view default.
	ws = mk(`{"relationTypes":[{"id":"contains","origin":"code","visibility":"visible"}]}`)
	run(t, ws, Options{})
	if got := js(t, readTree(t, ws, "projects/p/views/v.view.json"), "relations", "except"); got != `["r_p_c_contains","r_c_a_contains"]` {
		t.Errorf("visible type: %s", got)
	}
}

func TestContainsTypeAndTextAdded(t *testing.T) {
	fixedNow(t)
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/project.json":      `{"id":"p","title":"P","contractVersion":3,"languages":["ru","en","de"]}`,
		"projects/p/containers.json":   containersFixture,
		"projects/p/text.en.json":      `{"contractVersion":3,"language":"en","entries":{}}`,
		"projects/p/text.de.json":      `{"contractVersion":3,"language":"de","entries":{}}`,
		"projects/p/views/v.view.json": nestedView(""),
	}))
	rep := run(t, ws, Options{})
	rt := readTree(t, ws, "projects/p/relation-types.json")
	if got := js(t, rt, "relationTypes", 0); got != `{"id":"contains","origin":"authored","visibility":"hidden"}` {
		t.Errorf("type %s", got)
	}
	ru := readTree(t, ws, "projects/p/text.ru.json")
	if got := js(t, ru, "entries", "rt_contains", "name"); got != `{"v":"содержит","at":"2026-09-30T12:00:00Z","origin":"authored"}` {
		t.Errorf("ru %s", got)
	}
	en := readTree(t, ws, "projects/p/text.en.json")
	if got := str(t, en, "entries", "rt_contains", "name", "v"); got != "contains" {
		t.Errorf("en %s", got)
	}
	de := readTree(t, ws, "projects/p/text.de.json")
	if has(at(t, de, "entries"), "rt_contains") {
		t.Error("de must get no text")
	}
	if !anyContains(rep.Projects[0].Notes, "text.de.json") {
		t.Errorf("notes %v", rep.Projects[0].Notes)
	}
	// The hidden type keeps the views alone.
	if has(readTree(t, ws, "projects/p/views/v.view.json"), "relations") {
		t.Error("hidden type: no exception expected")
	}
}

func TestContainerWithoutZoneAndMissingParent(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/containers.json": `{"containers":[{"id":"c_lone","parent":"c_ghost"},{"id":"c_lone"},{"id":"c_ok","parent":null,"theme":"green"}]}`,
	}))
	rep := run(t, ws, Options{})
	p := rep.Projects[0]
	if got := entityIDs(t, ws, "p"); !reflect.DeepEqual(got, []string{"e_a", "e_lone", "e_ok"}) {
		t.Errorf("entities %v", got)
	}
	if p.ContainerEntities != 2 || p.ContainsAdded != 0 {
		t.Errorf("counts %+v", *p)
	}
	if !anyContains(p.Notes, "c_ghost") || !anyContains(p.Notes, "повторён") {
		t.Errorf("notes %v", p.Notes)
	}
	if len(p.MatchRules) != 1 || !strings.Contains(p.MatchRules[0], `theme "green"`) {
		t.Errorf("match rules %v", p.MatchRules)
	}
	if exists(ws, "projects/p/containers.json") {
		t.Error("containers.json remains")
	}
	// No contains relation was made, so relations.json only gets the version.
	if got := js(t, readTree(t, ws, "projects/p/relations.json"), "relations"); got != `[]` {
		t.Errorf("relations %s", got)
	}
}

func TestOverrideResolution(t *testing.T) {
	grad := `{"kind":"linear","angle":90,"stops":[{"offset":0,"color":"#111111"},{"offset":1,"color":"#222222"}]}`
	ws := mkws(t, baseProject(map[string]string{
		"styles.json": `{"version":1,"styles":[
			{"id":"zone.custom","appliesTo":"block","basedOn":"default.container","fill":` + grad + `,"border":{"color":"#111","dash":"1,1","width":3}},
			{"id":"zone.child","appliesTo":"block","basedOn":"zone.custom","border":{"color":"#222"}},
			{"id":"paint","basedOn":"default.zone","fill":"#abcdef"},
			{"id":"mystyle","fill":"#ffffff"},
			{"id":"zone.blue","appliesTo":"block","fill":"#010101"}]}`,
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[
			{"id":"z1","x":0,"y":0,"styleId":"zone.child"},
			{"id":"z2","x":0,"y":0,"styleId":"boundary"},
			{"id":"z3","x":0,"y":0,"styleId":"paint"},
			{"id":"z4","x":0,"y":0,"styleId":"mystyle"},
			{"id":"z5","x":0,"y":0,"styleId":"zone.blue"},
			{"id":"z6","x":0,"y":0,"styleId":null},
			{"id":"z7","x":0,"y":0}],"nodes":[{"entity":"e_a","zone":"z1","styleId":"zone.blue"}]}`,
	}))
	rep := run(t, ws, Options{})
	v := readTree(t, ws, "projects/p/views/v.view.json")
	// Nearer wins; missing links fall to the built-in default.container.
	if got := js(t, v, "placements", 0, "override"); got != `{"fill":`+grad+`,"border":{"color":"#222","dash":"1,1"},"header":{"fill":"#e2e8f0"}}` {
		t.Errorf("z1: %s", got)
	}
	// boundary is not in the workspace: built-in table.
	if got := js(t, v, "placements", 1, "override"); got != `{"fill":"#f8fafc","border":{"color":"#cbd5e1"},"header":{"fill":"#e2e8f0"}}` {
		t.Errorf("z2: %s", got)
	}
	// A custom name that reaches default.zone is a colour style too.
	if got := js(t, v, "placements", 2, "override", "fill"); got != `"#abcdef"` {
		t.Errorf("z3: %s", got)
	}
	// Not a colour style: kept.
	if got := js(t, v, "placements", 3, "styleId"); got != `"mystyle"` {
		t.Errorf("z4: %s", got)
	}
	// The workspace definition beats the built-in one.
	if got := js(t, v, "placements", 4, "override"); got != `{"fill":"#010101"}` {
		t.Errorf("z5: %s", got)
	}
	if got := js(t, v, "placements", 5, "styleId"); got != `null` {
		t.Errorf("z6: %s", got)
	}
	if has(at(t, v, "placements", 6), "styleId") || has(at(t, v, "placements", 6), "override") {
		t.Error("z7 must stay bare")
	}
	if got := js(t, v, "placements", 7, "styleId"); got != `"zone.blue"` {
		t.Errorf("node style is kept: %s", got)
	}
	if rep.Projects[0].Overrides != 4 || len(rep.Projects[0].KeptStyleIDs) != 1 {
		t.Errorf("overrides %d kept %v", rep.Projects[0].Overrides, rep.Projects[0].KeptStyleIDs)
	}
}

func TestOverrideFromBuiltinTable(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[
			{"id":"z1","styleId":"zone.red.dashed"},{"id":"z2","styleId":"subsystem"},{"id":"z3","styleId":"zone.gray"}],"nodes":[]}`,
	}))
	run(t, ws, Options{})
	v := readTree(t, ws, "projects/p/views/v.view.json")
	if got := js(t, v, "placements", 0, "override"); got != `{"fill":"#fff1f2","border":{"color":"#fca5a5","dash":"6,4"},"header":{"fill":"#ffe4e6"}}` {
		t.Errorf("dashed: %s", got)
	}
	if got := js(t, v, "placements", 1, "override"); got != `{"fill":"#eff6ff","border":{"color":"#93c5fd"},"header":{"fill":"#dbeafe"}}` {
		t.Errorf("subsystem: %s", got)
	}
	if got := js(t, v, "placements", 2, "override", "fill"); got != `"#f1f5f9"` {
		t.Errorf("gray: %s", got)
	}
}

func TestStylesFile(t *testing.T) {
	ws := mkws(t, map[string]string{
		"styles.json": `{"version":1,"styles":[
			{"id":"default.zone","appliesTo":"block","kinds":["k"]},
			{"id":"z1","basedOn":"default.zone"},
			{"id":"z2","basedOn":"zone.blue","appliesTo":"block"},
			{"id":"e1","appliesTo":"edge","kinds":["a"]},
			{"id":"blk","appliesTo":"block","fill":"#fff"},
			{"id":"bare"},
			{"id":"e2","appliesTo":"edge"},
			{"id":"default.node","appliesTo":"block"},
			{"id":"default.edge","appliesTo":"edge"},
			{"id":"container.x"},
			{"id":"kept","appliesTo":"block","forKinds":["q"],"kinds":["old"]}]}`,
	})
	rep := run(t, ws, Options{})
	s := readTree(t, ws, "styles.json")
	if got := js(t, s, "styles", 0); got != `{"id":"default.container","appliesTo":"container","forKinds":["k"]}` {
		t.Errorf("default: %s", got)
	}
	if got := js(t, s, "styles", 1); got != `{"id":"z1","basedOn":"default.container","appliesTo":"container"}` {
		t.Errorf("z1: %s", got)
	}
	if got := js(t, s, "styles", 2, "appliesTo"); got != `"container"` {
		t.Errorf("z2 (reaches default.zone via the built-in zone.blue): %s", got)
	}
	if got := js(t, s, "styles", 3); got != `{"id":"e1","appliesTo":"edge","forKinds":["a"]}` {
		t.Errorf("edge kinds: %s", got)
	}
	if got := js(t, s, "styles", 4, "appliesTo"); got != `"block"` {
		t.Errorf("plain block style: %s", got)
	}
	if got := js(t, s, "styles", 9); got != `{"id":"container.x","appliesTo":"container"}` {
		t.Errorf("container.x: %s", got)
	}
	if got := js(t, s, "styles", 10); got != `{"id":"kept","appliesTo":"block","forKinds":["q"]}` {
		t.Errorf("both kinds and forKinds: %s", got)
	}
	if !reflect.DeepEqual(rep.StylesNoKind, []string{"z1", "z2", "blk", "bare", "container.x"}) ||
		!reflect.DeepEqual(rep.StylesNoKindEdge, []string{"e2"}) {
		t.Errorf("without kinds: %v / %v", rep.StylesNoKind, rep.StylesNoKindEdge)
	}
	if run(t, ws, Options{}).Changed() {
		t.Error("styles.json: second run must find nothing")
	}
}

func TestStylesFileKeepsExistingDefaultContainer(t *testing.T) {
	ws := mkws(t, map[string]string{
		"styles.json": `{"styles":[
			{"id":"default.zone","appliesTo":"block"},
			{"id":"default.container","appliesTo":"container"},
			{"id":"z","basedOn":"default.zone","kinds":["x"]}]}`,
	})
	run(t, ws, Options{})
	s := readTree(t, ws, "styles.json")
	if got := js(t, s, "styles"); got != `[{"id":"default.zone","appliesTo":"block"},{"id":"default.container","appliesTo":"container"},{"id":"z","basedOn":"default.zone","forKinds":["x"]}]` {
		t.Errorf("styles %s", got)
	}
}

func TestStylesFileNothingToDo(t *testing.T) {
	src := `{"styles":[{"id":"default.container","appliesTo":"container","forKinds":["g"]},{"id":"class","appliesTo":"block","forKinds":["class"]}]}`
	ws := mkws(t, map[string]string{"styles.json": src})
	if run(t, ws, Options{}).Changed() {
		t.Error("a v5 styles.json must not change")
	}
	if snapshot(t, ws)["styles.json"] != src {
		t.Error("styles.json rewritten")
	}
}

func TestCanvasFile(t *testing.T) {
	ws := mkws(t, map[string]string{
		"canvas.json": `{"grid":10,"zone":{"minWidth":160},"gap":{"zone":40,"node":5}}`,
	})
	rep := run(t, ws, Options{})
	c := readTree(t, ws, "canvas.json")
	if got := js(t, c); got != `{"grid":10,"container":{"minWidth":160},"gap":{"container":40,"node":5}}` {
		t.Errorf("canvas %s", got)
	}
	if !contains(rep.Files, "canvas.json") {
		t.Errorf("files %v", rep.Files)
	}
	if run(t, ws, Options{}).Changed() {
		t.Error("second run must find nothing")
	}
	// Both spellings present: left alone.
	ws = mkws(t, map[string]string{"canvas.json": `{"zone":{"a":1},"container":{"b":2}}`})
	if run(t, ws, Options{}).Changed() {
		t.Error("nothing to rename when both keys exist")
	}
}

func TestVersionGate(t *testing.T) {
	// v5 project: untouched even with old-shaped files around.
	v5 := map[string]string{
		"projects/q/project.json":      `{"id":"q","title":"Q","contractVersion":5}`,
		"projects/q/containers.json":   `{"containers":[{"id":"c_x"}]}`,
		"projects/q/views/v.view.json": `{"id":"v","project":"q","zones":[{"id":"z_x"}],"nodes":[]}`,
	}
	ws := mkws(t, v5)
	before := snapshot(t, ws)
	rep := run(t, ws, Options{})
	if rep.Changed() || printed(rep) != "Уже контракт v5, менять нечего.\n" {
		t.Errorf("v5 project: %q", printed(rep))
	}
	if !rep.Projects[0].Skipped {
		t.Error("project not marked as skipped")
	}
	if !reflect.DeepEqual(before, snapshot(t, ws)) {
		t.Error("v5 project changed")
	}

	// Newer than v5: an error naming the file and the field.
	ws = mkws(t, map[string]string{"projects/q/project.json": `{"id":"q","contractVersion":6}`})
	if _, err := Workspace(ws, Options{}); err == nil || !strings.Contains(err.Error(), "projects/q/project.json: contractVersion") {
		t.Errorf("v6: %v", err)
	}

	// Mixed workspace: the v5 project is reported as such.
	files := baseProject(nil)
	for k, v := range v5 {
		files[k] = v
	}
	files["projects/p/views/v.view.json"] = `{"id":"v","project":"p","zones":[{"id":"z_g"}],"nodes":[]}`
	ws = mkws(t, files)
	rep = run(t, ws, Options{})
	out := printed(rep)
	if !strings.Contains(out, "Проект q: уже v5") || !strings.Contains(out, "Проект p\n") {
		t.Errorf("report:\n%s", out)
	}
	if snapshot(t, ws)["projects/q/containers.json"] == "" {
		t.Error("containers.json of the v5 project deleted")
	}

	// No contractVersion at all: migrated, the key is added.
	ws = mkws(t, baseProject(map[string]string{"projects/p/project.json": `{"id":"p","title":"P"}`}))
	run(t, ws, Options{})
	if got := js(t, readTree(t, ws, "projects/p/project.json"), "contractVersion"); got != "5" {
		t.Errorf("missing version: %s", got)
	}
}

func TestProjectWithoutZones(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/views/v.view.json": `{"id":"v","project":"p","axis":"axis_x"}`,
	}))
	before := snapshot(t, ws)
	rep := run(t, ws, Options{})
	after := snapshot(t, ws)
	changed := []string{}
	for k := range after {
		if after[k] != before[k] {
			changed = append(changed, k)
		}
	}
	// Only files that carry a contractVersion change, textually.
	if len(changed) != 2 {
		t.Errorf("changed %v", changed)
	}
	if after["projects/p/project.json"] != `{"id":"p","title":"P","contractVersion":5,"languages":["ru"]}` {
		t.Errorf("project.json: %s", after["projects/p/project.json"])
	}
	if after["projects/p/views/v.view.json"] != before["projects/p/views/v.view.json"] {
		t.Error("a view without zones or nodes must stay as it is")
	}
	if p := rep.Projects[0]; p.Zones != 0 || p.Placements != 0 {
		t.Errorf("counts %+v", *p)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"malformed JSON", map[string]string{"projects/p/entities.json": `{"entities": [`},
			"projects/p/entities.json: неверный JSON"},
		{"container of a zone is a number", map[string]string{"projects/p/views/v.view.json": `{"zones":[{"id":"z_a","container":5}]}`},
			"projects/p/views/v.view.json: zones[0].container"},
		{"parent of a zone is an object", map[string]string{"projects/p/views/v.view.json": `{"zones":[{"id":"z_a"},{"id":"z_b","parent":{}}]}`},
			"zones[1].parent"},
		{"zone without id", map[string]string{"projects/p/views/v.view.json": `{"zones":[{"id":"z_a"},{"x":1}]}`},
			"projects/p/views/v.view.json: zones[1].id"},
		{"duplicate zone", map[string]string{"projects/p/views/v.view.json": `{"zones":[{"id":"z_a"},{"id":"z_a"}]}`},
			"zones[1].id"},
		{"node without entity", map[string]string{"projects/p/views/v.view.json": `{"zones":[],"nodes":[{"zone":"z"}]}`},
			"nodes[0].entity"},
		{"zones not an array", map[string]string{"projects/p/views/v.view.json": `{"zones":{}}`},
			"projects/p/views/v.view.json: zones"},
		{"version is a string", map[string]string{"projects/p/project.json": `{"id":"p","contractVersion":"three"}`},
			"projects/p/project.json: contractVersion"},
		{"overrides value", map[string]string{"projects/p/containers.json": `{"containers":[],"overrides":{"e_a":5}}`},
			"projects/p/containers.json: overrides.e_a"},
		{"container id", map[string]string{"projects/p/containers.json": `{"containers":[{"parent":null}]}`},
			"projects/p/containers.json: containers[0].id"},
		{"entries not an object", map[string]string{"projects/p/text.ru.json": `{"entries":[]}`},
			"projects/p/text.ru.json: entries"},
		{"entities not an array", map[string]string{"projects/p/entities.json": `{"entities":{}}`},
			"projects/p/entities.json: entities"},
		{"styles.json", map[string]string{"styles.json": `{"styles":[1]}`},
			"styles.json: styles[0]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := baseProject(c.files)
			// A healthy second project that would be migrated must stay untouched.
			files["projects/z/project.json"] = `{"id":"z","contractVersion":3}`
			files["projects/z/views/v.view.json"] = `{"id":"v","project":"z","zones":[{"id":"z_ok"}],"nodes":[]}`
			ws := mkws(t, files)
			before := snapshot(t, ws)
			rep, err := Workspace(ws, Options{})
			if err == nil {
				t.Fatal("no error")
			}
			if rep != nil && rep.Changed() {
				t.Errorf("report of a failed run says changed")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
			if !reflect.DeepEqual(before, snapshot(t, ws)) {
				t.Error("files were written despite the error")
			}
		})
	}
}

func TestBumpVersionText(t *testing.T) {
	cases := []struct {
		in, out string
		changed bool
	}{
		{`{"contractVersion":3}`, `{"contractVersion":5}`, true},
		{"{ \"a\": {\"contractVersion\": 9},\r\n  \"contractVersion\" : 3 , \"b\":1}\r\n", "{ \"a\": {\"contractVersion\": 9},\r\n  \"contractVersion\" : 5 , \"b\":1}\r\n", true},
		{"\xef\xbb\xbf{\"contractVersion\": 4}", "\xef\xbb\xbf{\"contractVersion\": 5}", true},
		{`{"a":1}`, `{"a":1}`, false},
		{`{"contractVersion": 5, "x": 3}`, `{"contractVersion": 5, "x": 3}`, false},
		{`{"language":"ru","contractVersion":12}`, `{"language":"ru","contractVersion":5}`, true},
	}
	for _, c := range cases {
		out, changed, err := bumpVersionText([]byte(c.in), 5)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != c.out || changed != c.changed {
			t.Errorf("in %q: out %q changed %v", c.in, out, changed)
		}
	}
	if _, _, err := bumpVersionText([]byte(`{"a":`), 5); err == nil {
		t.Error("broken JSON must fail")
	}
}

func TestTreeRoundTrip(t *testing.T) {
	in := "{\"a\":1.50,\"b\":\"<&>\",\"c\":[],\"d\":{},\"e\":\"\\u00e9\\n\",\"f\":[1,{\"g\":null}],\"a2\":1e3}"
	v, err := parseTree([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := encodeTree(v)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1.50,\n  \"b\": \"<&>\",\n  \"c\": [],\n  \"d\": {},\n  \"e\": \"\\u00e9\\n\",\n  \"f\": [\n    1,\n    {\n      \"g\": null\n    }\n  ],\n  \"a2\": 1e3\n}\n"
	if string(out) != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
	if _, err := parseTree([]byte(`{"a":1,}`)); err == nil {
		t.Error("trailing comma must fail")
	}
}

func TestObjOps(t *testing.T) {
	o := newObj()
	o.set("a", jsonInt(1))
	o.set("b", jsonInt(2))
	o.set("c", jsonInt(3))
	if !o.rename("b", "x") || strings.Join(o.keys, ",") != "a,x,c" {
		t.Errorf("rename: %v", o.keys)
	}
	if o.rename("a", "c") || o.rename("nope", "z") {
		t.Error("rename must refuse a taken or absent key")
	}
	o.insertBefore("c", "n", jsonInt(9))
	o.insertBefore("missing", "m", jsonInt(8))
	if strings.Join(o.keys, ",") != "a,x,n,c,m" {
		t.Errorf("insertBefore: %v", o.keys)
	}
	o.del("x")
	o.del("absent")
	if strings.Join(o.keys, ",") != "a,n,c,m" || o.has("x") {
		t.Errorf("del: %v", o.keys)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"llm_middleware": "llm_middleware", "LLM Core!": "llm_core", "  a--b  ": "a_b",
		"Ядро": "ядро", "": "group", "---": "group", "a1_b2": "a1_b2",
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMissingRegistryFilesAreCreated(t *testing.T) {
	ws := mkws(t, map[string]string{
		"projects/p/project.json":      `{"id":"p","contractVersion":3}`,
		"projects/p/views/v.view.json": `{"id":"v","project":"p","zones":[{"id":"z_g"}],"nodes":[]}`,
	})
	run(t, ws, Options{})
	ents := readTree(t, ws, "projects/p/entities.json")
	if got := js(t, ents); got != `{"contractVersion":5,"entities":[{"id":"e_g","name":"g","kind":"group","origin":"authored","status":"present"}]}` {
		t.Errorf("entities.json: %s", got)
	}
	if exists(ws, "projects/p/relations.json") {
		t.Error("relations.json created without need")
	}
}

// A relation type has no style of its own in the project (ADR_20260930-2): the
// old `styleId` is dropped and named in the report for a human.
func TestRelationTypeStyleIdIsDropped(t *testing.T) {
	ws := mkws(t, baseProject(map[string]string{
		"projects/p/relation-types.json": `{"relationTypes":[{"id":"security","origin":"authored","styleId":"edge.security"},{"id":"call","origin":"authored"}]}`,
	}))
	rep := run(t, ws, Options{})
	got := js(t, readTree(t, ws, "projects/p/relation-types.json"))
	if got != `{"relationTypes":[{"id":"security","origin":"authored"},{"id":"call","origin":"authored"}]}` {
		t.Errorf("relation-types.json: %s", got)
	}
	if out := printed(rep); !strings.Contains(out, `тип связи security: styleId "edge.security" снят`) {
		t.Errorf("report: %s", out)
	}
	if again := run(t, ws, Options{}); again.Changed() {
		t.Error("the second run changes something")
	}
}

func TestEmptyWorkspace(t *testing.T) {
	ws := t.TempDir()
	rep := run(t, ws, Options{})
	if rep.Changed() || printed(rep) != "Уже контракт v5, менять нечего.\n" {
		t.Errorf("empty workspace: %q", printed(rep))
	}
	if _, err := Workspace(ws+"/absent", Options{}); err == nil {
		t.Error("a missing workspace must fail")
	}
}
