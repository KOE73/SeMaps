package core

import "testing"

// TestSyncIgnoresPositionFields proves PLAN_20260928-3 step 1-2: facts carrying
// the new position fields (endLine, spans, memberLines on a symbol; line, file
// on an edge) sync to a registry byte-identical to the same facts without
// them. These fields are place in the code, not part of the registry.
func TestSyncIgnoresPositionFields(t *testing.T) {
	const factsWithout = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["extends","implements","contains","depends","holds","uses"],
  "symbols": [
    {"id":"N.Base","kind":"type","nativeKind":"class","name":"Base","namespace":"N","file":"src/Base.cs"},
    {"id":"N.Guard","kind":"type","nativeKind":"class","name":"Guard","namespace":"N","file":"src/Guard.cs",
     "members":[{"kind":"property","name":"Limit","type":"int","visibility":"public"}]},
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/X.cs",
     "members":[{"kind":"method","name":"Run","type":"Task<int>","visibility":"public"}]}
  ],
  "edges": [
    {"from":"N.X","to":"N.Base","kind":"extends"},
    {"from":"N.X","to":"N.Guard","kind":"uses","via":{"member":"guard","memberKind":"field","text":"Guard"}}
  ]
}`
	const factsWith = `{
  "language": "csharp", "root": ".",
  "edgeKinds": ["extends","implements","contains","depends","holds","uses"],
  "symbols": [
    {"id":"N.Base","kind":"type","nativeKind":"class","name":"Base","namespace":"N","file":"src/Base.cs",
     "line":1,"endLine":9},
    {"id":"N.Guard","kind":"type","nativeKind":"class","name":"Guard","namespace":"N","file":"src/Guard.cs",
     "line":3,"endLine":20,
     "members":[{"kind":"property","name":"Limit","type":"int","visibility":"public"}],
     "memberLines":{"Limit":5}},
    {"id":"N.X","kind":"type","nativeKind":"class","name":"X","namespace":"N","file":"src/X.cs",
     "line":1,"endLine":30,
     "spans":[{"file":"src/X.a.cs","line":1,"endLine":30},{"file":"src/X.cs","line":1,"endLine":10}],
     "members":[{"kind":"method","name":"Run","type":"Task<int>","visibility":"public"}],
     "memberLines":{"Run":7}}
  ],
  "edges": [
    {"from":"N.X","to":"N.Base","kind":"extends","line":1},
    {"from":"N.X","to":"N.Guard","kind":"uses","via":{"member":"guard","memberKind":"field","text":"Guard"},
     "line":8,"file":"src/X.a.cs"}
  ]
}`

	project := `{"id":"p","contractVersion":3,"sources":{"include":["src"]}}`

	wsWithout, dirWithout := workspace(t, project, "", "", "")
	sync(t, wsWithout, facts(t, factsWithout), SyncOptions{})

	wsWith, dirWith := workspace(t, project, "", "", "")
	sync(t, wsWith, facts(t, factsWith), SyncOptions{})

	for _, name := range []string{"entities.json", "relations.json", "relation-types.json"} {
		got := readFile(t, dirWith+"/"+name)
		want := readFile(t, dirWithout+"/"+name)
		if got != want {
			t.Fatalf("%s differs between facts with and without position fields:\nwith:    %s\nwithout: %s", name, got, want)
		}
	}
}
