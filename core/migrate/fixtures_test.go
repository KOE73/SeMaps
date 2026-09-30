package migrate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var fixtureNames = []string{"shapes_demo", "overview", "nmfn_like"}

// checkV5Shape verifies invariants every migrated fixture must satisfy.
func checkV5Shape(t *testing.T, ws string) {
	t.Helper()
	projects, _ := filepath.Glob(filepath.Join(ws, "projects", "*", "project.json"))
	for _, pj := range projects {
		dir := filepath.Dir(pj)
		id := filepath.Base(dir)
		proj := readTree(t, ws, "projects/"+id+"/project.json")
		if got := js(t, proj, "contractVersion"); got != "5" {
			t.Errorf("%s: contractVersion %s", id, got)
		}
		if exists(ws, "projects/"+id+"/containers.json") {
			t.Errorf("%s: containers.json still there", id)
		}
		ents := map[string]bool{}
		for _, eid := range entityIDs(t, ws, id) {
			ents[eid] = true
		}
		views, _ := filepath.Glob(filepath.Join(dir, "views", "*.view.json"))
		for _, vf := range views {
			rel := "projects/" + id + "/views/" + filepath.Base(vf)
			v := readTree(t, ws, rel)
			if has(v, "zones") || has(v, "nodes") {
				t.Errorf("%s: zones/nodes remain", rel)
			}
			pl := at(t, v, "placements").([]any)
			containers := map[string]bool{}
			for i, p := range pl {
				po := p.(*obj)
				e := str(t, po, "entity")
				if !ents[e] {
					t.Errorf("%s placements[%d]: unknown entity %s", rel, i, e)
				}
				if !po.has("parent") {
					t.Errorf("%s placements[%d]: no parent key", rel, i)
				}
				if has(po, "id") || has(po, "zone") || has(po, "container") {
					t.Errorf("%s placements[%d]: legacy key", rel, i)
				}
				if ek := entityKind(t, ws, id, e); ek == "group" {
					containers[e] = true
				}
			}
			for i, p := range pl {
				par := at(t, p, "parent")
				if isNull(par) {
					continue
				}
				pe, _ := asStr(par)
				if !containers[pe] {
					t.Errorf("%s placements[%d]: parent %s is not a container placement", rel, i, pe)
				}
			}
		}
		texts, _ := filepath.Glob(filepath.Join(dir, "text.*.json"))
		for _, tf := range texts {
			tr := readTree(t, ws, "projects/"+id+"/"+filepath.Base(tf))
			if !has(tr, "entries") {
				continue
			}
			for _, k := range keysOf(t, tr, "entries") {
				if strings.HasPrefix(k, "z_") || strings.HasPrefix(k, "c_") {
					t.Errorf("%s: text key %s remains", tf, k)
				}
			}
			if got := js(t, tr, "contractVersion"); got != "5" {
				t.Errorf("%s: contractVersion %s", tf, got)
			}
		}
	}
}

func entityKind(t *testing.T, ws, proj, id string) string {
	t.Helper()
	k, _ := asStr(entityByID(t, ws, proj, id).get("kind"))
	return k
}

func TestFixturesIdempotent(t *testing.T) {
	fixedNow(t)
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			ws := copyFixture(t, name)
			first := run(t, ws, Options{})
			if !first.Changed() {
				t.Fatal("first run changed nothing")
			}
			checkV5Shape(t, ws)
			after1 := snapshot(t, ws)

			second := run(t, ws, Options{})
			if second.Changed() {
				t.Fatalf("second run reports changes: %s", printed(second))
			}
			if got := printed(second); got != "Уже контракт v5, менять нечего.\n" {
				t.Fatalf("second run report: %q", got)
			}
			after2 := snapshot(t, ws)
			if !reflect.DeepEqual(after1, after2) {
				for k := range after1 {
					if after1[k] != after2[k] {
						t.Errorf("%s changed by the second run", k)
					}
				}
			}
		})
	}
}

