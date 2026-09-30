package core

import (
	"strings"
	"testing"
)

// The editor's raw ops meet the rules the agent's verbs meet (ADR_20260926).
func TestApplyChecksContractForEveryWriter(t *testing.T) {
	refused := []struct {
		name, want string
		ops        []Op
	}{
		{"entity without e_", "starts with e_", []Op{modelOp("entity", "node_1", "", "", `{"id":"node_1","name":"N","kind":"concept"}`)}},
		{"entity without name", "name is empty", []Op{modelOp("entity", "e_n", "", "", `{"id":"e_n","name":" ","kind":"concept"}`)}},
		{"relation to nothing", "no entity e_ghost", []Op{modelOp("relation", "r_x", "", "", `{"id":"r_x","from":"e_a","to":"e_ghost","type":"call"}`)}},
		{"relation of unknown type", "no relation type", []Op{modelOp("relation", "r_x", "", "", `{"id":"r_x","from":"e_a","to":"e_b","type":"nope"}`)}},
		{"empty text", "empty text", []Op{modelOp("text", "e_a", "", "ru", `{"description":{"v":"","origin":"authored","at":"2026-09-26T00:00:00Z"}}`)}},
		{"bare text", "a value {v, origin, at}", []Op{modelOp("text", "rt_call", "", "ru", `{"name":"Вызов"}`)}},
		{"text of a zone", "expected a prefix e_, rt_, r_ or v_", []Op{modelOp("text", "z_core", "", "ru", `{"name":{"v":"Ядро","origin":"authored","at":"2026-09-26T00:00:00Z"}}`)}},
		{"zone op", "unknown operation kind", []Op{modelOp("zone", "z_in", "v_main", "", `{"id":"z_in","x":0,"y":0,"width":200,"height":100}`)}},
		{"node op", "unknown operation kind", []Op{modelOp("node", "e_b", "v_main", "", `{"entity":"e_b","x":0,"y":0}`)}},
		{"parent into a block", "is not a container", []Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":"e_a","x":0,"y":0}`)}},
		{"placement into itself", "cannot go into itself", []Op{modelOp("placement", "e_core", "v_main", "", `{"entity":"e_core","parent":"e_core","x":0,"y":0,"width":500,"height":500}`)}},
		{"placement of no entity", "no entity e_ghost", []Op{modelOp("placement", "e_ghost", "v_main", "", `{"entity":"e_ghost","parent":null,"x":0,"y":0}`)}},
		{"placement in no container", "no container e_ghost", []Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":"e_ghost","x":0,"y":0}`)}},
		{"placement without parent", "parent is required", []Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","x":0,"y":0}`)}},
		{"override outside the table", "cannot be overridden", []Op{modelOp("placement", "e_b", "v_main", "", `{"entity":"e_b","parent":null,"override":{"radius":4}}`)}},
	}
	for _, c := range refused {
		m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
		if err != nil {
			t.Fatal(err)
		}
		_, err = m.Apply(c.ops, "human")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
		if len(m.Dirty().Registry)+len(m.Dirty().Views) != 0 {
			t.Errorf("%s: refused batch left changes", c.name)
		}
	}
}

func TestApplyChecksTheWholeBatchAndOnlyWhatChanged(t *testing.T) {
	m, err := LoadModel(editWorkspace(t), "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	// order inside a batch does not matter: the relation comes before its type,
	// the placement before its entity
	ok := []Op{
		modelOp("relation", "r_a_n_uses", "", "", `{"id":"r_a_n_uses","from":"e_a","to":"e_n","type":"uses","origin":"code"}`),
		modelOp("placement", "e_n", "v_main", "", `{"entity":"e_n","parent":"e_core","x":0,"y":0}`),
		modelOp("entity", "e_n", "", "", `{"id":"e_n","name":"N","kind":"class","origin":"code"}`),
		modelOp("relationType", "uses", "", "", `{"id":"uses","origin":"code","visibility":"hidden"}`),
	}
	if _, err := m.Apply(ok, "sync"); err != nil {
		t.Fatal(err)
	}
	// data already there that breaks a rule does not block editing it elsewhere:
	// a placement whose override is out of the table, put in by a journal replay
	legacy := modelOp("placement", "e_x", "v_main", "", `{"entity":"e_x","parent":null,"x":0,"y":0,"override":{"radius":3}}`)
	if err := m.applyUnchecked(legacy); err != nil {
		t.Fatal(err)
	}
	moved := modelOp("placement", "e_x", "v_main", "", `{"entity":"e_x","parent":null,"x":40,"y":0,"override":{"radius":3}}`)
	if _, err := m.Apply([]Op{moved}, "human"); err != nil {
		t.Fatalf("moving a placement with a legacy override: %v", err)
	}
	// but a change to the override itself is checked
	worse := modelOp("placement", "e_x", "v_main", "", `{"entity":"e_x","parent":null,"x":40,"y":0,"override":{"radius":9}}`)
	if _, err := m.Apply([]Op{worse}, "human"); err == nil {
		t.Fatal("a changed override outside the table was accepted")
	}
}

// applyUnchecked stands for data already on disk: journal replay skips the rules.
func (m *Model) applyUnchecked(op Op) error {
	op.Author = "human"
	_, err := m.apply([]Op{op}, "", true)
	return err
}
