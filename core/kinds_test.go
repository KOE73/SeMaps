package core

import (
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// defaultKindsJSON is the tool's shipped dictionary — the one definition.
func defaultKindsJSON(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "host", "defaults", KindsFile))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testCatalog(t testing.TB) *KindCatalog {
	t.Helper()
	c, err := ParseKinds(defaultKindsJSON(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDefaultKinds(t *testing.T) {
	c := testCatalog(t)
	for _, k := range []string{"namespace", "package", "module", "assembly", "crate", "file", "system", "subsystem", "layer", "boundary", "group"} {
		if !c.IsContainer(k) {
			t.Errorf("%s must be a container kind", k)
		}
	}
	for _, k := range []string{"class", "record-struct", "component", "actor", "store", "note", "abstract-class"} {
		if _, ok := c.Lookup(k); !ok || c.IsContainer(k) {
			t.Errorf("%s must be a known block kind", k)
		}
	}
	if k, _ := c.Lookup("external"); k.Style != "external-system" {
		t.Errorf("external's base style: %q", k.Style)
	}
	for _, id := range []string{"extends", "implements", "contains", "depends", "holds.many.ro.internal", "uses", "injects", "data-flow", "call", "event"} {
		if _, ok := c.LookupRelation(id); !ok {
			t.Errorf("relation type %s is missing", id)
		}
	}
	if _, ok := c.Lookup("nonsense"); ok {
		t.Error("a kind outside the dictionary is not found")
	}
	var nilCatalog *KindCatalog
	if nilCatalog.IsContainer("group") {
		t.Error("a nil catalog knows nothing")
	}
}

func TestKindsMerge(t *testing.T) {
	base := `{"contractVersion":5,"groups":[
	 {"id":"a","name":{"ru":"А"},"kinds":[{"id":"x","name":{"ru":"икс"}},{"id":"y","name":{"ru":"игрек"}}]},
	 {"id":"b","name":{"ru":"Б"},"kinds":[{"id":"z","name":{"ru":"зет"}}]}],
	 "relationGroups":[{"id":"r","name":{"ru":"Р"},"types":[{"id":"t1","name":{"ru":"т1"}},{"id":"t2","name":{"ru":"т2"}}]}]}`
	extra := `{"groups":[
	 {"id":"a","name":{"ru":"А2"},"kinds":[{"id":"x","name":{"ru":"икс2"},"container":true}]},
	 {"id":"c","name":{"ru":"В"},"kinds":[{"id":"y","name":{"ru":"игрек2"}},{"id":"w","name":{"ru":"дабл"}}]}],
	 "relationGroups":[{"id":"r2","name":{"ru":"Р2"},"types":[{"id":"t1","name":{"ru":"т1-новый"},"style":"s"}]}]}`
	c, err := ParseKinds([]byte(base), []byte(extra))
	if err != nil {
		t.Fatal(err)
	}
	// group a is replaced entirely, in its place; b stays; c is appended;
	// the kind y left group a for group c
	var ids []string
	for _, g := range c.Groups {
		ids = append(ids, g.ID)
	}
	if strings.Join(ids, ",") != "a,b,c" {
		t.Fatalf("group order: %v", ids)
	}
	if len(c.Groups[0].Kinds) != 1 || c.Groups[0].Kinds[0].Name["ru"] != "икс2" || !c.IsContainer("x") {
		t.Fatalf("group a must be the workspace's: %+v", c.Groups[0])
	}
	if k, _ := c.Lookup("y"); k.Name["ru"] != "игрек2" {
		t.Fatalf("y must be the workspace's: %+v", k)
	}
	// relation types merge the same way: t1 moved to r2, t2 stays in r
	if t1, _ := c.LookupRelation("t1"); t1.Style != "s" {
		t.Fatalf("t1 must be the workspace's: %+v", t1)
	}
	if len(c.RelationGroups[0].Types) != 1 || c.RelationGroups[0].Types[0].ID != "t2" {
		t.Fatalf("t1 must leave its default group: %+v", c.RelationGroups[0])
	}
}

func TestKindsErrors(t *testing.T) {
	ok := `{"contractVersion":5,"groups":[{"id":"a","name":{"ru":"А"},"kinds":[{"id":"x","name":{"ru":"икс"}}]}]}`
	for name, tc := range map[string]struct{ base, extra, want string }{
		"version":      {ok, `{"contractVersion":4}`, "нужен 5"},
		"empty kind":   {ok, `{"groups":[{"id":"b","name":{"ru":"Б"},"kinds":[{"id":"","name":{"ru":"я"}}]}]}`, "id is empty"},
		"no name":      {ok, `{"groups":[{"id":"b","name":{"ru":"Б"},"kinds":[{"id":"q"}]}]}`, "name is empty"},
		"dup in group": {ok, `{"groups":[{"id":"b","name":{"ru":"Б"},"kinds":[{"id":"q","name":{"ru":"я"}}]},{"id":"c","name":{"ru":"В"},"kinds":[{"id":"q","name":{"ru":"я"}}]}]}`, "unique across the catalog"},
		"not json":     {ok, `{`, "kinds.json"},
		"visibility":   {ok, `{"relationGroups":[{"id":"r","name":{"ru":"Р"},"types":[{"id":"t","name":{"ru":"т"},"visibility":"shown"}]}]}`, "visible, hidden or nothing"},
	} {
		if _, err := ParseKinds([]byte(tc.base), []byte(tc.extra)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error with %q, got %v", name, tc.want, err)
		}
	}
	// no workspace file is the default alone
	if _, err := LoadKinds(t.TempDir(), []byte(ok)); err != nil {
		t.Fatal(err)
	}
}

func TestLocalized(t *testing.T) {
	m := map[string]string{"en": "Class", "ru": "Класс"}
	if Localized(m, "ru") != "Класс" || Localized(m, "de") != "Class" || Localized(nil, "ru") != "" {
		t.Fatal("Localized must pick the language, else the first by code")
	}
}

var backticked = regexp.MustCompile("`([^`]+)`")

// The dictionary holds every nativeKind the normative tables of
// docs/extractors/*.md list (ADR_20260930-2, the owner's rule: only what an
// extractor emits, for every language), and every underlying kind a Go type can
// have. The tables are read from the files, so a table row without a dictionary
// entry breaks the build.
func TestDictionaryCoversExtractorTables(t *testing.T) {
	c := testCatalog(t)
	files, err := filepath.Glob(filepath.Join("..", "docs", "extractors", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no extractor tables: %v", err)
	}
	seen := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		col := -1
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			if !strings.HasPrefix(line, "|") {
				col = -1
				continue
			}
			cells := strings.Split(strings.Trim(line, "| "), "|")
			for i, cell := range cells {
				if strings.TrimSpace(cell) == "nativeKind" {
					col = i
				}
			}
			if col < 0 || col >= len(cells) || strings.TrimSpace(cells[col]) == "nativeKind" || strings.HasPrefix(strings.TrimSpace(cells[0]), "-") {
				continue
			}
			for _, m := range backticked.FindAllStringSubmatch(cells[col], -1) {
				seen++
				if _, ok := c.Lookup(m[1]); !ok {
					t.Errorf("%s: nativeKind %q is in the table but not in host/defaults/kinds.json", filepath.Base(f), m[1])
				}
			}
		}
	}
	if seen < 30 {
		t.Fatalf("read only %d nativeKind cells: the table parser lost the tables", seen)
	}
	for i := types.Bool; i <= types.Complex128; i++ {
		name := types.Typ[i].Name()
		if _, ok := c.Lookup(name); !ok {
			t.Errorf("Go basic type %q (the nativeKind of type T %s) is not in the dictionary", name, name)
		}
	}
	for _, name := range []string{"byte", "rune", "uintptr"} {
		if _, ok := c.Lookup(name); !ok {
			t.Errorf("Go type %q is not in the dictionary", name)
		}
	}
}

// Every relation type sync derives has a dictionary entry, with the default
// visibility that a project used to record for it: hidden for the members that
// are not public, for uses and injects, visible for the rest (CONTRACT §5, §6).
func TestDictionaryCoversDerivedRelationTypes(t *testing.T) {
	c := testCatalog(t)
	want := map[string]string{"extends": "visible", "implements": "visible", "contains": "visible", "depends": "visible", "uses": "hidden", "injects": "hidden"}
	for _, base := range []string{"holds.one", "holds.optional", "holds.many", "holds.many.ro", "holds.keyed", "holds.keyed.ro"} {
		want[base], want[base+".internal"] = "visible", "hidden"
	}
	for id, visibility := range want {
		typ, ok := c.LookupRelation(id)
		if !ok {
			t.Errorf("relation type %s (derived by sync) is not in the dictionary", id)
			continue
		}
		if typ.Visibility != visibility || c.RelationVisibility(id) != visibility {
			t.Errorf("relation type %s: visibility %q, want %q", id, typ.Visibility, visibility)
		}
	}
	if c.RelationVisibility("not-in-the-dictionary") != "" || (*KindCatalog)(nil).RelationVisibility("uses") != "" {
		t.Error("a type outside the dictionary says nothing about its visibility: the view decides")
	}
	for _, via := range []*Via{nil, {Cardinality: "optional"}, {Cardinality: "many", Mutability: "readonly"}, {Cardinality: "keyed"}, {Cardinality: "keyed", Mutability: "readonly", Modifiers: []string{"private"}}} {
		if id := deriveRelationType("holds", via); id != "" {
			if _, ok := c.LookupRelation(id); !ok {
				t.Errorf("deriveRelationType gave %q, not in the dictionary", id)
			}
		}
	}
}
