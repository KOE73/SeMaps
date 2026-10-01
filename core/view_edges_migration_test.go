package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"semaps/core/migrate"
)

// The picture survives the migration of a view's full `edges` list: the lines
// that core draws after (both ends placed, the rule of CONTRACT §8.5) are exactly
// the ones the list held (ADR_20260930-7).
func TestMigratedEdgeListsDrawTheSameLines(t *testing.T) {
	ws := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(ws, "projects", "p", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("project.json", `{"id":"p","title":"P","contractVersion":5,"languages":["ru"]}`)
	write("entities.json", `{"contractVersion":5,"entities":[
		{"id":"e_a","name":"A","kind":"class","origin":"code"},{"id":"e_b","name":"B","kind":"class","origin":"code"},
		{"id":"e_c","name":"C","kind":"class","origin":"code"},{"id":"e_d","name":"D","kind":"class","origin":"code"},
		{"id":"e_z","name":"Z","kind":"class","origin":"code"}]}`)
	write("relations.json", `{"contractVersion":5,"relations":[
		{"id":"r1","from":"e_a","to":"e_b","type":"depends","origin":"code"},
		{"id":"r2","from":"e_a","to":"e_c","type":"depends","origin":"code"},
		{"id":"r3","from":"e_b","to":"e_c","type":"uses","origin":"code"},
		{"id":"r4","from":"e_c","to":"e_d","type":"depends","origin":"code"},
		{"id":"r5","from":"e_a","to":"e_z","type":"depends","origin":"code"}]}`)
	write("text.ru.json", `{"contractVersion":5,"language":"ru","entries":{}}`)

	entry := func(id, from, to, typ string) string {
		return `{"id":"` + id + `","from":"` + from + `","to":"` + to + `","type":"` + typ + `","routing":"bezier"}`
	}
	all := []string{entry("r1", "e_a", "e_b", "depends"), entry("r2", "e_a", "e_c", "depends"), entry("r3", "e_b", "e_c", "uses"),
		entry("r4", "e_c", "e_d", "depends"), entry("r5", "e_a", "e_z", "depends")}
	views := map[string][]string{ // the list of each view, in the old meaning: what was drawn
		"v_all":     all,
		"v_curated": {all[0], all[2]},
		"v_own":     {all[0], entry("r_new", "e_a", "e_d", "depends")},
		"v_none":    {},
		"v_hidden":  {all[3]},
	}
	for id, list := range views {
		head := ""
		if id == "v_hidden" {
			head = `"relations":{"default":"hidden","except":["r1"]},`
		}
		write("views/"+id+".view.json", `{"id":"`+id+`","project":"p","axis":"a",`+head+`"placements":[
			{"entity":"e_a","parent":null,"x":0,"y":0},{"entity":"e_b","parent":null,"x":0,"y":0},
			{"entity":"e_c","parent":null,"x":0,"y":0},{"entity":"e_d","parent":null,"x":0,"y":0}],
			"edges":[`+strings.Join(list, ",")+`]}`)
		write("text.ru.json", `{"contractVersion":5,"language":"ru","entries":{}}`)
	}
	kinds := defaultKindsJSON(t)
	if _, err := migrate.Workspace(ws, migrate.Options{DefaultKinds: kinds}); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModel(ws, "p", kinds)
	if err != nil {
		t.Fatal(err)
	}
	m.SetCanvas(testCanvas(t))
	placed := map[string]bool{"e_a": true, "e_b": true, "e_c": true, "e_d": true}
	for id, list := range views {
		var want []string
		for _, raw := range list {
			var e struct{ ID, From, To string }
			_ = json.Unmarshal([]byte(strings.NewReplacer(`"id"`, `"ID"`, `"from"`, `"From"`, `"to"`, `"To"`).Replace(raw)), &e)
			if placed[e.From] && placed[e.To] {
				want = append(want, e.ID)
			}
		}
		info, err := m.GetView(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var got []string
		for _, e := range info.Edges {
			got = append(got, e.ID)
			if e.Routing != "" && !slices.Contains(want, e.ID) {
				t.Errorf("%s: a line of an entry drawn that was not listed: %s", id, e.ID)
			}
		}
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: drawn %v, was %v", id, got, want)
		}
	}
}
