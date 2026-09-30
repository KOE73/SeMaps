package core

import (
	"fmt"
	"strings"
	"testing"
)

// contentsModel: a namespace e_ns that contains five types (names Alpha..Echo,
// ids not in name order), a nested namespace e_sub, e_a (already on v_main), a
// type whose entity is missing, a type whose contains relation is missing. e_ns
// is not on the view yet.
func contentsModel(t *testing.T) *Model {
	t.Helper()
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	m.SetCanvas(testCanvas(t))
	var ops []Op
	ent := func(id, kind, name, status string) {
		st := ""
		if status != "" {
			st = `,"status":"` + status + `"`
		}
		ops = append(ops,
			modelOp("entity", id, "", "", fmt.Sprintf(`{"id":%q,"kind":%q,"origin":"authored"%s}`, id, kind, st)),
			modelOp("text", id, "", "ru", fmt.Sprintf(`{"name":{"v":%q,"at":"2026-09-30T00:00:00Z","origin":"authored"}}`, name)))
	}
	contains := func(to, status string) {
		st := ""
		if status != "" {
			st = `,"status":"` + status + `"`
		}
		id := "r_ns_" + to + "_contains"
		ops = append(ops, modelOp("relation", id, "", "", fmt.Sprintf(`{"id":%q,"from":"e_ns","to":%q,"type":"contains","origin":"authored"%s}`, id, to, st)))
	}
	ent("e_ns", "namespace", "Ns", "")
	for id, name := range map[string]string{"e_t_d": "Delta", "e_t_a": "Alpha", "e_t_e": "Echo", "e_t_b": "Bravo", "e_t_c": "Charlie"} {
		ent(id, "class", name, "")
		contains(id, "")
	}
	ent("e_sub", "namespace", "Sub", "")
	contains("e_sub", "")
	contains("e_a", "") // already on the view
	ent("e_gone", "class", "Gone", "missing")
	contains("e_gone", "present")
	ent("e_old", "class", "Old", "")
	contains("e_old", "missing")
	if _, err := m.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	return m
}

func skipped(rep *ContentsReport, entity string) string {
	for _, s := range rep.Skipped {
		if s.Entity == entity {
			return s.Reason
		}
	}
	return ""
}

func overlap(a, b Rect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

func TestAddContainerContentsNeedsAHuman(t *testing.T) {
	m := contentsModel(t)
	batches := len(m.journal)
	_, _, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Contents: true}, false, "agent")
	refused(t, err, "direct request")
	if len(m.journal) != batches || rectOfMaybe(m, "e_ns") {
		t.Fatal("something was written without a human")
	}
}

func rectOfMaybe(m *Model, id string) bool {
	l, _ := m.loadLayout("v_main")
	return l.items[id] != nil
}

func TestAddContainerWithContentsPlacesAGrid(t *testing.T) {
	m := contentsModel(t)
	cv := testCanvas(t)
	cc := cv.Container
	batches := len(m.journal)
	id, rep, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Rect: Rect{X: 600, Y: 0}, Contents: true}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.journal) != batches+1 {
		t.Fatalf("%d batches, want one", len(m.journal)-batches)
	}
	if rep.Contents == nil || len(rep.Contents.Placed) != 6 || len(rep.Touched) != 7 {
		t.Fatalf("report = %+v touched %v", rep.Contents, rep.Touched)
	}
	// by name: Alpha Bravo Charlie / Delta Echo Sub — three columns for six
	order := []string{"e_t_a", "e_t_b", "e_t_c", "e_t_d", "e_t_e", "e_sub"}
	if strings.Join(rep.Contents.Placed, ",") != strings.Join(order, ",") {
		t.Fatalf("placed in %v", rep.Contents.Placed)
	}
	box := rectOf(t, m, id)
	if box.X != 600 || box.Y != 0 {
		t.Fatalf("the container moved from where it was asked: %+v", box)
	}
	// content starts inside padding and caption strip, on the grid step
	wantX := []float64{620, 840, 1060, 620, 840, 1060}
	wantY := []float64{50, 50, 50, 190, 190, 190}
	doc, _ := m.view("v_main")
	parents := map[string]string{}
	for _, p := range viewItems(doc, "placements") {
		parents[p.str("entity")] = p.str("parent")
	}
	var rects []Rect
	for i, e := range order {
		r := rectOf(t, m, e)
		if r.X != wantX[i] || r.Y != wantY[i] {
			t.Errorf("%s at %v,%v, want %v,%v", e, r.X, r.Y, wantX[i], wantY[i])
		}
		if int(r.X)%int(cv.Grid) != 0 || int(r.Y)%int(cv.Grid) != 0 {
			t.Errorf("%s is off the grid: %+v", e, r)
		}
		if parents[e] != "e_ns" {
			t.Errorf("%s has parent %q", e, parents[e])
		}
		if r.X < box.X+cc.Padding || r.Y < box.Y+cc.HeaderHeight+cc.Padding || r.Right()+cc.Padding > box.Right() || r.Bottom()+cc.Padding > box.Bottom() {
			t.Errorf("%s = %+v is not inside %+v with the caption strip and padding", e, r, box)
		}
		for _, o := range rects {
			if overlap(o, r) {
				t.Errorf("%s overlaps another placement: %+v %+v", e, r, o)
			}
		}
		rects = append(rects, r)
	}
	// the container is sized to what it holds: padding right and below exactly
	if box.Right() != 1060+180+cc.Padding || box.Bottom() != 190+cc.MinHeight+cc.Padding {
		t.Fatalf("container = %+v", box)
	}
	// a nested container kind is an empty frame of the default container size
	sub := rectOf(t, m, "e_sub")
	if sub.Width != cc.MinWidth || sub.Height != cc.MinHeight {
		t.Fatalf("frame = %+v", sub)
	}
	v, _ := m.GetView("v_main#e_ns")
	if n := len(v.Placements[0].Children); n != 6 {
		t.Fatalf("children = %d", n)
	}
	for _, c := range v.Placements[0].Children {
		if c.Entity == "e_sub" && (!c.Container || len(c.Children) != 0) {
			t.Fatalf("e_sub is not an empty frame: %+v", c)
		}
	}
	// what was left out is named with its reason
	if r := skipped(rep.Contents, "e_a"); !strings.Contains(r, "already on the view") {
		t.Errorf("e_a: %q", r)
	}
	if r := skipped(rep.Contents, "e_gone"); !strings.Contains(r, "entity is missing") {
		t.Errorf("e_gone: %q", r)
	}
	if r := skipped(rep.Contents, "e_old"); !strings.Contains(r, "relation is missing") {
		t.Errorf("e_old: %q", r)
	}
	// e_a was not moved and not re-parented
	if a := rectOf(t, m, "e_a"); a.X != 10 || a.Y != 20 || parents["e_a"] != "" {
		t.Fatalf("e_a moved: %+v parent %q", a, parents["e_a"])
	}
	// a given size is a minimum
	m2 := contentsModel(t)
	if _, _, err := m2.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Rect: Rect{X: 600, Y: 0, Width: 2000, Height: 900}, Contents: true}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if b := rectOf(t, m2, "e_ns"); b.Width != 2000 || b.Height != 900 {
		t.Fatalf("minimum ignored: %+v", b)
	}
}

