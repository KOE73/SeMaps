package core

import (
	"encoding/json"
	"testing"
)

func TestLiveIndexSourceFilterFollowsWorkingModel(t *testing.T) {
	ws := editWorkspace(t)
	m, err := LoadModel(ws, "p", defaultKindsJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(m.Manifest(), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["sources"] = map[string]any{"include": []string{"src", "labs"}}
	value, _ := json.Marshal(manifest)
	if _, err := m.Apply([]Op{{Kind: "project", ID: "p", Value: value}}, "human"); err != nil {
		t.Fatal(err)
	}
	check := func(index WorkspaceIndex, want string) {
		t.Helper()
		for _, p := range index.Projects {
			if p.ID == "p" {
				if string(p.Sources) != want {
					t.Fatalf("sources = %s, want %s", p.Sources, want)
				}
				return
			}
		}
		t.Fatal("project missing")
	}
	check(LiveIndex(ws, func(string) (*Model, error) { return m, nil }), `{"include":["src","labs"]}`)
	check(Index(ws), "")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(Index(ws).Projects[0].Sources, &saved); err != nil {
		t.Fatal(err)
	}
	includes := saved["include"].([]any)
	if len(includes) != 2 || includes[1] != "labs" {
		t.Fatalf("saved filter = %v", saved)
	}
	delete(manifest, "sources")
	value, _ = json.Marshal(manifest)
	if _, err := m.Apply([]Op{{Kind: "project", ID: "p", Value: value}}, "human"); err != nil {
		t.Fatal(err)
	}
	check(LiveIndex(ws, func(string) (*Model, error) { return m, nil }), "")
}
