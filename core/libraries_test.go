package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func libraryFacts(t *testing.T, expanded, complete bool, edge bool) *Facts {
	t.Helper()
	f := &Facts{Language: "csharp", Root: ".", EdgeKinds: []string{"contains", "extends", "implements", "holds", "uses"},
		Libraries: []Library{{ID: "LibA", Name: "LibA", Version: "1.0", Complete: complete}, {ID: "LibB", Name: "LibB", Version: "1.0"}},
		Symbols:   []Symbol{{ID: "[LibA]::N.A", Name: "A", Kind: "type", NativeKind: "class", Library: "LibA", Namespace: "N"}, {ID: "[LibB]::N.B", Name: "B", Kind: "type", NativeKind: "struct", Library: "LibB", Namespace: "N"}, {ID: "N.Local", Name: "Local", Kind: "type", NativeKind: "class", File: "local.cs"}},
		Edges:     []Edge{{From: "N.Local", To: "[LibA]::N.A", Kind: "holds", Via: &Via{Member: "value", MemberKind: "field"}}}}
	if expanded {
		f.Libraries[0].Expanded = []string{"[LibA]::N.A"}
		f.Symbols[0].Members = json.RawMessage(`[{"kind":"field","name":"child","type":"B"}]`)
	}
	if edge {
		f.Edges = append([]Edge{{From: "[LibA]::N.A", To: "[LibB]::N.B", Kind: "holds", Via: &Via{Member: "child", MemberKind: "field"}}}, f.Edges...)
	}
	sort.Slice(f.Symbols, func(i, j int) bool { return f.Symbols[i].ID < f.Symbols[j].ID })
	sort.Slice(f.Edges, func(i, j int) bool { return f.Edges[i].From < f.Edges[j].From })
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLibraryShardsJournalSaveAndDiscard(t *testing.T) {
	ws, dir := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
	m, err := LoadModel(ws, "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	f := libraryFacts(t, true, false, true)
	if r, err := Sync(m, f, SyncOptions{}); err != nil || r.ExitCode() != 0 {
		t.Fatalf("sync %+v %v", r, err)
	}
	if exists(filepath.Join(dir, LibraryFile("LibA"))) {
		t.Fatal("sync wrote contract before Save")
	}
	m, err = LoadModel(ws, "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	records, _ := m.Records("entities.json")
	if len(records) != 3 {
		t.Fatal("journal lost external entities")
	}
	if err = m.Save(); err != nil {
		t.Fatal(err)
	}
	own := readFile(t, filepath.Join(dir, "entities.json"))
	if strings.Contains(own, "LibA") || !strings.Contains(own, "Local") {
		t.Fatal(own)
	}
	if strings.Contains(readFile(t, filepath.Join(dir, "relations.json")), "r_") {
		t.Fatal("external relation in own file")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "relations.external.json")), "value") {
		t.Fatal("missing boundary relation")
	}
	lib := readFile(t, filepath.Join(dir, LibraryFile("LibA")))
	if !strings.Contains(lib, "child") || !strings.Contains(lib, "e_b") {
		t.Fatal(lib)
	}
	m, err = LoadModel(ws, "p", nil)
	if err != nil {
		t.Fatal(err)
	}
	records, _ = Records(ws, "p", "entities.json")
	if len(records) != 3 {
		t.Fatal("CLI records are not combined")
	}
	g, err := BuildGraph([]FactsSource{{Extractor: "cs", Facts: f}}, m)
	if err != nil || len(g.Nodes) != 3 || len(g.Edges) != 2 {
		t.Fatalf("graph %+v %v", g, err)
	}
	before, err := os.Stat(filepath.Join(dir, LibraryFile("LibA")))
	if err != nil {
		t.Fatal(err)
	}
	local := cloneObject(findByID(m.registries["entity"].items, "e_local"))
	local.set("name", "Local edited")
	raw, _ := local.MarshalJSON()
	if _, err = m.Apply([]Op{{Kind: "entity", ID: "e_local", Value: raw}}, "human"); err != nil {
		t.Fatal(err)
	}
	if err = m.Discard("registry", ""); err != nil {
		t.Fatal(err)
	}
	if findByID(m.registries["entity"].items, "e_local").str("name") != "Local" {
		t.Fatal("discard lost partition state")
	}
	if _, err = m.Apply([]Op{{Kind: "entity", ID: "e_local", Value: raw}}, "human"); err != nil {
		t.Fatal(err)
	}
	if err = m.Save(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(dir, LibraryFile("LibA")))
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("source edit rewrote library")
	}
	if readFile(t, filepath.Join(dir, LibraryFile("LibA"))) != lib {
		t.Fatal("library bytes changed")
	}
}

