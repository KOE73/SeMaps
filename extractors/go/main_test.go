package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/sample/expected.json")

var sampleArgs = []string{"--root", "testdata/sample", "--edges", "holds,uses"}

func runSample(t *testing.T, args ...string) []byte {
	t.Helper()
	var out, errb bytes.Buffer
	if code := run(args, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	return out.Bytes()
}

// TestExpected: the sample module, byte for byte.
func TestExpected(t *testing.T) {
	got := runSample(t, sampleArgs...)
	path := filepath.Join("testdata", "sample", "expected.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("output differs from %s; rerun with -update and review the diff", path)
	}
}

// TestDeterministic: two runs, same bytes.
func TestDeterministic(t *testing.T) {
	a, b := runSample(t, sampleArgs...), runSample(t, sampleArgs...)
	if !bytes.Equal(a, b) {
		t.Fatal("two runs differ")
	}
}

const sample = "example.com/sample/"

// mappingRows ties each row of the edge table in docs/extractors/go.md (by its
// first cell) to one edge of the sample that exemplifies it.
var mappingRows = map[string]struct{ from, to, kind, native string }{
	"пакет объявляет символ верхнего уровня":                            {"model", "model.User", "contains", ""},
	"`import` пакета из вывода":                                         {"store", "model", "depends", ""},
	"поле структуры `f T`":                                              {"model.User", "model.Item", "holds", "field"},
	"встроенное поле `struct{ T }`":                                     {"model.User", "model.Base", "holds", "embed"},
	"встраивание интерфейса `interface{ I }`":                           {"store.ReadWriter", "store.Reader", "extends", "embed"},
	"определение интерфейса над интерфейсом `type J I`":                 {"store.Source", "store.Reader", "extends", "underlying"},
	"набор методов `T` ⊇ интерфейс":                                     {"store.MemStore", "store.Reader", "implements", "methodset"},
	"набор методов только `*T` ⊇ интерфейс":                             {"store.MemStore", "store.Writer", "implements", "methodset.ptr"},
	"определение типа над составным или именованным типом `type T []E`": {"model.Batch", "model.Item", "holds", "underlying"},
	"определение функционального типа `type F func(A) R`":               {"model.HandlerFunc", "model.Item", "uses", "signature"},
	"псевдоним `type A = B`":                                            {"model.ItemAlias", "model.Item", "uses", "alias"},
	"параметр функции или метода":                                       {"store.NewMemStore", "model.Item", "uses", ""},
	"результат функции или метода":                                      {"store.NewMemStore", "store.MemStore", "uses", ""},
	"тип `var` / `const`":                                               {"model.Default", "model.Item", "uses", ""},
}

// docRows reads the edge mapping table of docs/extractors/go.md:
// construct → (kind, native), "—" meaning no native.
func docRows(t *testing.T) map[string][2]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "extractors", "go.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	i := strings.Index(text, "<!-- mapping-table")
	if i < 0 {
		t.Fatal("docs/extractors/go.md: no mapping-table marker")
	}
	rows := map[string][2]string{}
	started := false
	for _, line := range strings.Split(text[i:], "\n")[1:] {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if started {
				break
			}
			continue
		}
		started = true
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 3 {
			continue
		}
		c := func(k int) string { return strings.Trim(strings.TrimSpace(cells[k]), "`") }
		if c(0) == "Конструкция" || strings.HasPrefix(c(0), "---") {
			continue
		}
		native := c(2)
		if native == "—" {
			native = ""
		}
		rows[strings.TrimSpace(cells[0])] = [2]string{c(1), native}
	}
	return rows
}

// TestMappingTable: every row of the documented table has an example in the
// sample, with the documented kind and native; every example has a row.
func TestMappingTable(t *testing.T) {
	var facts Facts
	if err := json.Unmarshal(runSample(t, sampleArgs...), &facts); err != nil {
		t.Fatal(err)
	}
	rows := docRows(t)
	if len(rows) == 0 {
		t.Fatal("no rows parsed from the mapping table")
	}
	for construct, doc := range rows {
		ex, ok := mappingRows[construct]
		if !ok {
			t.Errorf("table row %q has no example in the test", construct)
			continue
		}
		if ex.kind != doc[0] || ex.native != doc[1] {
			t.Errorf("row %q: table says %s/%q, test expects %s/%q", construct, doc[0], doc[1], ex.kind, ex.native)
		}
	}
	for construct, ex := range mappingRows {
		if _, ok := rows[construct]; !ok {
			t.Errorf("test example %q has no row in docs/extractors/go.md", construct)
		}
		found := false
		for _, e := range facts.Edges {
			if e.From == sample+ex.from && e.To == sample+ex.to && e.Kind == ex.kind && e.Native == ex.native {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("row %q: no edge %s -%s/%s-> %s in the sample output", construct, ex.from, ex.kind, ex.native, ex.to)
		}
	}
}

// TestNoEdgesFlag: without --edges only the base kinds are printed and declared.
func TestNoEdgesFlag(t *testing.T) {
	var facts Facts
	if err := json.Unmarshal(runSample(t, "--root", "testdata/sample"), &facts); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(facts.EdgeKinds, ","); got != "contains,depends,extends,implements" {
		t.Fatalf("edgeKinds = %s", got)
	}
	for _, e := range facts.Edges {
		if e.Kind == "holds" || e.Kind == "uses" {
			t.Fatalf("unrequested edge %+v", e)
		}
	}
}

// TestSkipped: _test.go, generated files, testdata/, init and self edges.
func TestSkipped(t *testing.T) {
	out := string(runSample(t, sampleArgs...))
	for _, s := range []string{"TestOnly", "Generated", "Ignored", "store.init", `"from": "example.com/sample/store.MemStore",
      "to": "example.com/sample/store.MemStore"`} {
		if strings.Contains(out, s) {
			t.Errorf("output contains %q", s)
		}
	}
}

func TestExclude(t *testing.T) {
	out := string(runSample(t, "--root", "testdata/sample", "--exclude", "store/**"))
	if strings.Contains(out, "store.MemStore") || !strings.Contains(out, "model.User") {
		t.Fatal("--exclude store/** not applied")
	}
}

func TestArgs(t *testing.T) {
	for _, args := range [][]string{{"--edges", "extends"}, {"--bogus"}, {"--root"}, {"--implements", "io.*"}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 || out.Len() != 0 {
			t.Errorf("%v: exit %d, stdout %q", args, code, out.String())
		}
	}
}

func TestPathFilter(t *testing.T) {
	f := newPathFilter([]string{"core"}, []string{"**/*_gen.go"})
	for rel, want := range map[string]bool{
		"core/a.go": true, "core/sub/b.go": true, "host/c.go": false, "coreX/d.go": false,
		"core/vendor/x/e.go": false, "core/testdata/f.go": false, "core/g_test.go": false, "core/h_gen.go": false,
	} {
		if got := f.included(rel) && !f.excludedByPath(rel); got != want {
			t.Errorf("%s: got %v, want %v", rel, got, want)
		}
	}
}
