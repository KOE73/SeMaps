package core

import (
	"fmt"
	"strings"
	"testing"
)

// a copy of the live case: a parent zone, a reference zone with the base of a
// family and heirs, targets of the same make; one lacks a variant, one has an extra node.
func arrangeModel(t *testing.T) *Model {
	t.Helper()
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	var ops []Op
	add := func(kind, id, view, body string) { ops = append(ops, modelOp(kind, id, view, "", body)) }
	add("entity", "e_root", "", `{"id":"e_root","name":"Root","kind":"class","origin":"code","symbol":"N.Root"}`)
	rel := func(from, to string) {
		add("relation", fmt.Sprintf("r_%s_%s_extends", from, to), "", fmt.Sprintf(`{"id":"r_%s_%s_extends","from":"%s","to":"%s","type":"extends","origin":"code","status":"present"}`, from, to, from, to))
	}
	ent := func(id string) {
		add("entity", id, "", fmt.Sprintf(`{"id":"%s","name":"%s","kind":"class","origin":"code","symbol":"N.%s"}`, id, id, id))
	}
	add("zone", "z_t", "v_main", `{"id":"z_t","container":null,"x":2300,"y":300,"width":800,"height":1000}`)
	zone := func(id string, y, h int) {
		add("zone", id, "v_main", fmt.Sprintf(`{"id":"%s","container":"z_t","parent":null,"x":2360,"y":%d,"width":680,"height":%d}`, id, y, h))
	}
	node := func(id, zone string, x, y int, extra string) {
		add("node", id, "v_main", fmt.Sprintf(`{"id":"%s","container":"%s","x":%d,"y":%d,"width":280,"height":60%s}`, id, zone, x, y, extra))
	}
	// the reference
	zone("z_u", 400, 230)
	for _, id := range []string{"e_op_u_nchw_base", "e_op_u_u8_nhwc", "e_op_u_fp16_nchw", "e_op_u_fp32_nchw"} {
		ent(id)
	}
	rel("e_op_u_fp16_nchw", "e_op_u_nchw_base")
	rel("e_op_u_fp32_nchw", "e_op_u_nchw_base")
	rel("e_op_u_nchw_base", "e_root")
	rel("e_op_u_u8_nhwc", "e_root")
	node("e_op_u_nchw_base", "z_u", 2380, 450, `,"styleId":"class","template":"class"`)
	node("e_op_u_u8_nhwc", "z_u", 2380, 530, "")
	node("e_op_u_fp16_nchw", "z_u", 2710, 450, "")
	node("e_op_u_fp32_nchw", "z_u", 2710, 530, "")
	// a target of other names and without the u8 variant, its nodes thrown about
	zone("z_c", 660, 210)
	// ids are lower-case, only the names carry the words: RgbFP16Nchw
	for id, name := range map[string]string{"e_op_c_div255base": "Op_BgrU8Hwc_To_RgbNchw_Div255Base", "e_op_c_rgbfp16nchw_div255": "Op_BgrU8Hwc_To_RgbFP16Nchw_Div255", "e_op_c_rgbfp32nchw_div255": "Op_BgrU8Hwc_To_RgbFP32Nchw_Div255"} {
		add("entity", id, "", fmt.Sprintf(`{"id":"%s","name":"%s","kind":"class","origin":"code","symbol":"N.%s"}`, id, name, id))
	}
	rel("e_op_c_rgbfp16nchw_div255", "e_op_c_div255base")
	rel("e_op_c_rgbfp32nchw_div255", "e_op_c_div255base")
	rel("e_op_c_div255base", "e_root")
	node("e_op_c_div255base", "z_c", 2500, 800, "")
	node("e_op_c_rgbfp16nchw_div255", "z_c", 2380, 700, "")
	node("e_op_c_rgbfp32nchw_div255", "z_c", 2900, 690, "")
	// a target with all the variants and one more node
	zone("z_x", 900, 210)
	for _, id := range []string{"e_op_x_nchw_base", "e_op_x_u8_nhwc", "e_op_x_fp16_nchw", "e_op_x_fp32_nchw", "e_op_x_weird"} {
		ent(id)
	}
	rel("e_op_x_fp16_nchw", "e_op_x_nchw_base")
	rel("e_op_x_fp32_nchw", "e_op_x_nchw_base")
	rel("e_op_x_nchw_base", "e_root")
	rel("e_op_x_u8_nhwc", "e_root")
	node("e_op_x_fp32_nchw", "z_x", 2380, 940, "")
	node("e_op_x_fp16_nchw", "z_x", 2380, 1000, "")
	node("e_op_x_u8_nhwc", "z_x", 2700, 940, "")
	node("e_op_x_nchw_base", "z_x", 2700, 1000, "")
	node("e_op_x_weird", "z_x", 2500, 1060, "")
	if _, err := m.Apply(ops, "human"); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestArrangeLikeCarriesTheReferenceOver(t *testing.T) {
	m := arrangeModel(t)
	refBefore := rectOf(t, m, "z_u")
	rep, err := m.ArrangeLike("v_main", ArrangeOptions{Reference: "v_main#z_u", Targets: []string{"v_main#z_c", "z_x"}}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if rectOf(t, m, "z_u") != refBefore {
		t.Fatal("the reference moved")
	}
	// z_c sits at (2360, 660): every node at the offset the reference has in its zone
	c := rectOf(t, m, "z_c")
	want := map[string]Rect{
		"e_op_c_div255base":         {2380, 710, 280, 60},
		"e_op_c_rgbfp16nchw_div255": {2710, 710, 280, 60},
		"e_op_c_rgbfp32nchw_div255": {2710, 790, 280, 60},
	}
	for id, w := range want {
		if got := rectOf(t, m, id); got != w {
			t.Errorf("%s = %+v, want %+v", id, got, w)
		}
	}
	if c.Width != 680 || c.Height != 230 || c.X != 2360 || c.Y != 660 {
		t.Errorf("z_c = %+v", c)
	}
	x := rectOf(t, m, "z_x")
	if got := rectOf(t, m, "e_op_x_nchw_base"); got != (Rect{2380, 950, 280, 60}) {
		t.Errorf("x base = %+v", got)
	}
	if got := rectOf(t, m, "e_op_x_u8_nhwc"); got != (Rect{2380, 1030, 280, 60}) {
		t.Errorf("x u8 = %+v", got)
	}
	if got := rectOf(t, m, "e_op_x_fp32_nchw"); got != (Rect{2710, 1030, 280, 60}) {
		t.Errorf("x fp32 = %+v", got)
	}
	weird := rectOf(t, m, "e_op_x_weird")
	if weird.Y < x.Y+230 || !x.Contains(weird) {
		t.Errorf("the extra node is not in a free row inside a grown zone: %+v in %+v", weird, x)
	}
	byZone := map[string]ArrangeTarget{}
	for _, tg := range rep.Targets {
		byZone[tg.Zone] = tg
	}
	if got := byZone["z_c"]; len(got.Pairs) != 3 || len(got.Missing) != 1 || got.Missing[0] != "e_op_u_u8_nhwc" || len(got.Unpaired) != 0 {
		t.Errorf("z_c report = %+v", got)
	}
	if got := byZone["z_x"]; len(got.Pairs) != 4 || len(got.Unpaired) != 1 || got.Unpaired[0] != "e_op_x_weird" {
		t.Errorf("z_x report = %+v", got)
	}
	// style and template came along, and the parent still holds its zones
	doc, _ := m.view("v_main")
	for _, n := range viewItems(doc, "nodes") {
		if nodeID(n) == "e_op_c_div255base" && (n.str("styleId") != "class" || n.str("template") != "class") {
			t.Errorf("style/template not carried: %s", objString(n))
		}
	}
	if tz := rectOf(t, m, "z_t"); !tz.Contains(x) || !tz.Contains(c) {
		t.Errorf("parent %+v does not hold %+v %+v", tz, c, x)
	}
	if len(rep.Touched) == 0 || strings.Contains(strings.Join(rep.Touched, ","), "e_op_u_") {
		t.Errorf("touched = %v", rep.Touched)
	}
}

func TestArrangeLikeDryRunWritesNothing(t *testing.T) {
	m := arrangeModel(t)
	before := len(m.Dirty().Views["v_main"])
	c := rectOf(t, m, "e_op_c_div255base")
	rep, err := m.ArrangeLike("v_main", ArrangeOptions{Reference: "z_u", Targets: []string{"z_c"}, DryRun: true}, true, "agent")
	if err != nil || !rep.DryRun || len(rep.Touched) == 0 || len(rep.Targets[0].Pairs) != 3 {
		t.Fatalf("dry run: %+v, %v", rep, err)
	}
	if rectOf(t, m, "e_op_c_div255base") != c || len(m.Dirty().Views["v_main"]) != before {
		t.Fatal("dry run changed the model")
	}
}

func TestArrangeLikeParentRestacksWithTheReferenceGap(t *testing.T) {
	m := arrangeModel(t)
	rep, err := m.ArrangeLike("v_main", ArrangeOptions{Reference: "z_u", Parent: "z_t"}, true, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Targets) != 2 {
		t.Fatalf("targets = %+v", rep.Targets)
	}
	u, c, x := rectOf(t, m, "z_u"), rectOf(t, m, "z_c"), rectOf(t, m, "z_x")
	if u.Y != 400 {
		t.Fatalf("the reference moved: %+v", u)
	}
	// the gap of the reference to its neighbour was 660-630 = 30
	if c.Y != u.Bottom()+30 || x.Y < c.Bottom()+30 || c.X != u.X || x.X != u.X {
		t.Fatalf("stack: u %+v c %+v x %+v", u, c, x)
	}
	if got := rectOf(t, m, "e_op_c_div255base"); got.Y != c.Y+50 || got.X != c.X+20 {
		t.Fatalf("node did not follow its zone: %+v in %+v", got, c)
	}
	if tz := rectOf(t, m, "z_t"); !tz.Contains(x) {
		t.Fatalf("parent %+v does not hold %+v", tz, x)
	}
}

func TestArrangeLikeRefusals(t *testing.T) {
	m := arrangeModel(t)
	_, err := m.ArrangeLike("v_main", ArrangeOptions{Reference: "z_u", Targets: []string{"z_c"}}, false, "agent")
	refused(t, err, "direct request")
	_, err = m.ArrangeLike("v_main", ArrangeOptions{Reference: "e_op_u_nchw_base", Targets: []string{"z_c"}}, true, "agent")
	refused(t, err, "not a zone")
	_, err = m.ArrangeLike("v_main", ArrangeOptions{Reference: "z_u", Targets: []string{"z_u"}}, true, "agent")
	refused(t, err, "own target")
	_, err = m.ArrangeLike("v_main", ArrangeOptions{Reference: "z_u", Targets: []string{"z_nope"}}, true, "agent")
	refused(t, err, "z_nope is not on view")
	_, err = m.ArrangeLike("v_main", ArrangeOptions{Reference: "z_u"}, true, "agent")
	refused(t, err, "give targets or parent")
}
