package core

import (
	"reflect"
	"testing"
)

func TestResolveContainersName(t *testing.T) {
	defs := []Container{
		{ID: "c_llm", Match: ContainerMatch{Name: []string{"ILlmMiddleware"}}},
	}
	got := ResolveContainers(defs, nil, []ContainerSubject{{ID: "e_x", Name: "ILlmMiddleware"}})
	want := []string{"c_llm"}
	if !reflect.DeepEqual(got["e_x"], want) {
		t.Fatalf("got %v, want %v", got["e_x"], want)
	}
}

func TestResolveContainersNameRegex(t *testing.T) {
	defs := []Container{
		{ID: "c_guards", Match: ContainerMatch{NameRegex: []string{"^.*Guard$"}}},
	}
	got := ResolveContainers(defs, nil, []ContainerSubject{{ID: "e_x", Name: "RepetitionGuard"}})
	if want := []string{"c_guards"}; !reflect.DeepEqual(got["e_x"], want) {
		t.Fatalf("got %v, want %v", got["e_x"], want)
	}
	// A name that only satisfies name-regex, not name, still resolves.
	got2 := ResolveContainers(defs, nil, []ContainerSubject{{ID: "e_y", Name: "OtherGuard"}})
	if want := []string{"c_guards"}; !reflect.DeepEqual(got2["e_y"], want) {
		t.Fatalf("got %v, want %v", got2["e_y"], want)
	}
}

func TestResolveContainersPathLongestPrefix(t *testing.T) {
	defs := []Container{
		{ID: "c_llm", Match: ContainerMatch{Path: []string{"src/Acme.Domain/"}}},
		{ID: "c_llm_middleware", Match: ContainerMatch{Path: []string{"src/Acme.Domain/Llm/Middleware/"}}},
	}
	got := ResolveContainers(defs, nil, []ContainerSubject{
		{ID: "e_x", Name: "SomeMiddleware", File: "src/Acme.Domain/Llm/Middleware/SomeMiddleware.cs"},
	})
	if want := []string{"c_llm_middleware"}; !reflect.DeepEqual(got["e_x"], want) {
		t.Fatalf("longest prefix should win: got %v, want %v", got["e_x"], want)
	}
}

func TestResolveContainersPathTieYieldsBoth(t *testing.T) {
	defs := []Container{
		{ID: "c_a", Match: ContainerMatch{Path: []string{"src/Acme/"}}},
		{ID: "c_b", Match: ContainerMatch{Path: []string{"src/Acme/"}}},
	}
	got := ResolveContainers(defs, nil, []ContainerSubject{
		{ID: "e_x", Name: "Thing", File: "src/Acme/Thing.cs"},
	})
	if want := []string{"c_a", "c_b"}; !reflect.DeepEqual(got["e_x"], want) {
		t.Fatalf("a tie should list both, in containers.json order: got %v, want %v", got["e_x"], want)
	}
}

func TestResolveContainersFileNeighbour(t *testing.T) {
	defs := []Container{
		{ID: "c_llm", Match: ContainerMatch{Name: []string{"ILlmMiddleware"}}},
	}
	subjects := []ContainerSubject{
		{ID: "e_iface", Name: "ILlmMiddleware", File: "src/Acme/Llm/ILlmMiddleware.cs"},
		{ID: "e_impl", Name: "RepetitionGuardMiddleware", File: "src/Acme/Llm/ILlmMiddleware.cs"},
	}
	got := ResolveContainers(defs, nil, subjects)
	if want := []string{"c_llm"}; !reflect.DeepEqual(got["e_iface"], want) {
		t.Fatalf("the matched neighbour: got %v, want %v", got["e_iface"], want)
	}
	if want := []string{"c_llm"}; !reflect.DeepEqual(got["e_impl"], want) {
		t.Fatalf("adopts its file neighbour's container: got %v, want %v", got["e_impl"], want)
	}
}

func TestResolveContainersOverrideWinsOverName(t *testing.T) {
	defs := []Container{
		{ID: "c_llm", Match: ContainerMatch{Name: []string{"RepetitionGuardMiddleware"}}},
		{ID: "c_guards"},
	}
	overrides := map[string]string{"e_x": "c_guards"}
	got := ResolveContainers(defs, overrides, []ContainerSubject{
		{ID: "e_x", Name: "RepetitionGuardMiddleware", File: "src/x.cs"},
	})
	if want := []string{"c_guards"}; !reflect.DeepEqual(got["e_x"], want) {
		t.Fatalf("override must replace the rule match: got %v, want %v", got["e_x"], want)
	}
}

func TestResolveContainersOrderOfTiers(t *testing.T) {
	// A subject matching by both name and path must resolve by name (the
	// stronger tier), not by path.
	defs := []Container{
		{ID: "c_by_path", Match: ContainerMatch{Path: []string{"src/"}}},
		{ID: "c_by_name", Match: ContainerMatch{Name: []string{"Thing"}}},
	}
	got := ResolveContainers(defs, nil, []ContainerSubject{
		{ID: "e_x", Name: "Thing", File: "src/Thing.cs"},
	})
	if want := []string{"c_by_name"}; !reflect.DeepEqual(got["e_x"], want) {
		t.Fatalf("name must win over path: got %v, want %v", got["e_x"], want)
	}
}

func TestResolveContainersNoMatchIsAbsent(t *testing.T) {
	defs := []Container{{ID: "c_a", Match: ContainerMatch{Name: []string{"X"}}}}
	got := ResolveContainers(defs, nil, []ContainerSubject{{ID: "e_x", Name: "Y", File: "src/y.cs"}})
	if ids, ok := got["e_x"]; ok || len(ids) != 0 {
		t.Fatalf("no match should leave the subject out of the result: got %v", ids)
	}
}

func TestLoadContainersMissingFileIsNil(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadContainers(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil containers, got %+v", c)
	}
}

func TestLoadContainers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/containers.json", `{
		"contractVersion": 3,
		"containers": [
			{ "id": "c_llm", "parent": null, "theme": "green" },
			{ "id": "c_llm_middleware", "parent": "c_llm",
			  "match": { "name": ["ILlmMiddleware"], "path": ["src/Acme.Domain/Llm/Middleware/"] } }
		],
		"overrides": { "e_repetitionguardmiddleware": "c_llm_middleware" }
	}`)
	c, err := LoadContainers(dir)
	if err != nil {
		t.Fatalf("LoadContainers: %v", err)
	}
	if len(c.List) != 2 || c.List[1].ID != "c_llm_middleware" {
		t.Fatalf("unexpected containers: %+v", c.List)
	}
	if c.Overrides["e_repetitionguardmiddleware"] != "c_llm_middleware" {
		t.Fatalf("unexpected overrides: %+v", c.Overrides)
	}
}
