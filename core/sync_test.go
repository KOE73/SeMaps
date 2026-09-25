package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- fixtures

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// workspace makes <tmp>/projects/p with the given registry files; "" skips one.
func workspace(t *testing.T, project, entities, relations, types string) (ws, dir string) {
	t.Helper()
	ws = t.TempDir()
	dir = filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(dir, "project.json"), project)
	for name, content := range map[string]string{
		"entities.json": entities, "relations.json": relations, "relation-types.json": types,
	} {
		if content != "" {
			writeFile(t, filepath.Join(dir, name), content)
		}
	}
	return ws, dir
}

func facts(t *testing.T, doc string) *Facts {
	t.Helper()
	f, err := ReadFacts(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("facts: %v", err)
	}
	return f
}

type registryView struct {
	Entities  []map[string]any `json:"entities"`
	Relations []map[string]any `json:"relations"`
	Types     []map[string]any `json:"relationTypes"`
}

func load(t *testing.T, dir string) registryView {
	t.Helper()
	var v registryView
	for _, name := range []string{"entities.json", "relations.json", "relation-types.json"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	return v
}

func find(list []map[string]any, id string) map[string]any {
	for _, m := range list {
		if m["id"] == id {
			return m
		}
	}
	return nil
}

func sync(t *testing.T, ws string, f *Facts, opt SyncOptions) *SyncReport {
	t.Helper()
	rep, err := Sync(ws, f, opt)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return rep
}

// The situation of a real registry: an assembly and a class written by hand
// (the assembly by the ADOPTING script, the class with no origin at all), an
// authored entity whose name a code symbol also carries, a text catalogue and
// a view that sync must never touch.
const (
	adoptProject  = `{"id":"p","contractVersion":3,"sources":{"include":["src"]}}`
	adoptEntities = `{
  "entities": [
    {"id":"e_asm_onnx","name":"NeuroModFlowNet.ONNX","kind":"assembly","origin":"code","codeRef":"src/NeuroModFlowNet.ONNX/NeuroModFlowNet.ONNX.csproj"},
    {"id":"e_x","name":"X","kind":"class","namespace":"N","codeRef":"src/NeuroModFlowNet.ONNX/Old/X.cs","note":"kept"},
    {"id":"e_guard","name":"Guard","kind":"concept","origin":"authored","status":"planned"}
  ]
}`
	adoptRelations = `{"contractVersion":3,"relations":[
    {"id":"r_hand","from":"e_guard","to":"e_x","type":"call","origin":"authored"}
  ]}`
	adoptTypes = `{"contractVersion":3,"relationTypes":[{"id":"call","origin":"authored"},{"id":"extends","origin":"code"}]}`
	adoptFacts = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["extends","implements","contains","depends","holds","uses"],
  "symbols": [
    {"id":"N.Base","kind":"type","nativeKind":"class","name":"Base","namespace":"N","file":"src/NeuroModFlowNet.ONNX/Base.cs"},
    {"id":"N.Guard","kind":"type","nativeKind":"class","name":"Guard","namespace":"N","file":"src/NeuroModFlowNet.ONNX/Guard.cs",
     "members":[{"kind":"property","name":"Limit","type":"int","visibility":"public"}]},
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/NeuroModFlowNet.ONNX/X.cs",
     "members":[{"kind":"method","name":"Run","type":"Task<int>","visibility":"public"}]},
    {"id":"NeuroModFlowNet.ONNX","kind":"module","nativeKind":"assembly","name":"NeuroModFlowNet.ONNX","file":"src/NeuroModFlowNet.ONNX/NeuroModFlowNet.ONNX.csproj"},
    {"id":"Tests.T","kind":"type","nativeKind":"class","name":"T","namespace":"Tests","file":"tests/T.cs"}
  ],
  "edges": [
    {"from":"N.X","to":"N.Base","kind":"extends"},
    {"from":"N.X","to":"N.Guard","kind":"uses","via":{"member":"guard","memberKind":"field","text":"Guard"}},
    {"from":"NeuroModFlowNet.ONNX","to":"N.Base","kind":"contains"},
    {"from":"NeuroModFlowNet.ONNX","to":"N.Guard","kind":"contains"},
    {"from":"NeuroModFlowNet.ONNX","to":"N.X","kind":"contains"},
    {"from":"Tests.T","to":"N.X","kind":"uses","via":{"member":"x","memberKind":"field","text":"X"}}
  ]
}`
)

// ------------------------------------------------------------------- facts

func TestReadFactsReportsEveryProblem(t *testing.T) {
	_, err := ReadFacts(strings.NewReader(`{
  "language": "C#",
  "symbols": [
    {"id":"b","kind":"class","nativeKind":"class","name":"B","file":"src\\B.cs"},
    {"id":"a","kind":"type","nativeKind":"class","name":"A","file":"a.cs","members":[{"type":"int"}]}
  ],
  "edges": [
    {"from":"a","to":"zzz","kind":"references"},
    {"from":"a","to":"b","kind":"calls"}
  ]
}`))
	var fe *FactsError
	if !errors.As(err, &fe) {
		t.Fatalf("want FactsError, got %v", err)
	}
	for _, want := range []string{
		"`root` is required", `language "C#"`, `kind "class" is not one of`, "forward slashes",
		"members[0] has no name", "not sorted by id", "to is not a symbol", `kind "calls"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing problem %q in:\n%v", want, err)
		}
	}
}

