package core

import "testing"

func f(v float64) *float64 { return &v }

func geomModel(t *testing.T) *Model {
	t.Helper()
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	m.SetCanvas(testCanvas(t))
	ops := []Op{
		modelOp("entity", "e_in", "", "", `{"id":"e_in","kind":"subsystem","origin":"authored"}`),
		modelOp("text", "e_in", "", "ru", `{"name":{"v":"In","at":"2026-09-30T00:00:00Z","origin":"authored"}}`),
		modelOp("placement", "e_in", "v_main", "", `{"entity":"e_in","parent":"e_core","x":100,"y":100,"width":200,"height":150}`),
		modelOp("placement", "e_a", "v_main", "", `{"entity":"e_a","parent":"e_in","x":120,"y":140,"width":100,"height":40}`),
		modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":null,"x":600,"y":600}`),
	}
	if _, err := m.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	return m
}

func rectOf(t *testing.T, m *Model, id string) Rect {
	t.Helper()
	l, err := m.loadLayout("v_main")
	if err != nil {
		t.Fatal(err)
	}
	if l.items[id] == nil {
		t.Fatalf("no %s", id)
	}
	return l.rect(id)
}

func TestGeomStepsNeedAHuman(t *testing.T) {
	m := geomModel(t)
	_, err := m.MoveElements("v_main", []string{"e_b"}, f(10), f(0), nil, nil, false, "agent")
	refused(t, err, "direct request")
	_, err = m.FitContainer("v_main", []string{"e_in"}, false, "agent")
	refused(t, err, "direct request")
}

func TestMoveElementsContainerGoesWithContent(t *testing.T) {
	m := geomModel(t)
	rep, err := m.MoveElements("v_main", []string{"v_main#e_in"}, f(30), f(-20), nil, nil, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "e_in"); got.X != 130 || got.Y != 80 {
		t.Fatalf("container = %+v", got)
	}
	if got := rectOf(t, m, "e_a"); got.X != 150 || got.Y != 120 || got.Width != 100 {
		t.Fatalf("block inside = %+v", got)
	}
	if len(rep.Touched) != 2 {
		t.Fatalf("touched = %v", rep.Touched)
	}
	agent := 0
	for _, r := range m.Dirty().Views["v_main"] {
		if r.Author == "agent" {
			agent++
		}
	}
	if agent != 2 {
		t.Fatalf("agent-authored refs = %d", agent)
	}
	// to a point: the box of the elements lands there; a block without size gets none
	if _, err := m.MoveElements("v_main", []string{"e_b"}, nil, nil, f(700), f(650), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "e_b"); got.X != 700 || got.Y != 650 {
		t.Fatalf("to point = %+v", got)
	}
	doc, _ := m.view("v_main")
	for _, n := range viewItems(doc, "placements") {
		if n.str("entity") == "e_b" && has(n, "width") {
			t.Fatal("moving gave e_b a size")
		}
	}
	// moving a block out of its container's reach grows the container
	if _, err := m.MoveElements("v_main", []string{"e_a"}, f(400), f(0), nil, nil, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if z := rectOf(t, m, "e_in"); z.Right() < rectOf(t, m, "e_a").Right()+testCanvas(t).Container.Padding {
		t.Fatalf("container did not grow: %+v", z)
	}
}

func TestResizeElementsMinimumsAndContent(t *testing.T) {
	m := geomModel(t)
	if _, err := m.ResizeElements("v_main", []string{"e_a"}, f(20), f(10), true, "agent"); err != nil {
		t.Fatal(err)
	}
	cv := testCanvas(t)
	if got := rectOf(t, m, "e_a"); got.Width != cv.Node.MinWidth || got.Height != cv.Node.MinHeight {
		t.Fatalf("block = %+v", got)
	}
	// the container holds e_a (right edge 220): it cannot shrink under it
	if _, err := m.ResizeElements("v_main", []string{"e_in"}, f(50), f(50), true, "agent"); err != nil {
		t.Fatal(err)
	}
	z := rectOf(t, m, "e_in")
	if z.Width < 140 || z.Right() < 220+cv.Container.Padding || z.Bottom() < 180+cv.Container.Padding {
		t.Fatalf("container shrank under its content: %+v", z)
	}
	if z.Width < cv.Container.MinWidth || z.Height < cv.Container.MinHeight {
		t.Fatalf("container under minimum: %+v", z)
	}
}

func TestSetParentAndCycle(t *testing.T) {
	m := geomModel(t)
	if _, err := m.SetParent("v_main", []string{"e_b"}, "e_core", true, "agent"); err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "e_b"); got.X != 600 {
		t.Fatalf("coordinates moved: %+v", got)
	}
	v, _ := m.GetView("v_main")
	core := v.Placements[0]
	if core.Entity != "e_core" || len(core.Children) != 2 || core.Children[1].Entity != "e_b" {
		t.Fatalf("e_b not in e_core: %+v", core)
	}
	_, err := m.SetParent("v_main", []string{"e_core"}, "e_in", true, "agent")
	refused(t, err, "cannot go into itself")
	_, err = m.SetParent("v_main", []string{"e_b"}, "e_nope", true, "agent")
	refused(t, err, "e_nope is not on view")
	_, err = m.SetParent("v_main", []string{"e_b"}, "e_a", true, "agent")
	refused(t, err, "e_a is not a container")
	if _, err := m.SetParent("v_main", []string{"e_b"}, "", true, "agent"); err != nil {
		t.Fatal(err)
	}
	doc, _ := m.view("v_main")
	for _, p := range viewItems(doc, "placements") {
		if p.str("entity") == "e_b" && string(p.vals["parent"]) != "null" {
			t.Fatalf("out of any container is parent null: %s", objString(p))
		}
	}
}

