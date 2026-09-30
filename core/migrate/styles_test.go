package migrate

import (
	"reflect"
	"strings"
	"testing"
)

// The tool's shipped library, as far as the ids matter.
const shippedStyles = `{"styles":[
	{"id":"default.node","appliesTo":"block"},{"id":"default.container","appliesTo":"container"},{"id":"default.edge","appliesTo":"edge"},
	{"id":"class","appliesTo":"block","forKinds":["class"]},{"id":"interface","appliesTo":"block","forKinds":["interface"]},
	{"id":"subsystem","appliesTo":"container","forKinds":["subsystem"]},{"id":"call","appliesTo":"edge","forKinds":["call"]}]}`

// A workspace library of contract 3: a copy of the old default library plus what
// the project added.
const workspaceStyles = `{"version":1,"styles":[
	{"id":"default.node","appliesTo":"block","icon":{"glyph":"📄"}},
	{"id":"default.zone","appliesTo":"block"},
	{"id":"class","appliesTo":"block","fill":"#fff"},
	{"id":"interface","appliesTo":"block","kinds":["interface"],"fill":"#eee"},
	{"id":"component","appliesTo":"block","fill":"#abc"},
	{"id":"zone.green","appliesTo":"block","basedOn":"default.zone","fill":"#0f0"},
	{"id":"call","appliesTo":"edge"},
	{"id":"custom.edge","appliesTo":"edge"},
	{"id":"iface2","appliesTo":"block","basedOn":"component","forKinds":["interface"]}]}`

func styledProject() map[string]string {
	return v5Old(map[string]string{
		"styles.json":               workspaceStyles,
		"projects/p/entities.json":  `{"entities":[{"id":"e_a","name":"A","kind":"class"},{"id":"e_b","name":"B","kind":"class"},{"id":"e_c","name":"C","kind":"class"}]}`,
		"projects/p/relations.json": `{"relations":[]}`,
		"projects/p/views/v.view.json": `{"id":"v","project":"p","axis":"a","placements":[
			{"entity":"e_a","parent":null,"x":0,"y":0,"styleId":"component"},
			{"entity":"e_b","parent":null,"x":0,"y":0,"styleId":"class"},
			{"entity":"e_c","parent":null,"x":0,"y":0,"styleId":"iface2"}],
			"edges":[{"id":"r_x","from":"e_a","to":"e_b","type":"call","styleId":"custom.edge"},{"id":"r_y","from":"e_a","to":"e_c","type":"call","styleId":"call"}]}`,
	})
}

func styleIDs(t *testing.T, ws string) []string {
	t.Helper()
	var out []string
	for _, s := range at(t, readTree(t, ws, "styles.json"), "styles").([]any) {
		out = append(out, str(t, s, "id"))
	}
	return out
}

func TestStylesLikeAShippedDefaultAreDropped(t *testing.T) {
	ws := mkws(t, styledProject())
	rep := run(t, ws, Options{DefaultStyles: []byte(shippedStyles), Extractors: []Extractor{{Project: "p", Language: "csharp"}}})
	// default.zone became default.container and, with default.node, class and call, lost to the shipped default of its id;
	// a style that names its types (interface, iface2) stays even under a shipped id
	if got := styleIDs(t, ws); !reflect.DeepEqual(got, []string{"interface", "component", "zone.green", "custom.edge", "iface2"}) {
		t.Errorf("styles left: %v", got)
	}
	if !reflect.DeepEqual(rep.StylesLikeDefault, []string{"default.node", "default.container", "class", "call"}) {
		t.Errorf("dropped as a default: %v", rep.StylesLikeDefault)
	}
	// the others without a type are listed, as before
	if !reflect.DeepEqual(rep.StylesNoKind, []string{"component", "zone.green"}) || !reflect.DeepEqual(rep.StylesNoKindEdge, []string{"custom.edge"}) {
		t.Errorf("without a type: %v / %v", rep.StylesNoKind, rep.StylesNoKindEdge)
	}
	// nothing was named that stays unresolved: the ids of the shipped styles still resolve, the views are unchanged
	v := readTree(t, ws, "projects/p/views/v.view.json")
	if got := js(t, v, "placements", 1, "styleId"); got != `"class"` {
		t.Errorf("a placement of a shipped style: %s", got)
	}
	out := printed(rep)
	for _, frag := range []string{"удалены стили, совпадающие по id со стилями по умолчанию и без типа", "(default.node, default.container, class, call)",
		"остаётся 5 стилей и заменяет библиотеку по умолчанию целиком"} {
		if !strings.Contains(out, frag) {
			t.Errorf("report lacks %q:\n%s", frag, out)
		}
	}
	if again := run(t, ws, Options{DefaultStyles: []byte(shippedStyles), Extractors: []Extractor{{Project: "p", Language: "csharp"}}}); again.Changed() {
		t.Errorf("second run: %s", printed(again))
	}
}