func TestReadFactsAcceptsValidOutput(t *testing.T) {
	f := facts(t, adoptFacts)
	if len(f.Symbols) != 5 || len(f.Edges) != 6 {
		t.Fatalf("got %d symbols, %d edges", len(f.Symbols), len(f.Edges))
	}
}

// -------------------------------------------------------------------- sync

func TestSyncFirstRunAdoptsHandMadeEntities(t *testing.T) {
	ws, dir := workspace(t, adoptProject, adoptEntities, adoptRelations, adoptTypes)
	text := `{"contractVersion":3,"language":"ru","entries":{"e_x":{"description":{"v":"x","at":"2026-09-23T00:00:00Z","origin":"authored"}}}}`
	viewDoc := `{"id":"v_main","project":"p","axis":"axis_layer","zones":[],"nodes":[{"entity":"e_x","zone":null,"x":1,"y":2}]}`
	writeFile(t, filepath.Join(dir, "text.ru.json"), text)
	writeFile(t, filepath.Join(dir, "views", "v_main.view.json"), viewDoc)

	rep := sync(t, ws, facts(t, adoptFacts), SyncOptions{})
	if rep.ExitCode() != 0 {
		var b bytes.Buffer
		rep.Print(&b)
		t.Fatalf("exit %d:\n%s", rep.ExitCode(), b.String())
	}
	if rep.Symbols != 4 {
		t.Errorf("include filter: want 4 symbols under src, got %d", rep.Symbols)
	}
	v := load(t, dir)

	asm := find(v.Entities, "e_asm_onnx")
	if asm["symbol"] != "NeuroModFlowNet.ONNX" || asm["kind"] != "assembly" {
		t.Errorf("assembly not adopted by codeRef+name: %v", asm)
	}
	x := find(v.Entities, "e_x")
	if x["symbol"] != "N.X" || x["origin"] != "code" || x["codeRef"] != "src/NeuroModFlowNet.ONNX/X.cs" || x["note"] != "kept" {
		t.Errorf("class not adopted by namespace+name+kind, or a field lost: %v", x)
	}
	if find(v.Entities, "e_x_2") != nil || find(v.Entities, "e_assembly_neuromodflownet_onnx") != nil {
		t.Error("an adopted entity was duplicated")
	}

	guard := find(v.Entities, "e_guard")
	if guard["origin"] != "authored" || guard["kind"] != "concept" || guard["symbol"] != nil {
		t.Errorf("authored entity touched: %v", guard)
	}
	guard2 := find(v.Entities, "e_guard_2")
	if guard2 == nil || guard2["symbol"] != "N.Guard" || guard2["kind"] != "class" || guard2["status"] != "present" {
		t.Fatalf("new entity for N.Guard: want e_guard_2, got %v", guard2)
	}
	if members, _ := guard2["members"].([]any); len(members) != 1 {
		t.Errorf("members not copied: %v", guard2["members"])
	}
	if find(v.Entities, "e_base") == nil {
		t.Error("e_base not minted")
	}
	if find(v.Entities, "e_t") != nil {
		t.Error("a symbol outside sources.include became an entity")
	}

	contains := find(v.Relations, "r_asm_onnx_x_contains")
	if contains == nil || contains["type"] != "contains" || contains["origin"] != "code" || contains["from"] != "e_asm_onnx" {
		t.Fatalf("contains relation: %v", contains)
	}
	ev := contains["evidence"].([]any)[0].(map[string]any)
	if ev["codeRef"] != "src/NeuroModFlowNet.ONNX/NeuroModFlowNet.ONNX.csproj" || ev["symbol"] != "NeuroModFlowNet.ONNX" {
		t.Errorf("evidence: %v", ev)
	}
	if find(v.Relations, "r_x_base_extends") == nil || find(v.Relations, "r_asm_onnx_guard_2_contains") == nil {
		t.Errorf("relations: %v", v.Relations)
	}
	if ref := find(v.Relations, "r_x_guard_2_guard"); ref == nil || ref["type"] != "uses" || ref["origin"] != "code" {
		t.Errorf("uses relation: %v", v.Relations)
	}
	if rt := find(v.Types, "uses"); rt == nil || rt["visibility"] != "hidden" {
		t.Errorf("uses type not declared: %v", v.Types)
	}
	if find(v.Relations, "r_hand") == nil {
		t.Error("authored relation lost")
	}

	if find(v.Types, "contains") == nil || find(v.Types, "call") == nil {
		t.Errorf("relation types: %v", v.Types)
	}
	if find(v.Types, "implements") != nil {
		t.Error("an unused structural type was declared")
	}
	if n := strings.Count(readFile(t, filepath.Join(dir, "relation-types.json")), `"extends"`); n != 1 {
		t.Errorf("extends declared %d times", n)
	}

	if readFile(t, filepath.Join(dir, "text.ru.json")) != text || readFile(t, filepath.Join(dir, "views", "v_main.view.json")) != viewDoc {
		t.Error("sync wrote a text catalogue or a view")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "entities.json")), "Task<int>") {
		t.Error("members were HTML-escaped")
	}

	// A second run with the same facts finds nothing and writes nothing.
	before := readFile(t, filepath.Join(dir, "entities.json"))
	again := sync(t, ws, facts(t, adoptFacts), SyncOptions{DryRun: true})
	if !again.Empty() || again.ExitCode() != 0 {
		var b bytes.Buffer
		again.Print(&b)
		t.Errorf("second run not clean:\n%s", b.String())
	}
	if sync(t, ws, facts(t, adoptFacts), SyncOptions{}); readFile(t, filepath.Join(dir, "entities.json")) != before {
		t.Error("a clean run rewrote entities.json")
	}
}

