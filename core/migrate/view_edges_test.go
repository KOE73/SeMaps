package migrate

import (
	"reflect"
	"strings"
	"testing"
)

// The `edges` of a view is an overlay on the registry (ADR_20260930-7): a full
// list of the earlier form becomes exceptions of the view and the entries that
// have a style, an override or a routing of their own.

func edgeEntry(id, from, to, typ, own string) string {
	s := `{"id":"` + id + `","from":"` + from + `","to":"` + to + `","type":"` + typ + `"`
	if own != "" {
		s += "," + own
	}
	return s + "}"
}

const edgePlacements = `"placements":[
	{"entity":"e_a","parent":null,"x":0,"y":0},{"entity":"e_b","parent":null,"x":0,"y":0},
	{"entity":"e_c","parent":null,"x":0,"y":0},{"entity":"e_d","parent":null,"x":0,"y":0}]`

func edgeView(id, head, edges string) string {
	s := `{"id":"` + id + `","project":"p","axis":"a",` + head + edgePlacements
	if edges != "" {
		s += `,"edges":` + edges
	}
	return s + "}"
}

// edgesProject: four placed entities and one that is not; `depends` is visible
// in the dictionary, `uses` is hidden.
func edgesProject() map[string]string {
	return map[string]string{
		"projects/p/project.json": `{"id":"p","title":"P","contractVersion":5,"languages":["ru"]}`,
		"projects/p/entities.json": `{"contractVersion":5,"entities":[
			{"id":"e_a","name":"A","kind":"class","origin":"code"},{"id":"e_b","name":"B","kind":"class","origin":"code"},
			{"id":"e_c","name":"C","kind":"class","origin":"code"},{"id":"e_d","name":"D","kind":"class","origin":"code"},
			{"id":"e_z","name":"Z","kind":"class","origin":"code"}]}`,
		"projects/p/relations.json": `{"contractVersion":5,"relations":[
			{"id":"r1","from":"e_a","to":"e_b","type":"depends","origin":"code"},
			{"id":"r2","from":"e_a","to":"e_c","type":"depends","origin":"code"},
			{"id":"r3","from":"e_b","to":"e_c","type":"uses","origin":"code"},
			{"id":"r4","from":"e_c","to":"e_d","type":"depends","origin":"code"},
			{"id":"r5","from":"e_a","to":"e_z","type":"depends","origin":"code"}]}`,
		"projects/p/text.ru.json": `{"contractVersion":5,"language":"ru","entries":{}}`,
	}
}

func allEdges() string {
	return "[" + strings.Join([]string{
		edgeEntry("r1", "e_a", "e_b", "depends", ""),
		edgeEntry("r2", "e_a", "e_c", "depends", `"routing":"bezier"`),
		edgeEntry("r3", "e_b", "e_c", "uses", `"override":{"line":{"color":"#ff0000"}}`),
		edgeEntry("r4", "e_c", "e_d", "depends", ""),
		edgeEntry("r5", "e_a", "e_z", "depends", ""),
	}, ",") + "]"
}