func TestDropUntypedStyles(t *testing.T) {
	opt := Options{DefaultStyles: []byte(shippedStyles), DropUntypedStyles: true, Extractors: []Extractor{{Project: "p", Language: "csharp"}}}
	ws := mkws(t, styledProject())
	before := snapshot(t, ws)
	dry := opt
	dry.DryRun = true
	dryRep := run(t, ws, dry)
	if !reflect.DeepEqual(before, snapshot(t, ws)) || !dryRep.Changed() {
		t.Fatal("a dry run wrote or found nothing")
	}
	rep := run(t, ws, opt)
	if got := styleIDs(t, ws); !reflect.DeepEqual(got, []string{"interface", "iface2"}) {
		t.Errorf("styles left: %v", got)
	}
	if !reflect.DeepEqual(rep.StylesDropped, []string{"component", "zone.green", "custom.edge"}) {
		t.Errorf("dropped untyped: %v", rep.StylesDropped)
	}
	if len(rep.StylesNoKind)+len(rep.StylesNoKindEdge) != 0 {
		t.Errorf("nothing without a type is left: %v %v", rep.StylesNoKind, rep.StylesNoKindEdge)
	}
	// the placement and the edge entry that named a dropped style lose it and say so; the rest stay
	v := readTree(t, ws, "projects/p/views/v.view.json")
	if has(at(t, v, "placements", 0), "styleId") {
		t.Errorf("a dropped style is still named: %s", js(t, v))
	}
	if got := js(t, v, "placements", 1, "styleId"); got != `"class"` {
		t.Errorf("the style of a shipped default stays named: %s", got)
	}
	// the entry that had only the dropped style has nothing of its own left and goes
	if got := js(t, v, "edges"); got != `[{"id":"r_y","styleId":"call"}]` {
		t.Errorf("edges: %s", got)
	}
	// the style that was based on a dropped one loses the link
	if got := js(t, readTree(t, ws, "styles.json"), "styles", 1); got != `{"id":"iface2","appliesTo":"block","forKinds":["interface"]}` {
		t.Errorf("iface2: %s", got)
	}
	refs := rep.Projects[0].DroppedStyleRefs
	if len(refs) != 2 || refs[0] != "«component» — 1: v: размещение e_a" || refs[1] != "«custom.edge» — 1: v: связь r_x" {
		t.Errorf("dropped refs: %v", refs)
	}
	out := printed(rep)
	for _, frag := range []string{"styleId сняты: стиль удалён из styles.json (--drop-untyped-styles)", "удалены стили без типа (--drop-untyped-styles): 3 (component, zone.green, custom.edge)",
		"у стиля iface2 снята ссылка basedOn на удалённый стиль component"} {
		if !strings.Contains(out, frag) {
			t.Errorf("report lacks %q:\n%s", frag, out)
		}
	}
	if again := run(t, ws, opt); again.Changed() {
		t.Errorf("second run: %s", printed(again))
	}
}

// A library left without a style is removed: an empty styles.json would replace
// the shipped library with nothing.
func TestAnEmptiedLibraryIsRemoved(t *testing.T) {
	files := styledProject()
	files["styles.json"] = `{"styles":[{"id":"default.node","appliesTo":"block"},{"id":"class","appliesTo":"block"},{"id":"component","appliesTo":"block"}]}`
	files["projects/p/views/v.view.json"] = `{"id":"v","project":"p","axis":"a","placements":[{"entity":"e_a","parent":null,"styleId":"component"}]}`
	ws := mkws(t, files)
	opt := Options{DefaultStyles: []byte(shippedStyles), DropUntypedStyles: true, Extractors: []Extractor{{Project: "p", Language: "csharp"}}}
	rep := run(t, ws, opt)
	if exists(ws, "styles.json") || !contains(rep.Removed, "styles.json") {
		t.Errorf("styles.json must be removed: %v", rep.Removed)
	}
	if !anyContains(rep.WorkspaceNotes, "стилей не осталось, файл удалён") {
		t.Errorf("notes: %v", rep.WorkspaceNotes)
	}
	if again := run(t, ws, opt); again.Changed() {
		t.Errorf("second run: %s", printed(again))
	}
}

// Without the tool's library nothing is known to be a default: only the option
// drops, and the three fallback styles are never dropped by it.
func TestDropUntypedKeepsTheFallbackStyles(t *testing.T) {
	ws := mkws(t, styledProject())
	rep := run(t, ws, Options{DropUntypedStyles: true, Extractors: []Extractor{{Project: "p", Language: "csharp"}}})
	if got := styleIDs(t, ws); !reflect.DeepEqual(got, []string{"default.node", "default.container", "interface", "iface2"}) {
		t.Errorf("styles left: %v", got)
	}
	if len(rep.StylesLikeDefault) != 0 || len(rep.StylesDropped) != 5 {
		t.Errorf("like default %v, dropped %v", rep.StylesLikeDefault, rep.StylesDropped)
	}
}