// After the first run the ids are held by `symbol`: renaming the class in code
// keeps nothing by name, and a changed name alone must not re-mint.
func TestSyncMatchesBySymbolNotByName(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, `{"entities":[
    {"id":"e_legacy_name","name":"Legacy","kind":"class","origin":"code","symbol":"A.C","codeRef":"a/Old.cs"}]}`, "", "")
	rep := sync(t, ws, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"A.C","kind":"type","nativeKind":"record","name":"C","namespace":"A","file":"a/C.cs"}],"edges":[]}`), SyncOptions{})
	e := find(load(t, dir).Entities, "e_legacy_name")
	if e["name"] != "Legacy" || e["kind"] != "class" || e["codeRef"] != "a/C.cs" || e["namespace"] != "A" {
		t.Errorf("entity: %v", e)
	}
	if len(rep.Added) != 0 {
		t.Errorf("minted: %v", rep.Added)
	}
}

func TestSyncMissingThenReturns(t *testing.T) {
	ws, dir := workspace(t, adoptProject, adoptEntities, adoptRelations, adoptTypes)
	sync(t, ws, facts(t, adoptFacts), SyncOptions{})

	// N.Base is gone from the code (its file too): entity and relations go missing.
	withoutBase := strings.NewReplacer(
		`{"id":"N.Base","kind":"type","nativeKind":"class","name":"Base","namespace":"N","file":"src/NeuroModFlowNet.ONNX/Base.cs"},`, "",
		`{"from":"N.X","to":"N.Base","kind":"extends"},`, "",
		`{"from":"NeuroModFlowNet.ONNX","to":"N.Base","kind":"contains"},`, "",
	).Replace(adoptFacts)
	dry := sync(t, ws, facts(t, withoutBase), SyncOptions{DryRun: true})
	if dry.ExitCode() != 1 || len(dry.Gone) != 3 {
		t.Fatalf("dry run: exit %d, gone %v", dry.ExitCode(), dry.Gone)
	}
	if find(load(t, dir).Entities, "e_base")["status"] != "present" {
		t.Fatal("--dry-run wrote")
	}

	rep := sync(t, ws, facts(t, withoutBase), SyncOptions{})
	v := load(t, dir)
	if base := find(v.Entities, "e_base"); base == nil || base["status"] != "missing" {
		t.Errorf("e_base: %v", base)
	}
	if r := find(v.Relations, "r_x_base_extends"); r == nil || r["status"] != "missing" {
		t.Errorf("r_x_base_extends: %v", r)
	}
	if rep.ExitCode() != 0 {
		t.Errorf("exit %d", rep.ExitCode())
	}

	// Back in the code: same ids, present again, nothing minted.
	back := sync(t, ws, facts(t, adoptFacts), SyncOptions{})
	v = load(t, dir)
	if find(v.Entities, "e_base")["status"] != "present" || find(v.Relations, "r_x_base_extends")["status"] != "present" {
		t.Error("did not return to present")
	}
	if len(back.Added) != 0 {
		t.Errorf("minted on return: %v", back.Added)
	}
}

func TestSyncRenameIsOnlyACandidate(t *testing.T) {
	ws, dir := workspace(t, adoptProject, adoptEntities, adoptRelations, adoptTypes)
	sync(t, ws, facts(t, adoptFacts), SyncOptions{})

	// Base renamed to Root in the same file.
	renamed := strings.NewReplacer(
		`"id":"N.Base","kind":"type","nativeKind":"class","name":"Base"`, `"id":"N.Root","kind":"type","nativeKind":"class","name":"Root"`,
		`"to":"N.Base"`, `"to":"N.Root"`,
	).Replace(adoptFacts)
	// Keep the facts sorted: N.Root sorts after N.Guard.
	renamed = strings.Replace(renamed,
		`{"id":"N.Root","kind":"type","nativeKind":"class","name":"Root","namespace":"N","file":"src/NeuroModFlowNet.ONNX/Base.cs"},
    {"id":"N.Guard"`, `{"id":"N.Guard"`, 1)
	renamed = strings.Replace(renamed, `{"id":"N.X",`,
		`{"id":"N.Root","kind":"type","nativeKind":"class","name":"Root","namespace":"N","file":"src/NeuroModFlowNet.ONNX/Base.cs"},
    {"id":"N.X",`, 1)
	renamed = strings.Replace(renamed,
		`{"from":"NeuroModFlowNet.ONNX","to":"N.Root","kind":"contains"},
    {"from":"NeuroModFlowNet.ONNX","to":"N.Guard","kind":"contains"},`,
		`{"from":"NeuroModFlowNet.ONNX","to":"N.Guard","kind":"contains"},
    {"from":"NeuroModFlowNet.ONNX","to":"N.Root","kind":"contains"},`, 1)
	renamed = strings.Replace(renamed,
		`{"from":"N.X","to":"N.Root","kind":"extends"},
    {"from":"N.X","to":"N.Guard","kind":"uses","via":{"member":"guard","memberKind":"field","text":"Guard"}},`,
		`{"from":"N.X","to":"N.Guard","kind":"uses","via":{"member":"guard","memberKind":"field","text":"Guard"}},
    {"from":"N.X","to":"N.Root","kind":"extends"},`, 1)

	rep := sync(t, ws, facts(t, renamed), SyncOptions{})
	if len(rep.Renames) != 1 || !strings.Contains(rep.Renames[0], "e_base (Base) → N.Root") || rep.ExitCode() != 1 {
		t.Fatalf("renames %v, exit %d", rep.Renames, rep.ExitCode())
	}
	v := load(t, dir)
	if find(v.Entities, "e_base")["status"] != "present" || find(v.Entities, "e_root") != nil {
		t.Error("a rename candidate was decided by sync")
	}
	if find(v.Relations, "r_x_base_extends")["status"] != "present" {
		t.Error("relation of a held entity marked missing")
	}

	// The human says "not a rename".
	sync(t, ws, facts(t, renamed), SyncOptions{NoRenames: true})
	v = load(t, dir)
	if find(v.Entities, "e_base")["status"] != "missing" || find(v.Entities, "e_root") == nil {
		t.Error("--no-renames did not apply gone + new")
	}
	if find(v.Relations, "r_x_root_extends") == nil || find(v.Relations, "r_x_base_extends")["status"] != "missing" {
		t.Errorf("relations after --no-renames: %v", v.Relations)
	}
}

func TestSyncConfirmedRenameKeepsID(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, `{"entities":[
    {"id":"e_old","name":"Old","kind":"class","origin":"code","symbol":"A.Old","codeRef":"a/F.cs"}]}`, "", "")
	next := `{"language":"csharp","root":".","symbols":[
    {"id":"A.New","kind":"type","nativeKind":"class","name":"New","namespace":"A","file":"a/F.cs"}],"edges":[]}`
	if rep := sync(t, ws, facts(t, next), SyncOptions{}); len(rep.Renames) != 1 {
		t.Fatalf("renames: %v", rep.Renames)
	}
	// What the report tells the human to do: point the old entity at the new symbol.
	writeFile(t, filepath.Join(dir, "entities.json"), `{"entities":[
    {"id":"e_old","name":"New","kind":"class","origin":"code","namespace":"A","symbol":"A.New","codeRef":"a/F.cs"}]}`)
	rep := sync(t, ws, facts(t, next), SyncOptions{DryRun: true})
	if !rep.Empty() {
		t.Errorf("after confirming: %+v", rep)
	}
}

func TestSyncAmbiguousAdoptionHoldsBothSides(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, `{"entities":[
    {"id":"e_a1","name":"A","kind":"class","namespace":"N"},
    {"id":"e_a2","name":"A","kind":"class","namespace":"N"}]}`, "", "")
	rep := sync(t, ws, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"N.A","kind":"type","nativeKind":"class","name":"A","namespace":"N","file":"n/A.cs"}],"edges":[]}`), SyncOptions{})
	if len(rep.Ambiguous) != 2 || rep.ExitCode() != 1 {
		t.Fatalf("ambiguous %v, exit %d", rep.Ambiguous, rep.ExitCode())
	}
	if n := len(load(t, dir).Entities); n != 2 {
		t.Errorf("held symbol was minted anyway: %d entities", n)
	}
}

func TestSyncBrokenRegistryWritesNothing(t *testing.T) {
	entities := `{"entities":[{"id":"e_a","name":"A","kind":"class"},{"id":"e_a","name":"B","kind":"class"}]}`
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, entities, "", "")
	rep := sync(t, ws, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"C","kind":"type","nativeKind":"class","name":"C","file":"C.cs"}],"edges":[]}`), SyncOptions{})
	if len(rep.Broken) != 1 || rep.ExitCode() != 1 {
		t.Fatalf("broken %v", rep.Broken)
	}
	if readFile(t, filepath.Join(dir, "entities.json")) != entities {
		t.Error("wrote on a broken registry")
	}
}

func TestSyncTypeScriptFileModuleAndClassOfOneName(t *testing.T) {
	// A TS file module and its class share file and name; the hand entity's
	// kind picks the class.
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, `{"entities":[
    {"id":"e_canvas","name":"DiagramCanvas","kind":"class","codeRef":"src/DiagramCanvas.ts"}]}`, "", "")
	sync(t, ws, facts(t, `{"language":"typescript","root":".","symbols":[
    {"id":"src/DiagramCanvas","kind":"module","nativeKind":"file","name":"DiagramCanvas","file":"src/DiagramCanvas.ts"},
    {"id":"src/DiagramCanvas#DiagramCanvas","kind":"type","nativeKind":"class","name":"DiagramCanvas","file":"src/DiagramCanvas.ts"}],
    "edges":[{"from":"src/DiagramCanvas","to":"src/DiagramCanvas#DiagramCanvas","kind":"contains"}]}`), SyncOptions{})
	v := load(t, dir)
	if find(v.Entities, "e_canvas")["symbol"] != "src/DiagramCanvas#DiagramCanvas" {
		t.Errorf("e_canvas: %v", find(v.Entities, "e_canvas"))
	}
	file := find(v.Entities, "e_file_src_diagramcanvas")
	if file == nil || file["kind"] != "file" {
		t.Fatalf("file module: %v", v.Entities)
	}
	if r := find(v.Relations, "r_file_src_diagramcanvas_canvas_contains"); r == nil {
		t.Errorf("relations: %v", v.Relations)
	}
}

// A hand-written registry names generics with their parameter list and says
// `class` for what the extractor calls `abstract-class` / `static-class`.
// Both rules must see through that; kind stays the human's.
func TestSyncAdoptsGenericsAndModifierKinds(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, `{"entities":[
    {"id":"e_runner","name":"IRunner<in TIn, out TOut>","kind":"interface","namespace":"N","codeRef":"src/IRunner.cs","origin":"code"},
    {"id":"e_conv","name":"ConverterBase<TIn>","kind":"class","namespace":"N","codeRef":"src/Old/ConverterBase.cs","origin":"code"},
    {"id":"e_nested","name":"Outer<T>","kind":"class","namespace":"N","codeRef":"src/Outer.cs","origin":"code"},
    {"id":"e_util","name":"Util","kind":"class","namespace":"N","origin":"code"},
    {"id":"e_pt","name":"Point","kind":"record-struct","namespace":"N","codeRef":"src/Point.cs","origin":"code"}]}`, "", "")
	rep := sync(t, ws, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"N.ConverterBase`+"`"+`1","kind":"type","nativeKind":"abstract-class","name":"ConverterBase","namespace":"N","file":"src/ConverterBase.cs"},
    {"id":"N.IRunner`+"`"+`2","kind":"interface","nativeKind":"interface","name":"IRunner","namespace":"N","file":"src/IRunner.cs"},
    {"id":"N.Outer`+"`"+`1","kind":"type","nativeKind":"sealed-class","name":"Outer","namespace":"N","file":"src/Outer.cs"},
    {"id":"N.Point","kind":"type","nativeKind":"record-struct","name":"Point","namespace":"N","file":"src/Point.cs"},
    {"id":"N.Util","kind":"type","nativeKind":"static-class","name":"Util","namespace":"N","file":"src/Util.cs"}],"edges":[]}`), SyncOptions{})
	if !(len(rep.Renames) == 0 && len(rep.Gone) == 0 && len(rep.Added) == 0 && len(rep.Ambiguous) == 0) {
		var b bytes.Buffer
		rep.Print(&b)
		t.Fatalf("not all adopted:\n%s", b.String())
	}
	v := load(t, dir)
	for id, want := range map[string]string{
		"e_runner": "N.IRunner`2",       // rule 1, generic name
		"e_conv":   "N.ConverterBase`1", // rule 2: file moved, generic name, abstract-class ~ class
		"e_nested": "N.Outer`1",         // rule 1, sealed-class
		"e_util":   "N.Util",            // rule 2, no codeRef, static-class ~ class
		"e_pt":     "N.Point",
	} {
		e := find(v.Entities, id)
		if e["symbol"] != want {
			t.Errorf("%s: symbol %v, want %s", id, e["symbol"], want)
		}
	}
	if e := find(v.Entities, "e_conv"); e["kind"] != "class" || e["name"] != "ConverterBase<TIn>" {
		t.Errorf("kind or name overwritten: %v", e)
	}
}

