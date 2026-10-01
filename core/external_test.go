package core

import (
	"strings"
	"testing"
)

const externalFacts = `{"language":"go","root":".","edgeKinds":["extends","implements","contains","depends","holds","uses"],"symbols":[
    {"id":"demo/game.Log","kind":"type","nativeKind":"struct","name":"Log","namespace":"demo/game","file":"go/log.go","line":3},
    {"id":"error","kind":"interface","nativeKind":"external","name":"error"},
    {"id":"io.Writer","kind":"interface","nativeKind":"external","name":"Writer","namespace":"io","members":[{"kind":"method","name":"Write","type":"func(p []byte) (n int, err error)","visibility":"exported"}]}
  ],"edges":[
    {"from":"demo/game.Log","to":"error","kind":"implements","native":"methodset"},
    {"from":"demo/game.Log","to":"io.Writer","kind":"implements","native":"methodset.ptr"}
  ]}`

// An interface outside the read code comes as an external symbol: no file,
// not filtered by sources.include; it becomes an entity whose realization has no ref, and
// the edge to it is an ordinary relation (ADR_20260927-4).
func TestSyncExternalSymbol(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":5,"sources":{"include":["go"]}}`, `{"entities":[]}`, "", "")
	f := facts(t, externalFacts)
	if rep := sync(t, ws, f, SyncOptions{}); rep.ExitCode() != 0 {
		t.Fatalf("exit %d: %+v", rep.ExitCode(), rep)
	}
	v := load(t, dir)
	w := find(v.Entities, "e_writer")
	if w == nil || w["kind"] != "external" || symbolOf(w) != "io.Writer" || w["namespace"] != "io" {
		t.Fatalf("external entity: %v", v.Entities)
	}
	// its realization has no file: a code[] entry without ref (ADR_20260927-4)
	if r := realization(w, "go"); r == nil || len(w["code"].([]any)) != 1 {
		t.Fatalf("external entity has no realization of its language: %v", w)
	} else if ref, has := r["ref"]; has {
		t.Fatalf("an external realization has no ref, got %v", ref)
	}
	if find(v.Entities, "e_error") == nil {
		t.Fatalf("entities: %v", v.Entities)
	}
	r := find(v.Relations, "r_log_writer_implements")
	if r == nil || r["type"] != "implements" {
		t.Fatalf("relations: %v", v.Relations)
	}
	if _, has := r["native"]; has {
		t.Fatalf("native is not written to the registry: %v", r)
	}
	if rep := sync(t, ws, f, SyncOptions{DryRun: true}); len(rep.Changed)+len(rep.Added)+len(rep.Gone) != 0 {
		t.Fatalf("second run is not quiet: %+v", rep)
	}
}

// An external symbol is an end of an edge like any other: it can be the `from`
// end too, and the relation's evidence then names only the symbol.
func TestSyncExternalSymbolAsFromEnd(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, "", "")
	f := facts(t, `{"language":"go","root":".","symbols":[
    {"id":"a.Local","kind":"interface","nativeKind":"interface","name":"Local","file":"a.go"},
    {"id":"io.Reader","kind":"interface","nativeKind":"external","name":"Reader","namespace":"io"}
  ],"edges":[{"from":"io.Reader","to":"a.Local","kind":"extends"}]}`)
	sync(t, ws, f, SyncOptions{})
	r := find(load(t, dir).Relations, "r_reader_local_extends")
	if r == nil {
		t.Fatalf("relations: %v", load(t, dir).Relations)
	}
	ev := r["evidence"].([]any)[0].(map[string]any)
	if _, has := ev["ref"]; has || ev["symbol"] != "io.Reader" || ev["lang"] != "go" {
		t.Fatalf("evidence of an external end: %v", ev)
	}
}

func TestReadFactsExternalHasNoFile(t *testing.T) {
	_, err := ReadFacts(strings.NewReader(`{"language":"go","root":".","symbols":[
    {"id":"io.Writer","kind":"interface","nativeKind":"external","name":"Writer","file":"io.go"}
  ],"edges":[]}`))
	if err == nil || !strings.Contains(err.Error(), "an external symbol has no file") {
		t.Fatalf("want a problem about the file, got %v", err)
	}
	// a method is never external: its nativeKind is a closed list
	_, err = ReadFacts(strings.NewReader(`{"language":"go","root":".","symbols":[
    {"id":"T","kind":"type","nativeKind":"struct","name":"T","file":"t.go"},
    {"id":"T.M()","kind":"method","nativeKind":"external","name":"M"}
  ],"edges":[{"from":"T","to":"T.M()","kind":"contains"}]}`))
	if err == nil || !strings.Contains(err.Error(), "nativeKind") {
		t.Fatalf("want a problem about a method's nativeKind, got %v", err)
	}
}