func TestAddContainerContentsIsAllOrNothing(t *testing.T) {
	m := contentsModel(t)
	batches := len(m.journal)
	// a refusal before anything is written leaves nothing, the container included
	_, _, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Parent: "e_nope", Contents: true}, true, "agent")
	refused(t, err, "e_nope")
	if len(m.journal) != batches || rectOfMaybe(m, "e_ns") || rectOfMaybe(m, "e_t_a") {
		t.Fatal("a refused call left placements behind")
	}
	// without contents a container that is already on the view is still refused
	if _, _, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Contents: true}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	_, _, err = m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Rect: Rect{Width: 300, Height: 200}}, true, "agent")
	refused(t, err, "already on")
}

func TestAddContainerContentsOnAContainerAlreadyPlaced(t *testing.T) {
	m := contentsModel(t)
	cv := testCanvas(t)
	if _, _, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Rect: Rect{X: 600, Y: 0}, Contents: true}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	// nothing to add: nothing is touched
	batches := len(m.journal)
	before := rectOf(t, m, "e_ns")
	_, rep, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Contents: true}, true, "agent")
	if err != nil || len(rep.Touched) != 0 || len(rep.Contents.Placed) != 0 || len(m.journal) != batches {
		t.Fatalf("no news: %+v %v", rep, err)
	}
	if r := skipped(rep.Contents, "e_t_a"); !strings.Contains(r, "already on the view") {
		t.Fatalf("e_t_a: %q", r)
	}
	// a new member arrives; only it is placed, under the lowest child
	ops := []Op{
		modelOp("entity", "e_t_z", "", "", `{"id":"e_t_z","kind":"class","origin":"authored"}`),
		modelOp("text", "e_t_z", "", "ru", `{"name":{"v":"Zulu","at":"2026-09-30T00:00:00Z","origin":"authored"}}`),
		modelOp("relation", "r_ns_z_contains", "", "", `{"id":"r_ns_z_contains","from":"e_ns","to":"e_t_z","type":"contains","origin":"authored"}`),
	}
	if _, err := m.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	moved := rectOf(t, m, "e_t_a")
	batches = len(m.journal)
	_, rep, err = m.AddContainer("v_main", ContainerSpec{Entity: "e_ns", Contents: true}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.journal) != batches+1 || len(rep.Contents.Placed) != 1 || rep.Contents.Placed[0] != "e_t_z" || !strings.Contains(rep.Contents.Note, "under its lowest child") {
		t.Fatalf("report = %+v", rep.Contents)
	}
	z := rectOf(t, m, "e_t_z")
	lowest := rectOf(t, m, "e_sub").Bottom()
	if z.X != 620 || z.Y != gridUp(lowest+cv.Gap.Node, cv.Grid) {
		t.Fatalf("e_t_z = %+v, lowest child ends at %v", z, lowest)
	}
	after := rectOf(t, m, "e_ns")
	if after.X != before.X || after.Y != before.Y || after.Width != before.Width || after.Bottom() != z.Bottom()+cv.Container.Padding {
		t.Fatalf("container %+v -> %+v", before, after)
	}
	if rectOf(t, m, "e_t_a") != moved {
		t.Fatal("an existing member was moved")
	}
}