func TestBaseNameAndKindFamily(t *testing.T) {
	for in, want := range map[string]string{
		"IRunner<in TIn, out TOut>":      "IRunner",
		"Map<Dictionary<K, V>, List<T>>": "Map",
		"IRunner`2":                      "IRunner",
		"Plain":                          "Plain",
		"Weird<":                         "Weird<",
		"Op<>":                           "Op",
	} {
		if got := baseName(in); got != want {
			t.Errorf("baseName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"abstract-class": "class", "static-class": "class", "Class": "class", "record-struct": "struct", "": "",
	} {
		if got := kindFamily(in); got != want {
			t.Errorf("kindFamily(%q) = %q, want %q", in, got, want)
		}
	}
}

// Module entities are named from the whole symbol id with the native kind in
// front: short names like `Diagnostics` repeat and a namespace and an assembly
// often share a name.
func TestSyncModuleIDsFromFullName(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, `{"entities":[]}`, "", "")
	sync(t, ws, facts(t, `{"language":"csharp","root":".","symbols":[
    {"id":"A.Diagnostics","kind":"module","nativeKind":"namespace","name":"Diagnostics","file":"src/A.Diagnostics/X.cs"},
    {"id":"A.Diagnostics.X","kind":"type","nativeKind":"class","name":"X","namespace":"A.Diagnostics","file":"src/A.Diagnostics/X.cs"},
    {"id":"B.Diagnostics","kind":"module","nativeKind":"namespace","name":"Diagnostics","file":"src/B/Y.cs"},
    {"id":"[A.Diagnostics]","kind":"module","nativeKind":"assembly","name":"A.Diagnostics","file":"src/A.Diagnostics/A.Diagnostics.csproj"}],
    "edges":[{"from":"A.Diagnostics","to":"A.Diagnostics.X","kind":"contains"},
    {"from":"[A.Diagnostics]","to":"A.Diagnostics.X","kind":"contains"}]}`), SyncOptions{})
	v := load(t, dir)
	for _, id := range []string{
		"e_namespace_a_diagnostics", "e_namespace_b_diagnostics", "e_assembly_a_diagnostics", "e_x",
		"r_namespace_a_diagnostics_x_contains", "r_assembly_a_diagnostics_x_contains",
	} {
		if find(v.Entities, id) == nil && find(v.Relations, id) == nil {
			t.Errorf("%s not written; entities %v", id, v.Entities)
		}
	}
	if e := find(v.Entities, "e_namespace_b_diagnostics"); e["name"] != "Diagnostics" {
		t.Errorf("name is the extractor's: %v", e)
	}
}

func TestSyncNeedsProjectWhenSeveral(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")
	writeFile(t, filepath.Join(ws, "projects", "q", "project.json"), `{"id":"q","contractVersion":3}`)
	_, err := Sync(ws, facts(t, `{"language":"go","root":".","symbols":[],"edges":[]}`), SyncOptions{})
	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("want UsageError, got %v", err)
	}
}

