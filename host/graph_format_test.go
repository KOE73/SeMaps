package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"semaps/core"
)

// setsFixture builds a workspace whose graph has a `contains` edge (module
// containing A) alongside an `extends` edge, so the default neighbourhood
// set (links) and set=containment can be told apart by what they walk.
func setsFixture(t *testing.T) (*graphService, *modelService) {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json":   `{"id":"p"}`,
		"entities.json":  `{"entities":[]}`,
		"relations.json": `{"relations":[]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	models, err := newModelService(ws)
	if err != nil {
		t.Fatal(err)
	}
	semapsFile := filepath.Join(ws, "test.semaps")
	extractors := []extractorConf{{ID: "csharp", Language: "csharp", Project: "p"}}
	proj := project{File: semapsFile, Extractors: extractors}
	gs := newGraphService(proj, models)
	store := newRunStore(semapsFile)
	info := &runInfo{ID: "20260101-000000-csharp", Extractor: "csharp", Project: "p", Language: "csharp", State: "done"}
	if err := os.MkdirAll(filepath.Join(store.dir, info.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	facts := core.Facts{Language: "csharp", Root: ".", Symbols: []core.Symbol{
		{ID: "Mod", Kind: "module", NativeKind: "file", Name: "Mod", File: "Mod.cs"},
		{ID: "Mod.A", Kind: "type", NativeKind: "class", Name: "A", File: "Mod.cs"},
		{ID: "Mod.B", Kind: "type", NativeKind: "class", Name: "B", File: "Mod.cs"},
	}, Edges: []core.Edge{
		{From: "Mod", To: "Mod.A", Kind: "contains"},
		{From: "Mod.A", To: "Mod.B", Kind: "extends"},
	}}
	b, _ := json.Marshal(facts)
	if err := os.WriteFile(store.path(info.ID, "facts.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	info.Finished = time.Now()
	if err := store.save(info); err != nil {
		t.Fatal(err)
	}
	return gs, models
}

// TestGraphEndpointWholeGraphDefaultUnchanged: a plain GET (no `set`, no
// `kinds`, no `around`) must keep walking/keeping every edge kind, exactly
// as before named sets existed — the graph page depends on this.
func TestGraphEndpointWholeGraphDefaultUnchanged(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	edges, _ := body["edges"].([]any)
	if len(edges) != 2 {
		t.Fatalf("expected both edges (contains and extends) in a plain whole-graph request, got %+v", edges)
	}
}

// TestGraphEndpointNeighborhoodDefaultDoesNotWalkContains: `around` with
// neither `set` nor `kinds` uses the default neighbourhood set (links),
// which does not include `contains` — so Mod's neighbourhood does not pull
// in the module's whole content.
func TestGraphEndpointNeighborhoodDefaultDoesNotWalkContains(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:Mod&depth=2")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected only Mod itself (contains not walked by default), got %+v", nodes)
	}
}

// TestGraphEndpointSetContainmentWalksContains: set=containment on the same
// neighbourhood request does walk `contains`.
func TestGraphEndpointSetContainmentWalksContains(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:Mod&depth=2&set=containment")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("expected Mod and Mod.A (contains walked), got %+v", nodes)
	}
}

func TestGraphEndpointSetAndKindsConflict(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?set=links&kinds=extends")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for set+kinds both given, got %d", res.StatusCode)
	}
}

func TestGraphEndpointUnknownSet(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?set=bogus")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown set, got %d", res.StatusCode)
	}
}

func TestGraphEndpointUnknownFormat(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/graph/p?format=bogus")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown format, got %d", res.StatusCode)
	}
}

func TestGraphEndpointTreeWithoutAroundRefused(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/graph/p?format=tree")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for format=tree without around, got %d", res.StatusCode)
	}
}

func TestGraphEndpointLinesFormatContentType(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/graph/p?format=lines")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected a text/plain Content-Type for format=lines, got %q", ct)
	}
}

func TestGraphFormatsEndpoint(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph-formats")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	formats, _ := body["formats"].([]any)
	if len(formats) != 5 {
		t.Fatalf("expected 5 formats listed, got %+v", formats)
	}
	sets, _ := body["sets"].([]any)
	if len(sets) != 5 {
		t.Fatalf("expected 5 sets listed, got %+v", sets)
	}
	defaults, _ := body["defaults"].(map[string]any)
	if defaults["format"] != "json" || defaults["set"] != "all" || defaults["neighbourhoodSet"] != "links" {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
}
