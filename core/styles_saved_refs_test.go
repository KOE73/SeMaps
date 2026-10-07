package core

import "testing"

func TestStylesSaveRenamesSavedReferencesHiddenByPendingAssignments(t *testing.T) {
	w := stylesFixture(t)
	for _, id := range []string{"p", "q"} {
		m, err := w.Project(id)
		if err != nil {
			t.Fatal(err)
		}
		changeStylePlacement(t, m, func(o *object) { o.set("styleId", "variant"); o.set("x", 123) })
	}
	detail, err := w.GetStyle("base")
	if err != nil {
		t.Fatal(err)
	}
	saved := 0
	for _, use := range detail.Usage {
		if use.Kind == "saved-placement" {
			saved++
		}
	}
	if saved != 2 {
		t.Fatalf("hidden saved references absent: %+v", detail.Usage)
	}
	editTestStyle(t, w, StyleEdit{Action: "rename", ID: "base", NewID: "base.new"})
	if err := w.SaveStyles(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"p", "q"} {
		disk, err := LoadModelWithoutJournal(w.root, id, styleTestKinds)
		if err != nil {
			t.Fatal(err)
		}
		if got := stylePlacement(t, disk); got.str("styleId") != "base.new" || string(got.vals["x"]) != "10" {
			t.Fatalf("saved view invalid or assignment persisted: %s", encodeJSON(got))
		}
		working, _ := w.Project(id)
		if got := stylePlacement(t, working); got.str("styleId") != "variant" || string(got.vals["x"]) != "123" {
			t.Fatalf("working assignment lost: %s", encodeJSON(got))
		}
		if err := working.Discard("all", ""); err != nil {
			t.Fatal(err)
		}
		if got := stylePlacement(t, working).str("styleId"); got != "base.new" {
			t.Fatal(got)
		}
	}
}

func TestInternalStyleReferenceOperationCannotBypassWorkspaceValidation(t *testing.T) {
	w := stylesFixture(t)
	m, _ := w.Project("p")
	if _, err := m.Apply([]Op{{Kind: "style_ref", ID: "base", Value: encodeJSON("missing")}}, "agent"); err == nil {
		t.Fatal("public operation bypassed style validation")
	}
}