// Member relations: type derivation and ID generation
func TestMemberRelationTypeDerivation(t *testing.T) {
	for _, tc := range []struct {
		kind       string
		via        *Via
		wantType   string
		wantPublic bool
	}{
		{"uses", &Via{MemberKind: "constructor"}, "injects", true},
		{"uses", &Via{MemberKind: "parameter"}, "uses", true},
		{"holds", &Via{Cardinality: "one", Modifiers: []string{"public"}}, "holds.one", true},
		{"holds", &Via{Cardinality: "optional", Modifiers: []string{"public"}}, "holds.optional", true},
		{"holds", &Via{Cardinality: "many", Mutability: "mutable", Modifiers: []string{"public"}}, "holds.many", true},
		{"holds", &Via{Cardinality: "many", Mutability: "readonly", Modifiers: []string{"public"}}, "holds.many.ro", true},
		{"holds", &Via{Cardinality: "keyed", Mutability: "mutable", Modifiers: []string{"public"}}, "holds.keyed", true},
		{"holds", &Via{Cardinality: "keyed", Mutability: "readonly", Modifiers: []string{"public"}}, "holds.keyed.ro", true},
		{"holds", &Via{Cardinality: "one", Modifiers: []string{"private"}}, "holds.one.internal", false},
		{"holds", &Via{Cardinality: "many", Mutability: "mutable", Modifiers: []string{"private"}}, "holds.many.internal", false},
		{"holds", nil, "holds.one", true}, // no via: assume public
	} {
		got := deriveRelationType(tc.kind, tc.via)
		if got != tc.wantType {
			t.Errorf("deriveRelationType(%q, %v) = %q, want %q", tc.kind, tc.via, got, tc.wantType)
		}
		gotPublic := isPublicMember(tc.via)
		if gotPublic != tc.wantPublic {
			t.Errorf("isPublicMember(%v) = %v, want %v", tc.via, gotPublic, tc.wantPublic)
		}
	}
}