func TestAddContainerNewOrExisting(t *testing.T) {
	m := geomModel(t)
	cv := testCanvas(t)
	id, rep, err := m.AddContainer("v_main", ContainerSpec{Name: "Storage", Parent: "e_core", Rect: Rect{X: 13, Y: 268, Width: 50, Height: 50}}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if id != "e_storage" {
		t.Fatalf("id %q", id)
	}
	// the host does not snap: 13 stays 13; only the minimum size is enforced
	if got := rectOf(t, m, id); got != (Rect{13, 268, cv.Container.MinWidth, cv.Container.MinHeight}) {
		t.Fatalf("new container = %+v", got)
	}
	e := m.record("entity", id)
	if e == nil || e.str("kind") != "group" || e.str("origin") != "authored" || e.str("status") != "present" || has(e, "name") || m.EntityName(id) != "Storage" {
		t.Fatalf("entity = %v, name %q", e, m.EntityName(id))
	}
	if len(rep.Touched) == 0 || rep.Touched[len(rep.Touched)-1] != "v_main#"+id {
		t.Fatalf("touched = %v", rep.Touched)
	}
	v, _ := m.GetView("v_main#" + id)
	if p := v.Placements[0]; p.Name != "Storage" || p.Parent == nil || *p.Parent != "e_core" || !p.Container {
		t.Fatalf("container = %+v", p)
	}
	doc, _ := m.view("v_main")
	items := viewItems(doc, "placements")
	if items[len(items)-1].str("entity") != id {
		t.Fatal("a new container is appended")
	}
	// an existing entity of a container kind
	if _, err := m.AddEntity("e_sub", "Sub", "subsystem", "", "agent"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.AddContainer("v_main", ContainerSpec{Entity: "e_sub", Rect: Rect{X: 900, Y: 0, Width: 200, Height: 200}}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	_, _, err = m.AddContainer("v_main", ContainerSpec{Entity: "e_sub"}, true, "agent")
	refused(t, err, "already on v_main")
	_, _, err = m.AddContainer("v_main", ContainerSpec{Entity: "e_x"}, true, "agent")
	refused(t, err, "not a container kind")
	_, _, err = m.AddContainer("v_main", ContainerSpec{Name: "App", Kind: "app"}, true, "agent")
	refused(t, err, `kind "app" is not a container kind`)
	_, _, err = m.AddContainer("v_main", ContainerSpec{}, true, "agent")
	refused(t, err, "give entity")
}

func TestFitContainerGrowsParent(t *testing.T) {
	m := geomModel(t)
	// push the block far right and down, fit its container: it follows, then e_core
	if _, err := m.MoveElements("v_main", []string{"e_a"}, nil, nil, f(700), f(600), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FitContainer("v_main", []string{"e_in"}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	a, in, core := rectOf(t, m, "e_a"), rectOf(t, m, "e_in"), rectOf(t, m, "e_core")
	// exactly the content plus padding and the caption strip: no grid rounding
	if in.X != 700-16 || in.Y != 600-16-28 || !in.Contains(a) {
		t.Fatalf("container %+v does not fit block %+v", in, a)
	}
	if !core.Contains(in) || core.X != 0 || core.Y != 0 {
		t.Fatalf("parent did not grow to hold the container: %+v ⊉ %+v", core, in)
	}
	_, err := m.FitContainer("v_main", []string{"e_a"}, true, "agent")
	refused(t, err, "e_a is not a container")
}

func TestAlignAndAtomicity(t *testing.T) {
	m := geomModel(t)
	if _, err := m.AlignElements("v_main", []string{"e_a", "e_b"}, "left", true, "agent"); err != nil {
		t.Fatal(err)
	}
	if rectOf(t, m, "e_b").X != 120 {
		t.Fatalf("left: %+v", rectOf(t, m, "e_b"))
	}
	if _, err := m.AlignElements("v_main", []string{"e_a", "e_b"}, "width", true, "agent"); err != nil {
		t.Fatal(err)
	}
	if rectOf(t, m, "e_b").Width != 100 {
		t.Fatalf("width: %+v", rectOf(t, m, "e_b"))
	}
	_, err := m.MoveElements("v_main", []string{"e_a", "e_missing"}, f(10), f(10), nil, nil, true, "agent")
	refused(t, err, "e_missing is not on view")
	if got := rectOf(t, m, "e_a"); got.X != 120 || got.Y != 140 {
		t.Fatalf("a failed step changed e_a: %+v", got)
	}
	_, err = m.AlignElements("v_main", []string{"e_a"}, "left", true, "agent")
	refused(t, err, "at least two")
	_, err = m.AlignElements("v_main", []string{"e_a", "e_b"}, "diagonal", true, "agent")
	refused(t, err, "mode is one of")
}