func TestFixturesDryRun(t *testing.T) {
	fixedNow(t)
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			ws := copyFixture(t, name)
			before := snapshot(t, ws)
			dry := run(t, ws, Options{DryRun: true})
			if !dry.Changed() {
				t.Fatal("dry run reports nothing to do")
			}
			if !reflect.DeepEqual(before, snapshot(t, ws)) {
				t.Fatal("dry run wrote files")
			}
			if !strings.Contains(printed(dry), "--dry-run") {
				t.Errorf("dry run report lacks the mode line:\n%s", printed(dry))
			}
			real := run(t, ws, Options{})
			if !reflect.DeepEqual(dry.Projects, real.Projects) || !reflect.DeepEqual(dry.Files, real.Files) ||
				!reflect.DeepEqual(dry.WorkspaceNotes, real.WorkspaceNotes) {
				t.Errorf("dry-run report differs from the real one")
			}
			// Repeated dry run of the migrated workspace: nothing to do.
			if run(t, ws, Options{DryRun: true}).Changed() {
				t.Error("dry run after migration still finds work")
			}
		})
	}
}

func TestFixturesUntouched(t *testing.T) {
	// The fixtures themselves must stay v3.
	for _, name := range fixtureNames {
		matches, _ := filepath.Glob(filepath.Join("testdata", "v3", name, "workspace", "projects", "*", "project.json"))
		if len(matches) != 1 {
			t.Fatalf("%s: %d projects", name, len(matches))
		}
		data, _ := os.ReadFile(matches[0])
		if !strings.Contains(string(data), `"contractVersion": 3`) {
			t.Errorf("%s: fixture is not v3", name)
		}
	}
}