// Member relation ID is stable across wrapper changes (List -> IReadOnlyList).
func TestMemberRelationIDStability(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")
	facts1 := `{"language":"csharp","root":".","edgeKinds":["holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[{"from":"A","to":"B","kind":"holds","via":{"member":"items","memberKind":"field","cardinality":"many","text":"List<B>"}}]}`
	sync(t, ws, facts(t, facts1), SyncOptions{})
	v := load(t, dir)
	relID := find(v.Relations, "r_a_b_items")
	if relID == nil || relID["type"] != "holds.many" {
		t.Fatalf("first run: %v", v.Relations)
	}

	// Same edge, different wrapper: IReadOnlyList -> holds.many.ro
	facts2 := `{"language":"csharp","root":".","edgeKinds":["holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[{"from":"A","to":"B","kind":"holds","via":{"member":"items","memberKind":"field","cardinality":"many","mutability":"readonly","text":"IReadOnlyList<B>"}}]}`
	sync(t, ws, facts(t, facts2), SyncOptions{})
	v = load(t, dir)
	updated := find(v.Relations, "r_a_b_items")
	if updated == nil || updated["type"] != "holds.many.ro" || updated["id"] != "r_a_b_items" {
		t.Errorf("second run: id changed or type not updated: %v", updated)
	}
}

