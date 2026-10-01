package core

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	stdsync "sync"
	"testing"
)

// visibilityWorkspace: entities e_a, e_b, e_c (e_c is not placed) and one
// relation of each type — vis (dictionary: visible), hid (dictionary: hidden),
// none (not in the dictionary) — between e_a and e_b, plus r_a_c between e_a and
// e_c. The view's `relations` is given as it stands in the file.
func visibilityWorkspace(t *testing.T, relations string) string {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	rel := func(id, from, to, typ string) string {
		return fmt.Sprintf(`{"id":%q,"from":%q,"to":%q,"type":%q,"origin":"authored"}`, id, from, to, typ)
	}
	files := map[string]string{
		"project.json": `{"id":"p","contractVersion":5}`,
		"../../kinds.json": `{"groups":[],"relationGroups":[{"id":"r","name":{"ru":"р"},"types":[
    {"id":"vis","visibility":"visible","name":{"ru":"в"}},
    {"id":"hid","visibility":"hidden","name":{"ru":"с"}}]}]}`,
		"entities.json": `{"entities":[{"id":"e_a","kind":"app","origin":"authored"},{"id":"e_b","kind":"app","origin":"authored"},{"id":"e_c","kind":"app","origin":"authored"}]}`,
		"text.ru.json": `{"contractVersion":5,"language":"ru","entries":{
  "e_a":{"name":{"v":"A","at":"2026-09-24T00:00:00Z","origin":"authored"}},
  "e_b":{"name":{"v":"B","at":"2026-09-24T00:00:00Z","origin":"authored"}},
  "e_c":{"name":{"v":"C","at":"2026-09-24T00:00:00Z","origin":"authored"}}}}`,
		"relations.json": `{"contractVersion":5,"relations":[` + strings.Join([]string{
			rel("r_vis", "e_a", "e_b", "vis"), rel("r_hid", "e_a", "e_b", "hid"), rel("r_none", "e_a", "e_b", "none"), rel("r_a_c", "e_a", "e_c", "vis"),
		}, ",") + `]}`,
		"views/main.view.json": `{"id":"v_main","project":"p","axis":"axis_layer",` + relations + `"placements":[{"entity":"e_a","parent":null,"x":0,"y":0},{"entity":"e_b","parent":null,"x":200,"y":0}]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func loadVisibility(t *testing.T, ws string) (*Model, error) {
	t.Helper()
	m, err := LoadModel(ws, "p", nil)
	if err == nil {
		m.SetCanvas(testCanvas(t))
	}
	return m, err
}

func drawn(t *testing.T, m *Model, id string) bool {
	t.Helper()
	got, err := m.GetView("v_main")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got.Edges {
		if e.ID == id {
			return true
		}
	}
	return false
}

// visibleByRule is the rule of CONTRACT §8.5 written out independently of the
// code: the dictionary's visibility of the type, else the view's default
// (visible when absent), flipped by except.
func visibleByRule(dict, viewDefault string, inExcept bool) bool {
	d := "visible"
	switch {
	case dict != "":
		d = dict
	case viewDefault != "":
		d = viewDefault
	}
	return (d == "visible") != inExcept
}

// After SetRelationsVisible the relation is visible on the view exactly when
// `visible` says so — whatever the type's default in the dictionary, the view's
// default (visible, hidden or absent), whether the id already stands in
// `except`, and whichever state was asked. The call sets a state; it never
// flips one.
func TestSetRelationsVisibleSetsTheStateInEveryCombination(t *testing.T) {
	dictionary := map[string]string{"r_vis": "visible", "r_hid": "hidden", "r_none": ""}
	for _, rel := range []string{"r_vis", "r_hid", "r_none"} {
		for _, def := range []string{"", "visible", "hidden"} {
			for _, inExcept := range []bool{false, true} {
				for _, want := range []bool{true, false} {
					name := fmt.Sprintf("%s/default=%q/except=%v/visible=%v", rel, def, inExcept, want)
					t.Run(name, func(t *testing.T) {
						var parts []string
						if def != "" {
							parts = append(parts, `"default":"`+def+`"`)
						}
						if inExcept {
							parts = append(parts, fmt.Sprintf(`"except":[%q]`, rel))
						}
						relations := ""
						if len(parts) > 0 {
							relations = `"relations":{` + strings.Join(parts, ",") + `},`
						}
						m, err := loadVisibility(t, visibilityWorkspace(t, relations))
						if err != nil {
							t.Fatal(err)
						}
						before := drawn(t, m, rel)
						if oracle := visibleByRule(dictionary[rel], def, inExcept); before != oracle {
							t.Fatalf("the rule draws %v, the spec says %v", before, oracle)
						}
						checkSetsTheState(t, m, rel, want, before)
					})
				}
			}
		}
	}
}

func checkSetsTheState(t *testing.T, m *Model, rel string, want, before bool) {
	t.Helper()
	res, err := m.SetRelationsVisible("v_main", []string{rel}, want, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if got := drawn(t, m, rel); got != want {
		t.Fatalf("asked visible=%v, drawn=%v (was %v)", want, got, before)
	}
	if (before == want) != (len(res.Already) == 1) || (before != want) != (len(res.Changed) == 1) {
		t.Fatalf("was %v, asked %v: reported %+v", before, want, res)
	}
	// asking again changes nothing, and it says so
	again, err := m.SetRelationsVisible("v_main", []string{rel}, want, "agent")
	if err != nil || len(again.Changed) != 0 || len(again.Already) != 1 || drawn(t, m, rel) != want {
		t.Fatalf("second call: %+v %v", again, err)
	}
	// the answer survives Save and a reload
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	m2, err := loadVisibility(t, m.workspace)
	if err != nil {
		t.Fatal(err)
	}
	if drawn(t, m2, rel) != want {
		t.Fatalf("after reload visible != %v", want)
	}
}

func TestSetRelationsVisibleIsOneAtomicBatch(t *testing.T) {
	ws := visibilityWorkspace(t, `"relations":{"default":"visible"},`)
	m, err := loadVisibility(t, ws)
	if err != nil {
		t.Fatal(err)
	}
	// an unknown id refuses the whole call: the known ones stay as they were
	_, err = m.SetRelationsVisible("v_main", []string{"r_vis", "r_nope", "r_none"}, false, "agent")
	refused(t, err, "r_nope")
	if !drawn(t, m, "r_vis") || !drawn(t, m, "r_none") || len(m.Dirty().Views) != 0 {
		t.Fatal("a refused call changed something")
	}
	_, err = m.SetRelationsVisible("v_main", nil, false, "agent")
	refused(t, err, "at least one")

	// r_hid is hidden by its type already; r_a_c has an end that is not placed;
	// the same id twice counts once
	res, err := m.SetRelationsVisible("v_main", []string{"r_vis", "r_hid", "r_none", "r_a_c", "r_vis"}, false, "agent")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(res.Changed)
	if !slices.Equal(res.Changed, []string{"r_a_c", "r_none", "r_vis"}) || !slices.Equal(res.Already, []string{"r_hid"}) || !slices.Equal(res.Unplaced, []string{"r_a_c"}) {
		t.Fatalf("%+v", res)
	}
	for _, id := range []string{"r_vis", "r_hid", "r_none"} {
		if drawn(t, m, id) {
			t.Fatalf("%s still drawn", id)
		}
	}
}

// The reported "did not hide the first time": the edit read `except`, changed it
// and wrote it back outside the model's lock, so single calls made at once (an
// agent sends several tool calls together) lost each other's entries.
func TestSetRelationsVisibleSurvivesCallsAtOnce(t *testing.T) {
	const n = 40
	rels := make([]string, n)
	for i := range rels {
		rels[i] = fmt.Sprintf(`{"id":"r_%02d","from":"e_a","to":"e_b","type":"vis","origin":"authored"}`, i)
	}
	ws := visibilityWorkspace(t, `"relations":{"default":"visible"},`)
	file := filepath.Join(ws, "projects", "p", "relations.json")
	if err := os.WriteFile(file, []byte(`{"contractVersion":5,"relations":[`+strings.Join(rels, ",")+`]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadVisibility(t, ws)
	if err != nil {
		t.Fatal(err)
	}
	var wg stdsync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := m.SetRelationsVisible("v_main", []string{id}, false, "agent"); err != nil {
				t.Error(err)
			}
		}(fmt.Sprintf("r_%02d", i))
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if id := fmt.Sprintf("r_%02d", i); drawn(t, m, id) {
			t.Errorf("%s was not hidden", id)
		}
	}
}