func TestShapesDemo(t *testing.T) {
	fixedNow(t)
	ws := copyFixture(t, "shapes_demo")
	orig := snapshot(t, ws)
	rep := run(t, ws, Options{})
	p := rep.Projects[0]
	if p.Zones != 3 || p.ZoneEntities != 3 || p.ContainerEntities != 0 || p.Placements != 11 ||
		p.Overrides != 3 || p.ContainsAdded != 0 || p.TextsMoved != 3 || p.TextsRemoved != 3 {
		t.Errorf("counts: %+v", *p)
	}
	after := snapshot(t, ws)
	base := "projects/shapes_demo/"

	// Files whose only change is the version are edited textually.
	for _, f := range []string{"project.json", "relations.json", "relation-types.json"} {
		want := strings.Replace(orig[base+f], `"contractVersion": 3`, `"contractVersion": 5`, 1)
		if after[base+f] != want {
			t.Errorf("%s is not a minimal textual edit:\n%s", f, after[base+f])
		}
	}

	view := readTree(t, ws, base+"views/v_main.view.json")
	if got := strings.Join(view.keys, ","); got != "id,project,axis,icon,theme,routing,placements,edges" {
		t.Errorf("view keys: %s", got)
	}
	if arrLen(t, view, "placements") != 11 {
		t.Errorf("placements: %d", arrLen(t, view, "placements"))
	}
	z0 := at(t, view, "placements", 0)
	if got := strings.Join(keysOf(t, z0), ","); got != "entity,parent,x,y,width,height,override,collapsed" {
		t.Errorf("zone placement keys: %s", got)
	}
	if str(t, z0, "entity") != "e_zone_human" || !isNull(at(t, z0, "parent")) {
		t.Errorf("zone placement: %s", js(t, z0))
	}
	// zone.slate resolves through default.zone in the built-in table.
	if got := js(t, z0, "override"); got != `{"fill":"#f8fafc","border":{"color":"#cbd5e1"},"header":{"fill":"#e2e8f0"}}` {
		t.Errorf("override: %s", got)
	}
	if got := js(t, view, "placements", 2, "override"); got != `{"fill":"#f5f3ff","border":{"color":"#c4b5fd"},"header":{"fill":"#ede9fe"}}` {
		t.Errorf("override of zone.violet: %s", got)
	}
	n0 := at(t, view, "placements", 3)
	if got := js(t, n0); got != `{"entity":"actor_user","parent":"e_zone_human","styleId":"actor","x":80,"y":110,"width":110,"height":130}` {
		t.Errorf("node placement: %s", got)
	}
	if got := js(t, view, "placements", 7); !strings.Contains(got, `"template":"picture"`) || !strings.Contains(got, `"parent":"e_zone_machine"`) {
		t.Errorf("node with template: %s", got)
	}
	if arrLen(t, view, "edges") != 6 {
		t.Error("edges must stay")
	}

	// Entities: three groups appended, named from the texts.
	ids := entityIDs(t, ws, "shapes_demo")
	if len(ids) != 11 || ids[8] != "e_zone_human" || ids[10] != "e_zone_storage" {
		t.Errorf("entities: %v", ids)
	}
	g := entityByID(t, ws, "shapes_demo", "e_zone_human")
	if js(t, g) != `{"id":"e_zone_human","name":"Человек","kind":"group","origin":"authored","status":"present"}` {
		t.Errorf("group entity: %s", js(t, g))
	}
	ents := readTree(t, ws, base+"entities.json")
	if has(ents, "contractVersion") {
		t.Error("entities.json had no contractVersion; none must be invented")
	}

	// Texts: description moved to the entity key, the key of the zone gone.
	text := readTree(t, ws, base+"text.ru.json")
	if has(at(t, text, "entries"), "zone_human") {
		t.Error("zone key remains")
	}
	if got := str(t, text, "entries", "e_zone_human", "description", "v"); got != "То, что делается руками." {
		t.Errorf("description: %s", got)
	}
	if has(at(t, text, "entries", "e_zone_human"), "name") {
		t.Error("name must not move to the entity key")
	}
	if got := js(t, text, "contractVersion"); got != "5" {
		t.Errorf("text version %s", got)
	}
	if !contains(rep.Projects[0].Files, base+"entities.json") {
		t.Errorf("files: %v", rep.Projects[0].Files)
	}
	if rep.Projects[0].Overrides != 3 || len(rep.Projects[0].KeptStyleIDs) != 0 {
		t.Errorf("kept style ids: %v", rep.Projects[0].KeptStyleIDs)
	}
}

func TestOverview(t *testing.T) {
	fixedNow(t)
	ws := copyFixture(t, "overview")
	rep := run(t, ws, Options{})
	p := rep.Projects[0]
	if p.Zones != 5 || p.ZoneEntities != 5 || p.Placements != 20 || p.Overrides != 5 || p.TextsMoved != 0 || p.TextsRemoved != 5 {
		t.Errorf("counts: %+v", *p)
	}
	if len(p.NoName) != 0 {
		t.Errorf("no-name: %v", p.NoName)
	}
	ids := entityIDs(t, ws, "overview")
	want := []string{"e_clients", "e_host", "e_core", "e_storage", "e_extractors"}
	if !reflect.DeepEqual(ids[15:], want) {
		t.Errorf("new entities: %v", ids[15:])
	}
	if got := str(t, entityByID(t, ws, "overview", "e_host"), "name"); got != "Хост semaps.exe (host/)" {
		t.Errorf("name: %s", got)
	}
	view := readTree(t, ws, "projects/overview/views/v_overview.view.json")
	last := at(t, view, "placements", 19)
	if str(t, last, "entity") != "e_source" || !isNull(at(t, last, "parent")) {
		t.Errorf("node outside any zone: %s", js(t, last))
	}
	if got := js(t, view, "placements", 3, "override", "fill"); got != `"#fffbeb"` {
		t.Errorf("zone.amber fill: %s", got)
	}
	ents := readTree(t, ws, "projects/overview/entities.json")
	if got := js(t, ents, "contractVersion"); got != "5" {
		t.Errorf("entities version: %s", got)
	}
	// The entity descriptions were not touched.
	text := readTree(t, ws, "projects/overview/text.ru.json")
	if !has(at(t, text, "entries"), "e_editor") || !has(at(t, text, "entries"), "rt_calls") {
		t.Error("entity and relation-type texts must stay")
	}
}

