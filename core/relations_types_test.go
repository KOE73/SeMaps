package core

import (
	"slices"
	"testing"
)

// RelationsOfTypes is what the tools' `types` selector expands to: the registry
// relations of exactly those types whose two ends are both placed on the view
// now. No stored shape; a relation of the type added later is not in the answer
// of an earlier call.
func TestRelationsOfTypesExpandsToWhatIsOnTheViewNow(t *testing.T) {
	m, err := loadVisibility(t, visibilityWorkspace(t, `"relations":{"default":"visible"},`))
	if err != nil {
		t.Fatal(err)
	}
	// r_vis and r_a_c are of type vis; e_c is not placed, so r_a_c is not on the view
	got, err := m.RelationsOfTypes("v_main", []string{"vis"})
	if err != nil || !slices.Equal(got, []string{"r_vis"}) {
		t.Fatalf("%v %v", got, err)
	}
	if got, _ := m.RelationsOfTypes("v_main", []string{"vis", "hid"}); !slices.Equal(got, []string{"r_vis", "r_hid"}) {
		t.Fatalf("two types: %v", got)
	}
	// exact ids: "vi" is not "vis"; a type nothing has gives an empty list, not an error
	if got, err := m.RelationsOfTypes("v_main", []string{"vi", "nothing"}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no match: %v %v", got, err)
	}
	if _, err := m.RelationsOfTypes("v_none", []string{"vis"}); err == nil {
		t.Fatal("no such view")
	}

	// the expansion feeds the same write as a list of ids; a later relation is not covered
	ids, _ := m.RelationsOfTypes("v_main", []string{"none"})
	if _, err := m.SetRelationsVisible("v_main", ids, false, "agent"); err != nil {
		t.Fatal(err)
	}
	if drawn(t, m, "r_none") {
		t.Fatal("hidden")
	}
	later, err := m.AddRelation("e_b", "e_a", "none", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if !drawn(t, m, later) {
		t.Fatal("a relation of the type added later follows the view's defaults")
	}
}