func TestLibraryPartialCoveragePreservesExpandedMembersAndEdges(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
	m, _ := LoadModel(ws, "p", nil)
	if _, err := Sync(m, libraryFacts(t, true, false, true), SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	stub := libraryFacts(t, false, false, false)
	if r, err := Sync(m, stub, SyncOptions{}); err != nil || len(r.Gone) != 0 {
		t.Fatalf("partial %+v %v", r, err)
	}
	a := findByID(m.registries["entity"].items, "e_a")
	if !strings.Contains(string(a.vals["members"]), "child") {
		t.Fatal("stub erased members")
	}
	edge := findByID(m.registries["relation"].items, "r_a_b_child")
	if edge == nil || edge.str("status") != "present" {
		t.Fatal("partial snapshot erased old edge")
	}
	changed := libraryFacts(t, true, false, false)
	if _, err := Sync(m, changed, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if findByID(m.registries["relation"].items, "r_a_b_child").str("status") != "missing" {
		t.Fatal("expanded type did not retire removed edge")
	}
	// A local edge can disappear even while its external target is preserved.
	changed.Edges = nil
	changed.Symbols = changed.Symbols[:1]
	changed.Libraries = nil
	if _, err := Sync(m, changed, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if findByID(m.registries["relation"].items, "r_local_a_value").str("status") != "missing" {
		t.Fatal("removed boundary edge still present")
	}
}

func TestLibraryVersionsAndCompleteCoverage(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
	m, _ := LoadModel(ws, "p", nil)
	f := libraryFacts(t, true, true, true)
	if _, err := Sync(m, f, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	f.Libraries[0].Version = "2.0"
	f.Libraries[0].Complete = false
	if r, err := Sync(m, f, SyncOptions{}); err != nil || len(r.Broken) == 0 {
		t.Fatalf("mixed versions accepted: %+v %v", r, err)
	}
	if m.LibrarySnapshots()[0].Version != "1.0" {
		t.Fatal("rejected batch changed version")
	}
	f.Libraries[0].Complete = true
	f.Libraries[0].Expanded = nil
	f.Symbols = []Symbol{f.Symbols[0], f.Symbols[2]}
	f.Edges = nil
	if _, err := Sync(m, f, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if findByID(m.registries["entity"].items, "e_a").str("status") != "missing" {
		t.Fatal("full assembly did not mark removed type")
	}
}

func TestLibraryGraphSharedAcrossExtractorsAndRetainedWithoutFacts(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
	m, _ := LoadModel(ws, "p", nil)
	f := libraryFacts(t, true, false, true)
	if _, err := Sync(m, f, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	g, err := BuildGraph([]FactsSource{{Extractor: "one", Facts: f}, {Extractor: "two", Facts: f}}, m)
	if err != nil {
		t.Fatal(err)
	}
	// Two local declarations, two shared metadata types, two boundary edges and one library edge.
	if len(g.Nodes) != 4 || len(g.Edges) != 3 {
		t.Fatalf("duplicated library graph: %d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}
	g, err = BuildGraph(nil, m)
	if err != nil {
		t.Fatal(err)
	}
	if err = AttachMembers(g, m); err != nil {
		t.Fatal(err)
	}
	for _, n := range g.Nodes {
		if n.Library == "LibA" && (n.Entity != "e_a" || n.Symbol != "[LibA]::N.A" || n.Kind != "type" || !strings.Contains(string(n.Members), "child")) {
			t.Fatalf("retained metadata node: %+v", n)
		}
	}
}

func TestLibraryMetadataDryRunAndPhysicalReport(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
	m, _ := LoadModel(ws, "p", nil)
	f := &Facts{Language: "csharp", Root: ".", Libraries: []Library{{ID: "Empty", Name: "Empty", Version: "1.0", Complete: true}}}
	r, err := Sync(m, f, SyncOptions{DryRun: true})
	if err != nil || r.Empty() || len(r.Written) != 0 || len(m.LibrarySnapshots()) != 0 {
		t.Fatalf("metadata preview mutated or empty: %+v %v", r, err)
	}
	r, err = Sync(m, libraryFacts(t, true, false, true), SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"projects/p/entities.json", "projects/p/relations.external.json", "projects/p/" + LibraryFile("LibA"), "projects/p/" + LibraryFile("LibB")}
	sort.Strings(want)
	if strings.Join(r.Written, ",") != strings.Join(want, ",") {
		t.Fatalf("written %v, want %v", r.Written, want)
	}
}

func TestLibraryCorruptStorageIsRefused(t *testing.T) {
	for _, payload := range []string{
		`{"library":{"id":"Bad","name":"Bad","version":"1.0","complete":"yes"},"entities":[],"relations":[]}`,
		`{"library":{"id":"Bad","name":"Bad","version":"1.0"},"entities":{},"relations":[]}`,
		`{"library":{"id":"Bad","name":"Bad","version":"1.0"},"entities":[null],"relations":[]}`,
		`{"library":{"id":"Bad","name":"Bad","version":"1.0"},"entities":[]}`,
		`{"library":{"id":"Bad","name":"Bad","version":"1.0"},"entities":[],"relations":[{"id":"r_bad","from":"e_a","to":"e_b","type":"holds"}]}`,
	} {
		ws, dir := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
		file := filepath.Join(dir, LibraryFile("Bad"))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(payload), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadModel(ws, "p", nil); err == nil {
			t.Fatalf("accepted corrupt shard: %s", payload)
		}
	}
}

func TestLibraryInvalidEvidenceBatchIsAtomic(t *testing.T) {
	ws, _ := workspace(t, `{"id":"p","contractVersion":5}`, `{"entities":[]}`, `{"relations":[]}`, "")
	m, _ := LoadModel(ws, "p", nil)
	if _, err := Sync(m, libraryFacts(t, true, false, true), SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	r := cloneObject(findByID(m.registries["relation"].items, "r_a_b_child"))
	c := entries(r, "evidence")[0]
	c.set("library", "Unknown")
	setEntries(r, "evidence", []*object{c})
	raw, _ := r.MarshalJSON()
	if _, err := m.Apply([]Op{{Kind: "relation", ID: "r_a_b_child", Value: raw}}, "human"); err == nil {
		t.Fatal("unknown library accepted")
	}
	if entries(findByID(m.registries["relation"].items, "r_a_b_child"), "evidence")[0].str("library") != "LibA" {
		t.Fatal("refused batch mutated model")
	}
}
