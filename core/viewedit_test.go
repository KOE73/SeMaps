package core

import "testing"

func f(v float64) *float64 { return &v }

func geomModel(t *testing.T) *Model {
	t.Helper()
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	ops := []Op{
		modelOp("zone", "z_core", "v_main", "", `{"id":"z_core","container":null,"x":0,"y":0,"width":500,"height":500}`),
		modelOp("zone", "z_in", "v_main", "", `{"id":"z_in","container":null,"parent":"z_core","x":100,"y":100,"width":200,"height":150}`),
		modelOp("node", "e_a", "v_main", "", `{"entity":"e_a","zone":"z_in","x":120,"y":140,"width":100,"height":40}`),
		modelOp("node", "e_b", "v_main", "", `{"entity":"e_b","zone":null,"x":600,"y":600}`),
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
	if l.zones[id] == nil && l.nodes[id] == nil {
		t.Fatalf("no %s", id)
	}
	return l.rect(id)
}

func TestGeomStepsNeedAHuman(t *testing.T) {
	m := geomModel(t)
	_, err := m.MoveElements("v_main", []string{"e_b"}, f(10), f(0), nil, nil, false, "agent")
	refused(t, err, "direct request")
	_, err = m.FitZone("v_main", []string{"z_in"}, false, "agent")
	refused(t, err, "direct request")
}

func TestMoveElementsZoneGoesWithContent(t *testing.T) {
	m := geomModel(t)
	rep, err := m.MoveElements("v_main", []string{"v_main#z_in"}, f(30), f(-20), nil, nil, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "z_in"); got.X != 130 || got.Y != 80 {
		t.Fatalf("zone = %+v", got)
	}
	if got := rectOf(t, m, "e_a"); got.X != 150 || got.Y != 120 || got.Width != 100 {
		t.Fatalf("node inside = %+v", got)
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
	// to a point: the box of the elements lands there; a node without size gets none
	if _, err := m.MoveElements("v_main", []string{"e_b"}, nil, nil, f(700), f(650), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "e_b"); got.X != 700 || got.Y != 650 {
		t.Fatalf("to point = %+v", got)
	}
	doc, _ := m.view("v_main")
	for _, n := range viewItems(doc, "nodes") {
		if nodeID(n) == "e_b" && has(n, "width") {
			t.Fatal("moving gave e_b a size")
		}
	}
	// moving a node out of its zone's reach grows the zone
	if _, err := m.MoveElements("v_main", []string{"e_a"}, f(400), f(0), nil, nil, true, "agent"); err != nil {
		t.Fatal(err)
	}
	if z := rectOf(t, m, "z_in"); z.Right() < rectOf(t, m, "e_a").Right()+ZonePadding {
		t.Fatalf("zone did not grow: %+v", z)
	}
}

func TestResizeElementsMinimumsAndContent(t *testing.T) {
	m := geomModel(t)
	if _, err := m.ResizeElements("v_main", []string{"e_a"}, f(20), f(10), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "e_a"); got.Width != MinNodeWidth || got.Height != MinNodeHeight {
		t.Fatalf("node = %+v", got)
	}
	// the zone holds e_a (right edge 220): it cannot shrink under it
	if _, err := m.ResizeElements("v_main", []string{"z_in"}, f(50), f(50), true, "agent"); err != nil {
		t.Fatal(err)
	}
	z := rectOf(t, m, "z_in")
	if z.Width < 140 || z.Right() < 220+ZonePadding || z.Bottom() < 180+ZonePadding {
		t.Fatalf("zone shrank under its content: %+v", z)
	}
	if z.Width < MinZoneWidth || z.Height < MinZoneHeight {
		t.Fatalf("zone under minimum: %+v", z)
	}
}

func TestSetZoneAndCycle(t *testing.T) {
	m := geomModel(t)
	if _, err := m.SetZone("v_main", []string{"e_b"}, "z_core", true, "agent"); err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "e_b"); got.X != 600 {
		t.Fatalf("coordinates moved: %+v", got)
	}
	v, _ := m.GetView("v_main", "ru")
	if len(v.Zones[0].Nodes) != 1 || v.Zones[0].Nodes[0].Entity != "e_b" {
		t.Fatalf("e_b not in z_core: %+v", v.Zones[0])
	}
	_, err := m.SetZone("v_main", []string{"z_core"}, "z_in", true, "agent")
	refused(t, err, "cannot go into itself")
	_, err = m.SetZone("v_main", []string{"e_b"}, "z_nope", true, "agent")
	refused(t, err, "z_nope is not on view")
	if _, err := m.SetZone("v_main", []string{"e_b"}, "", true, "agent"); err != nil {
		t.Fatal(err)
	}
}

func TestAddZoneWithName(t *testing.T) {
	m := geomModel(t)
	_, err := m.AddZone("v_main", ZoneSpec{ID: "z_new", Parent: "z_core", Name: "Новая", Rect: Rect{X: 13, Y: 268, Width: 50, Height: 50}}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if got := rectOf(t, m, "z_new"); got != (Rect{10, 270, MinZoneWidth, MinZoneHeight}) {
		t.Fatalf("new zone = %+v", got)
	}
	v, _ := m.GetView("v_main#z_new", "ru")
	if v.Zones[0].Name != "Новая" || v.Zones[0].Parent != "z_core" {
		t.Fatalf("zone = %+v", v.Zones[0])
	}
	_, err = m.AddZone("v_main", ZoneSpec{ID: "z_new"}, true, "agent")
	refused(t, err, "already exists")
	_, err = m.AddZone("v_main", ZoneSpec{ID: "new"}, true, "agent")
	refused(t, err, "starts with z_")
}

func TestFitZoneGrowsParent(t *testing.T) {
	m := geomModel(t)
	if _, err := m.MoveElements("v_main", []string{"e_a"}, f(0), f(0), nil, nil, true, "agent"); err == nil {
		t.Log("no-op move is fine")
	}
	// push the node far right and down, fit its zone: the zone follows, then z_core
	if _, err := m.MoveElements("v_main", []string{"e_a"}, nil, nil, f(700), f(600), true, "agent"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FitZone("v_main", []string{"z_in"}, true, "agent"); err != nil {
		t.Fatal(err)
	}
	a, in, core := rectOf(t, m, "e_a"), rectOf(t, m, "z_in"), rectOf(t, m, "z_core")
	if in.X != 680 || in.Y != 550 || !in.Contains(a) {
		t.Fatalf("zone %+v does not fit node %+v", in, a)
	}
	if !core.Contains(in) || core.X != 0 || core.Y != 0 {
		t.Fatalf("parent did not grow to hold the zone: %+v ⊉ %+v", core, in)
	}
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