// Visibility is set when new relation types are created.
func TestMemberRelationVisibility(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")
	factSet := `{"language":"csharp","root":".","edgeKinds":["holds","uses"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"},
    {"id":"C","kind":"type","nativeKind":"class","name":"C","file":"c.cs"}],
    "edges":[
      {"from":"A","to":"B","kind":"holds","via":{"member":"pub","memberKind":"field","cardinality":"one","modifiers":["public"]}},
      {"from":"A","to":"B","kind":"uses","via":{"member":"Method","memberKind":"parameter"}},
      {"from":"A","to":"C","kind":"holds","via":{"member":"priv","memberKind":"field","cardinality":"one","modifiers":["private"]}}
    ]}`
	sync(t, ws, facts(t, factSet), SyncOptions{})
	v := load(t, dir)
	for _, tc := range []struct {
		typeID    string
		wantVisib string
	}{
		{"holds.one", "visible"},
		{"holds.one.internal", "hidden"},
		{"uses", "hidden"},
	} {
		rt := find(v.Types, tc.typeID)
		if rt == nil {
			t.Errorf("type %s not created", tc.typeID)
			continue
		}
		if rt["visibility"] != tc.wantVisib {
			t.Errorf("type %s visibility = %v, want %v", tc.typeID, rt["visibility"], tc.wantVisib)
		}
	}
}

// missing marking only for edge kinds in edgeKinds.
func TestMemberRelationMissingByEdgeKinds(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")

	// First run: extract both holds and uses
	facts1 := `{"language":"csharp","root":".","edgeKinds":["holds","uses"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[
      {"from":"A","to":"B","kind":"holds","via":{"member":"field1","memberKind":"field"}},
      {"from":"A","to":"B","kind":"uses","via":{"member":"Method","memberKind":"parameter"}}
    ]}`
	rep1 := sync(t, ws, facts(t, facts1), SyncOptions{})
	if len(rep1.Added) == 0 {
		t.Fatalf("first sync did not create relations: %v", rep1)
	}

	// Verify relations were created
	v1 := load(t, ws+"/projects/p")
	if find(v1.Relations, "r_a_b_field1") == nil {
		t.Fatalf("holds relation not created in first sync: %v", v1.Relations)
	}
	if find(v1.Relations, "r_a_b_method") == nil {
		t.Fatalf("uses relation not created in first sync: %v", v1.Relations)
	}

	// Second run: extract holds, and say we cover uses too (but find nothing)
	facts2 := `{"language":"csharp","root":".","edgeKinds":["holds","uses"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[
      {"from":"A","to":"B","kind":"holds","via":{"member":"field1","memberKind":"field"}}
    ]}`
	rep := sync(t, ws, facts(t, facts2), SyncOptions{DryRun: true})

	// uses edge should be marked missing (was extracted in first run, edgeKinds says we looked for it but found nothing)
	hasUsesGone := false
	for _, gone := range rep.Gone {
		if strings.Contains(gone, "r_a_b_method") {
			hasUsesGone = true
			break
		}
	}
	if !hasUsesGone {
		t.Errorf("uses edge (r_a_b_method) should be marked missing: gone=%v", rep.Gone)
	}
}

// Rename candidates for member relations: same (from, to, path), different member.
func TestMemberRelationRenameCandidate(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")

	// First run: create a member relation
	facts1 := `{"language":"csharp","root":".","edgeKinds":["holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[
      {"from":"A","to":"B","kind":"holds","via":{"member":"oldName","memberKind":"field"}}
    ]}`
	sync(t, ws, facts(t, facts1), SyncOptions{})

	// Second run: member renamed in code
	facts2 := `{"language":"csharp","root":".","edgeKinds":["holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[
      {"from":"A","to":"B","kind":"holds","via":{"member":"newName","memberKind":"field"}}
    ]}`
	rep := sync(t, ws, facts(t, facts2), SyncOptions{DryRun: true})

	// Should report as rename candidate
	if len(rep.Renames) != 1 || !strings.Contains(rep.Renames[0], "oldName") || !strings.Contains(rep.Renames[0], "newName") {
		t.Errorf("rename candidate not reported: %v", rep.Renames)
	}
	if rep.ExitCode() != 1 {
		t.Errorf("should have exit code 1 for rename candidate, got %d", rep.ExitCode())
	}

	// With --no-renames, should apply both gone and new
	sync(t, ws, facts(t, facts2), SyncOptions{NoRenames: true})
	v := load(t, ws+"/projects/p")
	if find(v.Relations, "r_a_b_oldname")["status"] != "missing" {
		t.Error("old relation should be marked missing after --no-renames")
	}
	if find(v.Relations, "r_a_b_newname") == nil {
		t.Error("new relation should be created after --no-renames")
	}
}

