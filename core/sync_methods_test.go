package core

import (
	"bytes"
	"strings"
	"testing"
)

// TestSyncDropsMethodsAndCalls proves the one rule of ADR_20260928-4 §4:
// methods and calls are dynamic data of the live graph and never reach the
// registry. Facts with methods and calls must sync to a registry byte
// identical to the same facts with that dynamic data removed, whether the
// registry starts empty or already has the types.
func TestSyncDropsMethodsAndCalls(t *testing.T) {
	const project = `{"id":"p","contractVersion":3,"sources":{"include":["src"]}}`

	const factsWithoutMethods = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["extends","contains"],
  "symbols": [
    {"id":"N.Base","kind":"type","nativeKind":"class","name":"Base","namespace":"N","file":"src/Base.cs"},
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/X.cs"}
  ],
  "edges": [
    {"from":"N.X","to":"N.Base","kind":"extends"}
  ]
}`

	const factsWithMethods = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["extends","contains","calls"],
  "symbols": [
    {"id":"N.Base","kind":"type","nativeKind":"class","name":"Base","namespace":"N","file":"src/Base.cs"},
    {"id":"N.Base.Run()","kind":"method","nativeKind":"method","name":"Run","namespace":"N","file":"src/Base.cs","line":3},
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/X.cs"},
    {"id":"N.X.Go()","kind":"method","nativeKind":"method","name":"Go","namespace":"N","file":"src/X.cs","line":5}
  ],
  "edges": [
    {"from":"N.Base","to":"N.Base.Run()","kind":"contains"},
    {"from":"N.X","to":"N.Base","kind":"extends"},
    {"from":"N.X","to":"N.X.Go()","kind":"contains"},
    {"from":"N.X.Go()","to":"N.Base.Run()","kind":"calls","line":6,"lines":[6,7]}
  ]
}`

	const entities = `{"entities":[
    {"id":"N.Base","kind":"type","name":"Base"},
    {"id":"N.X","kind":"type","name":"X"}
  ]}`
	const relations = `{"relations":[
    {"id":"r1","from":"N.X","to":"N.Base","type":"extends"}
  ]}`
	const types = `{"relationTypes":[{"id":"extends","name":"extends"}]}`

	t.Run("empty registry", func(t *testing.T) {
		wsWithout, dirWithout := workspace(t, project, "", "", "")
		sync(t, wsWithout, facts(t, factsWithoutMethods), SyncOptions{})

		wsWith, dirWith := workspace(t, project, "", "", "")
		rep := sync(t, wsWith, facts(t, factsWithMethods), SyncOptions{})

		for _, name := range []string{"entities.json", "relations.json", "relation-types.json"} {
			got := readFile(t, dirWith+"/"+name)
			want := readFile(t, dirWithout+"/"+name)
			if got != want {
				t.Fatalf("%s differs between facts with and without methods/calls:\nwith:    %s\nwithout: %s", name, got, want)
			}
		}
		assertNoMethodsOrCalls(t, rep, dirWith)
	})

	t.Run("registry already has the types", func(t *testing.T) {
		wsWithout, dirWithout := workspace(t, project, entities, relations, types)
		sync(t, wsWithout, facts(t, factsWithoutMethods), SyncOptions{})

		wsWith, dirWith := workspace(t, project, entities, relations, types)
		rep := sync(t, wsWith, facts(t, factsWithMethods), SyncOptions{})

		for _, name := range []string{"entities.json", "relations.json", "relation-types.json"} {
			got := readFile(t, dirWith+"/"+name)
			want := readFile(t, dirWithout+"/"+name)
			if got != want {
				t.Fatalf("%s differs between facts with and without methods/calls:\nwith:    %s\nwithout: %s", name, got, want)
			}
		}
		assertNoMethodsOrCalls(t, rep, dirWith)
	})
}

func assertNoMethodsOrCalls(t *testing.T, rep *SyncReport, dir string) {
	t.Helper()
	if rep.Symbols != 2 || rep.Edges != 1 {
		t.Fatalf("report should count only the two types and the extends edge, got Symbols=%d Edges=%d", rep.Symbols, rep.Edges)
	}
	var buf bytes.Buffer
	rep.Print(&buf)
	summary := buf.String()
	for _, word := range []string{"method", "Run()", "Go()", "calls"} {
		if strings.Contains(summary, word) {
			t.Fatalf("sync report mentions %q, methods/calls must never surface:\n%s", word, summary)
		}
	}
	v := load(t, dir)
	for _, e := range v.Entities {
		if e["kind"] == "method" {
			t.Fatalf("registry has a method entity: %v", e)
		}
	}
	for _, rt := range v.Types {
		id, _ := rt["id"].(string)
		if id == "calls" || id == "constructs" || id == "overrides" {
			t.Fatalf("relation-types.json has a %s relation type", id)
		}
	}
}

// TestSyncSecondPassMarksNothingMissingForRemovedMethod covers (d): a method
// present in one facts document and absent from the next must not be marked
// missing, since sync never saw it as a symbol to begin with.
func TestSyncSecondPassMarksNothingMissingForRemovedMethod(t *testing.T) {
	const project = `{"id":"p","contractVersion":3,"sources":{"include":["src"]}}`
	const factsWithMethod = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["contains","calls"],
  "symbols": [
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/X.cs"},
    {"id":"N.X.Go()","kind":"method","nativeKind":"method","name":"Go","namespace":"N","file":"src/X.cs","line":5}
  ],
  "edges": [
    {"from":"N.X","to":"N.X.Go()","kind":"contains"}
  ]
}`
	const factsWithoutMethod = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["contains","calls"],
  "symbols": [
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/X.cs"}
  ],
  "edges": []
}`
	ws, dir := workspace(t, project, "", "", "")
	sync(t, ws, facts(t, factsWithMethod), SyncOptions{})
	rep := sync(t, ws, facts(t, factsWithoutMethod), SyncOptions{})
	if len(rep.Gone) != 0 {
		t.Fatalf("removing a method from the facts must mark nothing missing, got %v", rep.Gone)
	}
	v := load(t, dir)
	var x map[string]any
	for _, e := range v.Entities {
		if e["symbol"] == "N.X" {
			x = e
		}
	}
	if x == nil || x["status"] == "missing" {
		t.Fatalf("N.X must not be marked missing: %v", x)
	}
}
