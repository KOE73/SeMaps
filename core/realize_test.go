package core

import (
	"path/filepath"
	"strings"
	"testing"
)

// The shape of ADR_20260930-4: an entity's realizations are `code[]`, a
// relation's are `evidence[]` (with the member signature `via` inside).
func TestApplyChecksTheCodeShape(t *testing.T) {
	cases := []struct {
		name, want string
		op         Op
	}{
		{"entry with nothing", "neither ref nor symbol", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"status":"missing"}]}`)},
		{"lang without symbol", "lang and symbol together", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"go","ref":"a.go"}]}`)},
		{"symbol without lang", "lang and symbol together", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"ref":"a.go","symbol":"a.A"}]}`)},
		{"one language twice", "two code[] entries of language go", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"go","symbol":"a.A"},{"lang":"go","symbol":"a.B"}]}`)},
		{"a status of nothing", "status", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"ref":"a.go","status":"gone"}]}`)},
		{"codeRef of contract 5 before", "верхнего уровня", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","codeRef":"a.go"}`)},
		{"symbol of contract 5 before", "верхнего уровня", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","symbol":"N.A"}`)},
		{"via on the relation", "верхнего уровня", modelOp("relation", "r_a_b_items_item", "", "", `{"id":"r_a_b_items_item","from":"e_a","to":"e_b","type":"holds.many","via":{"member":"items"}}`)},
		{"evidence codeRef", "evidence[].codeRef", modelOp("relation", "r_a_b_items_item", "", "", `{"id":"r_a_b_items_item","from":"e_a","to":"e_b","type":"holds.many","evidence":[{"codeRef":"a.cs"}]}`)},
		{"evidence with nothing", "neither ref nor symbol", modelOp("relation", "r_a_b_items_item", "", "", `{"id":"r_a_b_items_item","from":"e_a","to":"e_b","type":"holds.many","evidence":[{"lang":"csharp"}]}`)},
		{"via of an entity", "via belongs to a relation", modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"ref":"a.go","via":{"member":"x"}}]}`)},
	}
	for _, c := range cases {
		m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Apply([]Op{c.op}, "human"); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	// what the shape allows: zero, one or several realizations of different languages, a file link alone,
	// an external symbol without a file, an evidence entry that carries a via alone
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	ok := []Op{
		modelOp("entity", "e_a", "", "", `{"id":"e_a","name":"A","kind":"class","origin":"code","code":[{"lang":"csharp","ref":"a.cs","symbol":"N.A"},{"lang":"go","ref":"a.go","symbol":"a.A","status":"missing"},{"ref":"a.csproj"}]}`),
		modelOp("entity", "e_b", "", "", `{"id":"e_b","name":"Writer","kind":"external","origin":"code","code":[{"lang":"go","symbol":"io.Writer"}]}`),
		modelOp("relation", "r_a_b_items_item", "", "", `{"id":"r_a_b_items_item","from":"e_a","to":"e_b","type":"holds.many","evidence":[{"via":{"member":"items"}}]}`),
	}
	if _, err := m.Apply(ok, "human"); err != nil {
		t.Fatalf("a valid shape: %v", err)
	}
}

// The loader reads only the current shape and names the old one.
func TestLoaderRefusesTheShapeBeforeRealizations(t *testing.T) {
	for _, c := range []struct{ name, entities, relations, want string }{
		{"codeRef", `{"entities":[{"id":"e_a","name":"A","kind":"class","codeRef":"a.go"}]}`, `{"relations":[]}`, "e_a: `codeRef` верхнего уровня"},
		{"symbol", `{"entities":[{"id":"e_a","name":"A","kind":"class","symbol":"N.A"}]}`, `{"relations":[]}`, "e_a: `symbol` верхнего уровня"},
		{"name of an authored entity", `{"entities":[{"id":"e_a","name":"A","kind":"group","origin":"authored"}]}`, `{"relations":[]}`, "e_a: `name` у authored-сущности"},
		{"via", `{"entities":[]}`, `{"relations":[{"id":"r_x","from":"a","to":"b","type":"t","via":{"member":"m"}}]}`, "r_x: `via` верхнего уровня"},
	} {
		ws, _ := workspace(t, `{"id":"p","contractVersion":5}`, c.entities, c.relations, "")
		if _, err := LoadModel(ws, "p", defaultKindsJSON(t)); err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "semaps migrate") {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

// Sync reconciles ONE realization per entity per run: the entry of the facts'
// language. An entity realized in another language is not this run's business
// (not marked missing, not adopted, not touched); nothing else is decided
// (ADR_20260930-4).
func TestSyncTouchesOnlyTheRealizationOfItsLanguage(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[
    {"id":"e_go_only","name":"Storage","kind":"struct","origin":"code","code":[{"lang":"go","ref":"go/storage.go","symbol":"demo/storage.Storage"}]},
    {"id":"e_both","name":"Snake","kind":"class","origin":"code","namespace":"Demo","code":[{"lang":"csharp","ref":"cs/Old.cs","symbol":"Demo.Snake"},{"lang":"go","ref":"go/snake.go","symbol":"demo/game.Snake"}],"note":"kept"},
    {"id":"e_named","name":"Snake","kind":"class","origin":"code","status":"present"}]}`, "", "")
	rep := sync(t, ws, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"Demo.Snake","kind":"type","nativeKind":"class","name":"Snake","namespace":"Demo","file":"cs/Snake.cs"}],"edges":[]}`), SyncOptions{})
	v := load(t, dir)
	if s := find(v.Entities, "e_go_only"); s["status"] != nil || len(s["code"].([]any)) != 1 {
		t.Errorf("a Go-only entity was touched by a C# run: %v", s)
	}
	both := find(v.Entities, "e_both")
	if r := realization(both, "csharp"); r["ref"] != "cs/Snake.cs" || r["symbol"] != "Demo.Snake" {
		t.Errorf("the C# realization was not updated: %v", both)
	}
	if r := realization(both, "go"); r["ref"] != "go/snake.go" || r["symbol"] != "demo/game.Snake" || r["status"] != nil {
		t.Errorf("the Go realization was touched: %v", both)
	}
	if len(both["code"].([]any)) != 2 || both["note"] != "kept" || both["status"] == "missing" {
		t.Errorf("entity: %v", both)
	}
	// the entity with no realization bound anywhere and no origin match is left alone by name alone:
	// it is `origin: code`, so the run answers for it; nothing in the facts fits, so it goes missing
	if n := find(v.Entities, "e_named"); n["status"] != "missing" {
		t.Errorf("an origin-code entity without a realization: %v", n)
	}
	if len(rep.Added) != 0 || len(rep.Ambiguous) != 0 || len(rep.Renames) != 0 {
		t.Errorf("report: %+v", rep)
	}
	// an entity bound in another language is not adopted by name either: no second realization is bound by a run
	ws2, dir2 := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[
    {"id":"e_go","name":"Snake","kind":"class","origin":"code","namespace":"Demo","code":[{"lang":"go","ref":"go/snake.go","symbol":"demo/game.Snake"}]}]}`, "", "")
	sync(t, ws2, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"Demo.Snake","kind":"type","nativeKind":"class","name":"Snake","namespace":"Demo","file":"cs/Snake.cs"}],"edges":[]}`), SyncOptions{})
	v2 := load(t, dir2)
	if len(v2.Entities) != 2 || len(find(v2.Entities, "e_go")["code"].([]any)) != 1 || find(v2.Entities, "e_snake") == nil {
		t.Errorf("a run of another language must add its own entity, not bind: %v", v2.Entities)
	}
}

// A relation's evidence carries the language, the file and symbol of its `from`
// end and, for a member relation, the via; a rerun changes nothing, a changed
// wrapper changes only the via of this language's entry.
func TestSyncWritesEvidenceWithViaInside(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":5}`, "", "", "")
	f1 := `{"language":"csharp","root":".","edgeKinds":["extends","holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[{"from":"A","to":"B","kind":"extends"},
    {"from":"A","to":"B","kind":"holds","via":{"member":"items","memberKind":"field","cardinality":"many","text":"List<B>"}}]}`
	sync(t, ws, facts(t, f1), SyncOptions{})
	v := load(t, dir)
	organic, member := find(v.Relations, "r_a_b_extends"), find(v.Relations, "r_a_b_items")
	if organic == nil || member == nil {
		t.Fatalf("relations: %v", v.Relations)
	}
	if _, has := member["via"]; has {
		t.Errorf("via is not a key of the relation any more: %v", member)
	}
	ev := member["evidence"].([]any)
	if len(ev) != 1 {
		t.Fatalf("evidence: %v", ev)
	}
	e0 := ev[0].(map[string]any)
	if e0["lang"] != "csharp" || e0["ref"] != "a.cs" || e0["symbol"] != "A" || e0["via"].(map[string]any)["member"] != "items" {
		t.Errorf("evidence entry: %v", e0)
	}
	if o := organic["evidence"].([]any)[0].(map[string]any); o["lang"] != "csharp" || o["symbol"] != "A" || o["via"] != nil {
		t.Errorf("organic evidence: %v", o)
	}
	if rep := sync(t, ws, facts(t, f1), SyncOptions{DryRun: true}); !rep.Empty() {
		t.Fatalf("second run is not quiet: %+v", rep)
	}
	before := readFile(t, filepath.Join(dir, "relations.json"))
	sync(t, ws, facts(t, f1), SyncOptions{})
	if readFile(t, filepath.Join(dir, "relations.json")) != before {
		t.Error("a clean run rewrote relations.json")
	}
	f2 := strings.Replace(f1, `"text":"List<B>"`, `"text":"IReadOnlyList<B>","mutability":"readonly"`, 1)
	rep := sync(t, ws, facts(t, f2), SyncOptions{})
	if len(rep.Changed) == 0 {
		t.Fatalf("changed via not reported: %+v", rep)
	}
	got := find(load(t, dir).Relations, "r_a_b_items")
	if got["type"] != "holds.many.ro" || got["evidence"].([]any)[0].(map[string]any)["via"].(map[string]any)["text"] != "IReadOnlyList<B>" {
		t.Errorf("relation after the wrapper change: %v", got)
	}
}