// `native` is for display and filters: sync does not compare it, and it never
// changes the relation type sync derives (ADR_20260927 §3, ADR_20260930-3).
func TestSyncNativeDoesNotChangeTheRegistry(t *testing.T) {
	const tmpl = `{"language":"csharp","root":".","edgeKinds":["extends","implements","contains","holds","uses"],"symbols":[
    {"id":"N.A","kind":"type","nativeKind":"class","name":"A","namespace":"N","file":"a.cs"},
    {"id":"N.B","kind":"type","nativeKind":"class","name":"B","namespace":"N","file":"b.cs"},
    {"id":"N.IA","kind":"interface","nativeKind":"interface","name":"IA","namespace":"N","file":"i.cs"}
  ],"edges":[
    {"from":"N.A","to":"N.B","kind":"extends"%[1]s},
    {"from":"N.A","to":"N.B","kind":"holds"%[2]s,"via":{"member":"b","memberKind":"field","cardinality":"one","modifiers":["public"]}},
    {"from":"N.A","to":"N.B","kind":"uses"%[3]s,"via":{"member":"p","memberKind":"constructor"}},
    {"from":"N.A","to":"N.IA","kind":"implements"%[4]s}
  ]}`
	build := func(a, b, c, d string) string {
		return strings.NewReplacer("%[1]s", a, "%[2]s", b, "%[3]s", c, "%[4]s", d).Replace(tmpl)
	}
	plain := build("", "", "", "")
	withNative := build(`,"native":"class"`, `,"native":"field"`, `,"native":"constructor"`, `,"native":"interface"`)
	other := build(`,"native":"embed"`, `,"native":"property"`, `,"native":"parameter"`, `,"native":"methodset"`)

	wsA, dirA := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, "", "")
	sync(t, wsA, facts(t, plain), SyncOptions{})
	wsB, dirB := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, "", "")
	sync(t, wsB, facts(t, withNative), SyncOptions{})
	for _, name := range []string{"entities.json", "relations.json"} {
		if a, b := readFile(t, dirA+"/"+name), readFile(t, dirB+"/"+name); a != b {
			t.Errorf("%s differs with native:\n--- without\n%s\n--- with\n%s", name, a, b)
		}
	}
	v := load(t, dirB)
	types := map[string]bool{}
	for _, r := range v.Relations {
		types[r["type"].(string)] = true
	}
	for _, want := range []string{"extends", "implements", "holds.one", "injects"} {
		if !types[want] {
			t.Errorf("relation type %s is not derived: %v", want, types)
		}
	}
	// A different native on the next run is no change at all.
	if rep := sync(t, wsB, facts(t, other), SyncOptions{DryRun: true}); len(rep.Changed)+len(rep.Added)+len(rep.Gone) != 0 {
		t.Fatalf("native is compared: %+v", rep)
	}
}

// The live graph shows an external symbol as a node like any other: it has no
// file, and no format may print a hole where the file would be.
func TestBuildGraphExternalNode(t *testing.T) {
	m := graphModel(t, `{"entities":[]}`, `{"relations":[]}`)
	f := facts(t, externalFacts)
	g, err := BuildGraph([]FactsSource{{Extractor: "go", Facts: f}}, m)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	var w *GraphNode
	for i := range g.Nodes {
		if g.Nodes[i].Symbol == "io.Writer" {
			w = &g.Nodes[i]
		}
	}
	if w == nil || w.Presence != "code" || w.File != "" || w.NativeKind != "external" || w.Kind != "interface" {
		t.Fatalf("external node: %+v", w)
	}
	if len(g.Edges) != 2 || g.Edges[0].Kind != "implements" {
		t.Fatalf("edges: %+v", g.Edges)
	}
	for _, format := range GraphFormats() {
		out, err := format.Format(g, FormatOptions{Focus: "go:io.Writer", Fields: map[string]bool{"via": true, "position": true}})
		if err != nil {
			continue // tree without a usable focus etc. has its own tests
		}
		if !strings.Contains(string(out), "Writer") {
			t.Errorf("format %s lost the external node:\n%s", format.Name(), out)
		}
		if strings.Contains(string(out), ":0") && format.Name() != "json" && format.Name() != "json-compact" {
			t.Errorf("format %s prints a hole for the missing file:\n%s", format.Name(), out)
		}
	}
}
