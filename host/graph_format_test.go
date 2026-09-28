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
	gs := newGraphService(proj, models, nil)
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

// TestGraphEndpointNeighborhoodDefaultDoesNotWalkContains: `around` with no
// `follow` uses DefaultFollow, which does not include containment — so
// Mod's neighbourhood does not pull in the module's whole content.
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

// TestGraphEndpointFollowContainsWalksIt: follow=contains on the same
// neighbourhood request does walk containment.
func TestGraphEndpointFollowContainsWalksIt(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, body := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:Mod&depth=2&follow=contains")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %+v", res.StatusCode, body)
	}
	nodes, _ := body["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("expected Mod and Mod.A (contains walked), got %+v", nodes)
	}
}

// TestGraphEndpointAroundWithKindsRefused: `kinds` is only for a whole-graph
// request; a neighbourhood names `follow` instead.
func TestGraphEndpointAroundWithKindsRefused(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:Mod&kinds=extends")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for around+kinds both given, got %d", res.StatusCode)
	}
}

func TestGraphEndpointUnknownFollow(t *testing.T) {
	gs, _ := setsFixture(t)
	srv := graphServer(gs)
	defer srv.Close()
	res, _ := getGraphJSON(t, srv.URL+"/api/graph/p?around=csharp:Mod&follow=bogus")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown follow name, got %d", res.StatusCode)
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
	if len(formats) != 6 {
		t.Fatalf("expected 6 formats listed, got %+v", formats)
	}
	relations, _ := body["relations"].([]any)
	if len(relations) < 20 {
		t.Fatalf("expected the relation vocabulary (both directions of every relation), got %+v", relations)
	}
	defaults, _ := body["defaults"].(map[string]any)
	if defaults["format"] != "json" {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
}
