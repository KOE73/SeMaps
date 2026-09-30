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
		{"bare text", "a value {v, origin, at}", []Op{modelOp("text", "z_core", "", "ru", `{"name":"Ядро"}`)}},
		{"zone without z_", "starts with z_", []Op{modelOp("zone", "zone_1", "v_main", "", `{"id":"zone_1","x":0,"y":0,"width":200,"height":100}`)}},
		{"zone into a node", "is not a zone", []Op{modelOp("zone", "z_in", "v_main", "", `{"id":"z_in","parent":"e_a","x":0,"y":0,"width":200,"height":100}`)}},
		{"zone into itself", "cannot go into itself", []Op{modelOp("zone", "z_core", "v_main", "", `{"id":"z_core","parent":"z_core","x":0,"y":0,"width":500,"height":500}`)}},
		{"node of no entity", "no entity e_ghost", []Op{modelOp("node", "e_ghost", "v_main", "", `{"entity":"e_ghost","x":0,"y":0}`)}},
		{"node in no zone", "no zone z_ghost", []Op{modelOp("node", "e_b", "v_main", "", `{"entity":"e_b","zone":"z_ghost","x":0,"y":0}`)}},
	}
	for _, c := range refused {
		m, err := LoadModel(editWorkspace(t), "p")
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
	m, err := LoadModel(editWorkspace(t), "p")
	if err != nil {
		t.Fatal(err)
	}
	// order inside a batch does not matter: the relation comes before its type,
	// the node before its entity
	ok := []Op{
		modelOp("relation", "r_a_n_uses", "", "", `{"id":"r_a_n_uses","from":"e_a","to":"e_n","type":"uses","origin":"code"}`),
		modelOp("node", "e_n", "v_main", "", `{"entity":"e_n","zone":"z_core","x":0,"y":0}`),
		modelOp("entity", "e_n", "", "", `{"id":"e_n","name":"N","kind":"class","origin":"code"}`),
		modelOp("relationType", "uses", "", "", `{"id":"uses","origin":"code","visibility":"hidden"}`),
	}
	if _, err := m.Apply(ok, "sync"); err != nil {
		t.Fatal(err)
	}
	// legacy data that breaks a rule does not block editing it elsewhere
	legacy := modelOp("zone", "zone_old", "v_main", "", `{"id":"zone_old","x":0,"y":0,"width":200,"height":100}`)
	if err := m.applyUnchecked(legacy); err != nil {
		t.Fatal(err)
	}
	moved := modelOp("zone", "zone_old", "v_main", "", `{"id":"zone_old","x":40,"y":0,"width":200,"height":100}`)
	if _, err := m.Apply([]Op{moved}, "human"); err != nil {
		t.Fatalf("moving a legacy zone: %v", err)
	}
}

// applyUnchecked stands for data already on disk: journal replay skips the rules.
func (m *Model) applyUnchecked(op Op) error {
	op.Author = "human"
	_, err := m.apply([]Op{op}, "", true)
	return err
}