func TestNmfnLike(t *testing.T) {
	fixedNow(t)
	ws := copyFixture(t, "nmfn_like")
	rep := run(t, ws, Options{})
	p := rep.Projects[0]
	if p.Zones != 8 || p.ZoneEntities != 4 || p.ContainerEntities != 4 || p.Placements != 14 ||
		p.Overrides != 4 || p.ContainsAdded != 2 || p.TextsMoved != 5 || p.TextsRemoved != 11 {
		t.Errorf("counts: %+v", *p)
	}
	base := "projects/nmfn/"

	// containers.json first (e_onnx is taken by an existing entity), then zones.
	ids := entityIDs(t, ws, "nmfn")
	want := []string{"e_onnx_2", "e_graph", "e_onnx_core", "e_orphan", "e_deco", "e_sub", "e_odd", "e_teal"}
	if !reflect.DeepEqual(ids[5:], want) {
		t.Fatalf("new entities: %v", ids[5:])
	}
	name := func(id string) string { return str(t, entityByID(t, ws, "nmfn", id), "name") }
	for id, n := range map[string]string{
		"e_onnx_2": "ONNX", "e_graph": "Graph", "e_onnx_core": "Ядро ONNX", "e_orphan": "Сирота",
		"e_deco": "Декор", "e_sub": "sub", "e_odd": "odd", "e_teal": "teal",
	} {
		if name(id) != n {
			t.Errorf("name of %s: %q, want %q", id, name(id), n)
		}
	}
	if len(p.NoName) != 3 {
		t.Errorf("no-name: %v", p.NoName)
	}
	if len(p.LostNames) != 3 || !anyContains(p.LostNames, "en z_deco") || !anyContains(p.LostNames, "en c_onnx:") {
		t.Errorf("lost names: %v", p.LostNames)
	}
	if len(p.LostText) != 1 || !anyContains(p.LostText, "ru z_core.title") {
		t.Errorf("lost text: %v", p.LostText)
	}
	if len(p.UnconsumedText) != 1 || !anyContains(p.UnconsumedText, "ru z_stale «Забытая зона»") {
		t.Errorf("unconsumed: %v", p.UnconsumedText)
	}
	if len(p.MatchRules) != 3 || !anyContains(p.MatchRules, `c_onnx → e_onnx_2: match {"path":["src/NeuroModFlowNet.ONNX/"]}; axis "axis_assembly"`) ||
		!anyContains(p.MatchRules, `"name":["OnnxModel","OnnxSession"]`) {
		t.Errorf("match rules: %v", p.MatchRules)
	}
	if len(p.UnappliedOverrides) != 2 || !anyContains(p.UnappliedOverrides, "e_ghost") || !anyContains(p.UnappliedOverrides, "c_missing") {
		t.Errorf("unapplied overrides: %v", p.UnappliedOverrides)
	}
	if len(p.KeptStyleIDs) != 2 || !anyContains(p.KeptStyleIDs, "«class» — не цветовой") || !anyContains(p.KeptStyleIDs, "«zone.teal»") {
		t.Errorf("kept style ids: %v", p.KeptStyleIDs)
	}
	if len(p.UnknownZones) != 1 || !anyContains(p.UnknownZones, "«z_missing»") {
		t.Errorf("unknown zones: %v", p.UnknownZones)
	}
	if !contains(p.Removed, base+"containers.json") || exists(ws, base+"containers.json") {
		t.Error("containers.json must be removed")
	}

	// Texts: moved to the entity keys in both languages, rt_contains added.
	ru := readTree(t, ws, base+"text.ru.json")
	en := readTree(t, ws, base+"text.en.json")
	if got := str(t, ru, "entries", "e_onnx_core", "doc", "v"); got != "Почему ядро вынесено отдельно.\n\nСм. ADR." {
		t.Errorf("doc: %q", got)
	}
	if got := str(t, ru, "entries", "e_onnx_core", "description", "v"); got != "Зона ядра на виде сборок." {
		t.Errorf("description from the z_ key: %q", got)
	}
	if got := str(t, en, "entries", "e_onnx_2", "description", "v"); got != "Assembly with the model and the session." {
		t.Errorf("en description: %q", got)
	}
	if got := js(t, en, "entries", "e_onnx_2", "description", "fromHash"); got != `"a3f19c01"` {
		t.Errorf("provenance must move as it is: %s", got)
	}
	if got := js(t, ru, "entries", "rt_contains"); got != `{"name":{"v":"содержит","at":"2026-09-30T12:00:00Z","origin":"authored"}}` {
		t.Errorf("rt_contains ru: %s", got)
	}
	if got := str(t, en, "entries", "rt_contains", "name", "v"); got != "contains" {
		t.Errorf("rt_contains en: %s", got)
	}
	// Key position: the entity entry takes the place of its first source key.
	if got := keysOf(t, ru, "entries"); !reflect.DeepEqual(got, []string{"e_onnx_2", "e_onnx_core", "e_deco", "e_session", "v_assemblies", "rt_depends", "rt_contains"}) {
		t.Errorf("ru entries: %v", got)
	}
	if got := str(t, en, "entries", "v_assemblies", "name", "v"); got != "Assemblies <&>" {
		t.Errorf("HTML characters must not be escaped: %q", got)
	}
	raw, _ := os.ReadFile(filepath.Join(ws, "projects", "nmfn", "text.en.json"))
	if !strings.Contains(string(raw), "Assemblies <&>") {
		t.Error("file contains an escaped string")
	}
	if !strings.HasSuffix(string(raw), "}\n") {
		t.Error("no trailing newline")
	}

	// Registry: contains relations and the relation type.
	rels := readTree(t, ws, base+"relations.json")
	if arrLen(t, rels, "relations") != 4 {
		t.Fatalf("relations: %s", js(t, rels))
	}
	if got := js(t, rels, "relations", 2); got != `{"id":"r_onnx_2_onnx_core_contains","from":"e_onnx_2","to":"e_onnx_core","type":"contains","origin":"authored"}` {
		t.Errorf("relation: %s", got)
	}
	if got := str(t, rels, "relations", 3, "id"); got != "r_graph_tracker_contains" {
		t.Errorf("relation: %s", got)
	}
	rt := readTree(t, ws, base+"relation-types.json")
	if got := js(t, rt, "relationTypes", 4); got != `{"id":"contains","origin":"authored","visibility":"hidden"}` {
		t.Errorf("relation type: %s", got)
	}

	// Views.
	v := readTree(t, ws, base+"views/v_assemblies.view.json")
	if got := strings.Join(v.keys, ","); got != "id,project,axis,icon,placements" {
		t.Errorf("view keys: %s", got)
	}
	if got := js(t, v, "placements", 0); got != `{"entity":"e_onnx_2","parent":null,"x":0,"y":0,"width":900,"height":700,"override":{"fill":"#eff6ff","border":{"color":"#93c5fd"},"header":{"fill":"#dbeafe"}}}` {
		t.Errorf("z_onnx: %s", got)
	}
	if got := js(t, v, "placements", 2); got != `{"entity":"e_onnx_core","parent":"e_onnx_2","x":40,"y":60,"width":500,"height":300,"override":{"fill":"#fff1f2","border":{"color":"#fca5a5","dash":"6,4"},"header":{"fill":"#ffe4e6"}},"collapsed":false}` {
		t.Errorf("z_core: %s", got)
	}
	if got := js(t, v, "placements", 4); got != `{"entity":"e_sub","parent":"e_deco","x":1020,"y":540,"width":200,"height":100}` {
		t.Errorf("z_sub (container names a zone): %s", got)
	}
	if got := js(t, v, "placements", 5, "styleId"); got != `"class"` {
		t.Errorf("non colour styleId must stay: %s", got)
	}
	if got := js(t, v, "placements", 6, "styleId"); got != `"zone.teal"` {
		t.Errorf("unresolved styleId must stay: %s", got)
	}
	if got := js(t, v, "placements", 10); !strings.Contains(got, `"entity":"e_tracker","parent":null`) {
		t.Errorf("node without zone: %s", got)
	}
	if got := js(t, v, "placements", 11); !strings.Contains(got, `"entity":"e_onnx","parent":null`) {
		t.Errorf("node in an unknown zone: %s", got)
	}
	// The relation type is hidden, so no view needs an exception.
	if has(v, "relations") {
		t.Error("hidden contains type: no relations.except expected")
	}
	ops := readTree(t, ws, base+"views/v_ops.view.json")
	if got := strings.Join(ops.keys, ","); got != "id,project,axis,routing,placements,edges" {
		t.Errorf("v_ops keys: %s", got)
	}
	if got := str(t, ops, "placements", 0, "entity"); got != "e_onnx_2" {
		t.Errorf("same container on another view is the same entity: %s", got)
	}

	// Workspace files.
	styles := readTree(t, ws, "styles.json")
	if got := str(t, styles, "styles", 1, "id"); got != "default.container" {
		t.Errorf("default zone style id: %s", got)
	}
	if got := str(t, styles, "styles", 1, "appliesTo"); got != "container" {
		t.Errorf("default.container appliesTo: %s", got)
	}
	if got := str(t, styles, "styles", 3, "basedOn"); got != "default.container" {
		t.Errorf("zone.red basedOn: %s", got)
	}
	if got := str(t, styles, "styles", 4, "appliesTo"); got != "container" {
		t.Errorf("zone.red.dashed appliesTo: %s", got)
	}
	if got := str(t, styles, "styles", 2, "appliesTo"); got != "block" {
		t.Errorf("class must stay a block style: %s", got)
	}
	if got := str(t, styles, "styles", 7, "appliesTo"); got != "edge" {
		t.Errorf("extends must stay an edge style: %s", got)
	}
	if !reflect.DeepEqual(rep.StylesNoKind, []string{"class", "zone.red", "zone.red.dashed", "zone.blue"}) ||
		!reflect.DeepEqual(rep.StylesNoKindEdge, []string{"extends"}) {
		t.Errorf("styles without kinds: %v / %v", rep.StylesNoKind, rep.StylesNoKindEdge)
	}
	canvas := readTree(t, ws, "canvas.json")
	if got := strings.Join(canvas.keys, ","); got != "grid,node,container,gap" {
		t.Errorf("canvas keys: %s", got)
	}
	if got := js(t, canvas, "gap"); got != `{"node":40,"container":40}` {
		t.Errorf("canvas gap: %s", got)
	}
	if !contains(rep.Files, "styles.json") || !contains(rep.Files, "canvas.json") {
		t.Errorf("workspace files: %v", rep.Files)
	}

	out := printed(rep)
	for _, frag := range []string{
		"Проект nmfn", "зон → сущностей: 8 → 4", "контейнеров containers.json → сущностей: 4",
		"размещений: 14", "override создано: 4", "связей contains добавлено: 2", "версия: 3 → 5",
		"Решает человек", "правила match", "styleId оставлены", "overrides не применены",
		"стили без типа: 4", "стили без типа (связи): 1", "Общие файлы рабочего пространства",
	} {
		if !strings.Contains(out, frag) {
			t.Errorf("report lacks %q:\n%s", frag, out)
		}
	}
}