func TestViewEdgesFullListBecomesAnOverlay(t *testing.T) {
	files := edgesProject()
	// a list that copied the whole registry, a few lines with their own routing
	files["projects/p/views/v_bloat.view.json"] = edgeView("v_bloat", "", allEdges())
	// a curated list; the old exceptions were dead under a list and are recomputed
	files["projects/p/views/v_curated.view.json"] = edgeView("v_curated", `"relations":{"default":"visible","except":["r_old"]},`,
		"["+edgeEntry("r1", "e_a", "e_b", "depends", "")+","+edgeEntry("r3", "e_b", "e_c", "uses", `"routing":"bezier"`)+"]")
	// a line that exists on the view only becomes a relation of the registry; one whose end is no entity is dropped
	files["projects/p/views/v_own.view.json"] = edgeView("v_own", "",
		"["+edgeEntry("r1", "e_a", "e_b", "depends", "")+","+edgeEntry("r_new", "e_a", "e_d", "depends", `"routing":"tree-vertical"`)+","+
			edgeEntry("r_ghost", "e_a", "e_nope", "depends", "")+"]")
	// `edges: []` was "no lines"
	files["projects/p/views/v_none.view.json"] = edgeView("v_none", "", "[]")
	// the current shape and a view with no list are left alone
	files["projects/p/views/v_now.view.json"] = edgeView("v_now", "", `[{"id":"r1","routing":"bezier"}]`)
	files["projects/p/views/v_plain.view.json"] = edgeView("v_plain", `"relations":{"default":"hidden","except":["r2"]},`, "")
	ws := mkws(t, files)

	rep := run(t, ws, Options{DefaultKinds: defaultKinds()})
	p := rep.Projects[0]
	if p.Skipped || len(p.ViewEdges) != 4 {
		t.Fatalf("report: %+v", *p)
	}
	view := func(id string) *obj { return readTree(t, ws, "projects/p/views/"+id+".view.json") }

	// bloated: everything was listed; `uses` is hidden by its type, so only r3 needs an exception
	b := view("v_bloat")
	if got := js(t, b, "relations"); got != `{"default":"visible","except":["r3","r_new"]}` {
		t.Errorf("v_bloat relations: %s", got)
	}
	if got := js(t, b, "edges"); got != `[{"id":"r2","routing":"bezier"},{"id":"r3","override":{"line":{"color":"#ff0000"}}}]` {
		t.Errorf("v_bloat edges: %s", got)
	}
	// the relation r5 is listed but one end is not placed: it is not drawn (and the report says so)
	if !anyContains(p.ViewEdges, "v_bloat: edges: 5 → 2, relations.except: 0 → 2, линий списка без обоих концов на виде (не рисуются): 1") {
		t.Errorf("report: %v", p.ViewEdges)
	}

	// curated: r1 and r3 stay visible, the other lines with both ends placed are hidden
	c := view("v_curated")
	if got := js(t, c, "relations"); got != `{"default":"visible","except":["r2","r3","r4","r_new"]}` {
		t.Errorf("v_curated relations: %s", got)
	}
	if got := js(t, c, "edges"); got != `[{"id":"r3","routing":"bezier"}]` {
		t.Errorf("v_curated edges: %s", got)
	}

	// view-only: an authored relation of the registry, the ghost dropped and named
	o := view("v_own")
	rels := readTree(t, ws, "projects/p/relations.json")
	if arrLen(t, rels, "relations") != 6 {
		t.Fatalf("relations: %s", js(t, rels))
	}
	if got := js(t, rels, "relations", 5); got != `{"id":"r_new","from":"e_a","to":"e_d","type":"depends","origin":"authored","status":"present"}` {
		t.Errorf("new relation: %s", got)
	}
	if got := js(t, o, "edges"); got != `[{"id":"r_new","routing":"tree-vertical"}]` {
		t.Errorf("v_own edges: %s", got)
	}
	if got := js(t, o, "relations"); got != `{"default":"visible","except":["r2","r4"]}` {
		t.Errorf("v_own relations: %s", got)
	}
	if !anyContains(p.Notes, "линия r_ghost") || !anyContains(p.ViewEdges, "v_own: edges: 3 → 1") || !anyContains(p.ViewEdges, "связей реестра добавлено: 1, отброшено: 1") {
		t.Errorf("notes %v, report %v", p.Notes, p.ViewEdges)
	}

	// `edges: []`: the key goes, every relation with both ends placed and a visible type is hidden
	n := view("v_none")
	if has(n, "edges") {
		t.Errorf("v_none keeps edges: %s", js(t, n))
	}
	if got := js(t, n, "relations"); got != `{"default":"visible","except":["r1","r2","r4","r_new"]}` {
		t.Errorf("v_none relations: %s", got)
	}

	// the views that had no list of the earlier form keep their picture: the relation the lists added is hidden there
	if got := js(t, view("v_now"), "edges"); got != `[{"id":"r1","routing":"bezier"}]` || js(t, view("v_now"), "relations") != `{"default":"visible","except":["r_new"]}` {
		t.Errorf("v_now: %s", js(t, view("v_now")))
	}
	if got := js(t, view("v_plain"), "relations"); got != `{"default":"hidden","except":["r2","r_new"]}` || has(view("v_plain"), "edges") {
		t.Errorf("v_plain: %s", js(t, view("v_plain")))
	}

	// idempotent
	before := snapshot(t, ws)
	second := run(t, ws, Options{DefaultKinds: defaultKinds()})
	if second.Changed() || !reflect.DeepEqual(before, snapshot(t, ws)) {
		t.Errorf("second run changed something: %q", printed(second))
	}
	if !strings.Contains(printed(rep), "виды с полным списком edges") {
		t.Errorf("the report does not name the views: %s", printed(rep))
	}
}

// A view with no `relations` gets one only when an exception is needed; the
// dictionary's visibility of the type of the workspace is the one that counts.
func TestViewEdgesUseTheWorkspaceDictionaryVisibility(t *testing.T) {
	files := edgesProject()
	files["kinds.json"] = `{"contractVersion":5,"relationGroups":[{"id":"g","name":{"ru":"г"},"types":[
		{"id":"depends","visibility":"hidden","name":{"ru":"зависит"}}]}]}`
	files["projects/p/views/v.view.json"] = edgeView("v", "", "["+edgeEntry("r1", "e_a", "e_b", "depends", `"routing":"bezier"`)+"]")
	ws := mkws(t, files)
	run(t, ws, Options{DefaultKinds: defaultKinds()})
	v := readTree(t, ws, "projects/p/views/v.view.json")
	// depends is hidden here: r1 was listed, so it is an exception; r2 and r4 are hidden already
	if got := js(t, v, "relations"); got != `{"default":"visible","except":["r1"]}` {
		t.Errorf("relations: %s", got)
	}
	if got := strings.Join(v.keys, ","); got != "id,project,axis,relations,placements,edges" {
		t.Errorf("keys: %s", got)
	}
}