func TestLegacyReferencesRelationBecomesMissing(t *testing.T) {
	// When old code relations (e.g. "references" from previous extractor version)
	// are not confirmed by new facts, they should be marked as missing.
	// This tests the fix for the bug where legacy relations would stay present forever.
	ws, dir := workspace(t, adoptProject, `{"entities":[
    {"id":"e_a","name":"A","kind":"class","origin":"code","symbol":"N.A","codeRef":"src/A.cs"},
    {"id":"e_b","name":"B","kind":"class","origin":"code","symbol":"N.B","codeRef":"src/B.cs"}
  ]}`, `{"contractVersion":3,"relations":[
    {"id":"r_a_b_references","from":"e_a","to":"e_b","type":"references","origin":"code","status":"present"}
  ]}`, "")

	// First sync: establish the entities and legacy relation
	sync(t, ws, facts(t, `{"language":"csharp","root":".","edgeKinds":["extends","implements","contains","depends","holds","uses"],
  "symbols":[
    {"id":"N.A","kind":"type","nativeKind":"class","name":"A","namespace":"N","file":"src/A.cs"},
    {"id":"N.B","kind":"type","nativeKind":"class","name":"B","namespace":"N","file":"src/B.cs"}
  ],"edges":[]}`), SyncOptions{})

	v := load(t, dir)
	ref := find(v.Relations, "r_a_b_references")
	if ref == nil {
		t.Fatal("legacy references relation not found")
	}
	if ref["status"] != "missing" {
		t.Errorf("legacy references relation should be marked missing, got status: %v", ref["status"])
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"NeuroModFlowNet.ONNX": "neuromodflownet_onnx",
		"RepetitionGuard":      "repetitionguard",
		"__x--y__":             "x_y",
		"Схема":                "схема",
		"...":                  "",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// A field and the constructor parameter that fills it share a name; they are
// two relations, and a second sync of the same facts changes nothing.
// (NeuroModFlowNet: IouTracker.options, a record's positional property.)
func TestMemberRelationFieldAndCtorParamAreTwo(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")
	f := `{"language":"csharp","root":".","edgeKinds":["holds","injects"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[
      {"from":"A","to":"B","kind":"holds","via":{"member":"options","memberKind":"field","cardinality":"one","modifiers":["private","readonly"]}},
      {"from":"A","to":"B","kind":"uses","via":{"member":"options","memberKind":"constructor"}}
    ]}`
	sync(t, ws, facts(t, f), SyncOptions{})
	v := load(t, dir)
	types := map[string]bool{}
	for _, r := range v.Relations {
		types[r["type"].(string)] = true
	}
	if len(v.Relations) != 2 || !types["holds.one.internal"] || !types["injects"] {
		t.Fatalf("want a holds and an injects relation: %v", v.Relations)
	}
	rep := sync(t, ws, facts(t, f), SyncOptions{DryRun: true})
	if len(rep.Changed)+len(rep.Added)+len(rep.Gone) != 0 {
		t.Fatalf("second sync is not quiet: changed %v added %v gone %v", rep.Changed, rep.Added, rep.Gone)
	}
}

// A rename in the same edit as a wrapper change (List -> IReadOnlyList) is
// still offered as a rename: candidates are matched by family, not exact type.
func TestMemberRelationRenameWithWrapperChange(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":3}`, "", "", "")
	before := `{"language":"csharp","root":".","edgeKinds":["holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[{"from":"A","to":"B","kind":"holds","via":{"member":"items","memberKind":"field","cardinality":"many","path":["item"]}}]}`
	after := `{"language":"csharp","root":".","edgeKinds":["holds"],"symbols":[
    {"id":"A","kind":"type","nativeKind":"class","name":"A","file":"a.cs"},
    {"id":"B","kind":"type","nativeKind":"class","name":"B","file":"b.cs"}],
    "edges":[{"from":"A","to":"B","kind":"holds","via":{"member":"entries","memberKind":"field","cardinality":"many","mutability":"readonly","path":["item"]}}]}`
	sync(t, ws, facts(t, before), SyncOptions{})
	rep := sync(t, ws, facts(t, after), SyncOptions{DryRun: true})
	if len(rep.Renames) == 0 {
		t.Fatalf("no rename candidate: %+v", rep)
	}
}
