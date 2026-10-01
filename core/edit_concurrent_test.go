package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	stdsync "sync"
	"testing"
)

// crowdModel: entities e_00..e_39 and relations r_00..r_39 (all between e_00 and
// e_01); the view v_main places the first `placed` entities. The host shares one
// *Model per project and MCP serves calls at once, so these tests run single
// edits in parallel and demand that every one lands (Model.editMu).
const crowd = 40

func crowdModel(t *testing.T, placed int) *Model {
	t.Helper()
	ws := visibilityWorkspace(t, `"relations":{"default":"visible"},`)
	dir := filepath.Join(ws, "projects", "p")
	var ents, texts, rels, places []string
	for i := 0; i < crowd; i++ {
		ents = append(ents, fmt.Sprintf(`{"id":"e_%02d","kind":"app","origin":"authored"}`, i))
		texts = append(texts, fmt.Sprintf(`"e_%02d":{"name":{"v":"N%02d","at":"2026-09-24T00:00:00Z","origin":"authored"}}`, i, i))
		rels = append(rels, fmt.Sprintf(`{"id":"r_%02d","from":"e_00","to":"e_01","type":"none","origin":"authored"}`, i))
		if i < placed {
			places = append(places, fmt.Sprintf(`{"entity":"e_%02d","parent":null,"x":%d,"y":0}`, i, i*200))
		}
	}
	files := map[string]string{
		"entities.json":        `{"entities":[` + strings.Join(ents, ",") + `]}`,
		"text.ru.json":         `{"contractVersion":5,"language":"ru","entries":{` + strings.Join(texts, ",") + `}}`,
		"relations.json":       `{"contractVersion":5,"relations":[` + strings.Join(rels, ",") + `]}`,
		"views/main.view.json": `{"id":"v_main","project":"p","axis":"axis_layer","placements":[` + strings.Join(places, ",") + `]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := loadVisibility(t, ws)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func atOnce(t *testing.T, f func(i int) error) {
	t.Helper()
	var wg stdsync.WaitGroup
	for i := 0; i < crowd; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := f(i); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}

// Placements are written one object per op, so calls on different placements
// cannot overwrite each other; the races are on one object: a check-then-act
// ("already on the view") and a read-modify-write of one placement.

// One text entry gets its name and its description from two calls at once, for
// 40 entries: a text is written as the whole entry, so without the lock the
// later write drops the field the earlier one set.
func TestSetTextTwoFieldsAtOnce(t *testing.T) {
	m := crowdModel(t, 0)
	var wg stdsync.WaitGroup
	for i := 0; i < crowd; i++ {
		key := fmt.Sprintf("e_%02d", i)
		for _, field := range []string{"name", "description"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := m.SetText("ru", key, field, field+" of "+key, "agent"); err != nil {
					t.Error(err)
				}
			}()
		}
	}
	wg.Wait()
	for i := 0; i < crowd; i++ {
		key := fmt.Sprintf("e_%02d", i)
		entry, err := m.text("ru", key)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"name", "description"} {
			f, err := child(entry, field)
			if err != nil || f.str("v") != field+" of "+key {
				t.Errorf("%s.%s = %q, want it set (the write was lost)", key, field, f.str("v"))
			}
		}
	}
}

// 40 calls at once each move the same placement by 5: every move must land.
func TestMoveElementsSamePlacementAtOnce(t *testing.T) {
	m := crowdModel(t, 1)
	dx, dy := 5.0, 0.0
	atOnce(t, func(i int) error {
		_, err := m.MoveElements("v_main", []string{"e_00"}, &dx, &dy, nil, nil, true, "agent")
		return err
	})
	view, err := m.view("v_main")
	if err != nil {
		t.Fatal(err)
	}
	p := viewItems(view, "placements")[0]
	if x, _ := p.num("x"); x != crowd*dx {
		t.Errorf("x = %v, want %v (moves were lost)", x, crowd*dx)
	}
}

func TestSetRoutingSurvivesCallsAtOnce(t *testing.T) {
	m := crowdModel(t, 2)
	orth := "orthogonal"
	atOnce(t, func(i int) error {
		_, err := m.SetRouting("v_main", &orth, []string{fmt.Sprintf("r_%02d", i)}, true, "agent")
		return err
	})
	view, err := m.view("v_main")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range viewItems(view, "edges") {
		got[e.str("id")] = e.str("routing")
	}
	for i := 0; i < crowd; i++ {
		if id := fmt.Sprintf("r_%02d", i); got[id] != orth {
			t.Errorf("%s: routing %q, want %q (the edit was lost)", id, got[id], orth)
		}
	}
}
