package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// paramNames: the json field names (without ",omitempty") of a tool input
// struct, via reflection — the "exists in the tool's input struct" side of
// the description check below.
func paramNames(v any) map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

// TestMCPGraphToolsPerSet: `one` registers exactly get_graph/find_node/graph_formats;
// `narrow` registers exactly the ten narrow tools and NOT get_graph/graph_formats
// (step 2/3/4).
func TestMCPGraphToolsPerSet(t *testing.T) {
	for _, tc := range []struct {
		set  string
		want []string
		not  []string
	}{
		{"one", []string{"get_graph", "find_node", "graph_formats"}, []string{"who_extends", "who_calls"}},
		{"narrow", []string{"who_extends", "what_it_extends", "who_holds", "what_it_holds", "who_calls",
			"what_it_calls", "where_created", "what_is_inside", "where_it_lies", "find_node"}, []string{"get_graph", "graph_formats"}},
	} {
		t.Run(tc.set, func(t *testing.T) {
			ctx := context.Background()
			s := &mcpServer{}
			s.proj.Mcp = mcpSettings{Tools: tc.set}.withDefaults()
			st, ct := mcp.NewInMemoryTransports()
			if _, err := s.server().Connect(ctx, st, nil); err != nil {
				t.Fatal(err)
			}
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			res, err := cs.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			have := map[string]bool{}
			for _, tl := range res.Tools {
				have[tl.Name] = true
			}
			for _, n := range tc.want {
				if !have[n] {
					t.Errorf("set %s: missing tool %s", tc.set, n)
				}
			}
			for _, n := range tc.not {
				if have[n] {
					t.Errorf("set %s: unexpected tool %s", tc.set, n)
				}
			}
		})
	}
}

// TestMCPDescriptionLevelIsFromDataFile: the tool's live description text,
// at each level, is exactly what mcp_descriptions.go holds for it — not
// assembled or truncated on the way.
func TestMCPDescriptionLevelIsFromDataFile(t *testing.T) {
	for _, level := range []string{"brief", "standard", "full"} {
		s := &mcpServer{}
		s.proj.Mcp = mcpSettings{Tools: "one", Description: level}.withDefaults()
		got := toolDescByName(t, s, "get_graph")
		want := graphToolDescriptions["get_graph"].at(level)
		if got != want {
			t.Fatalf("level %s: description does not match the data file.\ngot:  %s\nwant: %s", level, got, want)
		}
	}
}

func toolDescByName(t *testing.T, s *mcpServer, name string) string {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range res.Tools {
		if tl.Name == name {
			return tl.Description
		}
	}
	t.Fatalf("no tool %s", name)
	return ""
}

// TestMCPDescriptionsNoForbiddenReferences: no level of any graph tool
// mentions a removed parameter's marker (`set=`), an ADR, or a repository
// path/markdown file — the agent reading these words works in another
// repository and cannot open ours (step 4 rules).
func TestMCPDescriptionsNoForbiddenReferences(t *testing.T) {
	forbidden := []string{"set=", "ADR", "docs/", ".md"}
	check := func(tool, level, text string) {
		for _, f := range forbidden {
			if strings.Contains(text, f) {
				t.Errorf("%s/%s: description contains forbidden %q:\n%s", tool, level, f, text)
			}
		}
	}
	for name, d := range graphToolDescriptions {
		check(name, "brief", d.Brief)
		check(name, "standard", d.Standard)
		check(name, "full", d.Full)
	}
	for name, nt := range narrowTools {
		d := narrowDescriptions(name, nt)
		check(name, "brief", d.Brief)
		check(name, "standard", d.Standard)
		check(name, "full", d.Full)
	}
}

// TestMCPDescriptionsOnlyNameRealParameters: every backtick-quoted parameter
// name a description mentions exists in that tool's own input struct — the
// defect that started this task was get_graph's description naming `set`,
// long since removed from getGraphIn.
func TestMCPDescriptionsOnlyNameRealParameters(t *testing.T) {
	// The vocabulary of parameter-shaped words across every graph tool's
	// input struct, PLUS the one word known to have been removed from
	// getGraphIn (`set`) — the canary for the original defect, which no
	// struct will ever reintroduce on its own.
	vocabulary := map[string]bool{"set": true}
	for k := range paramNames(getGraphIn{}) {
		vocabulary[k] = true
	}
	for k := range paramNames(narrowIn{}) {
		vocabulary[k] = true
	}
	for k := range paramNames(findNodeIn{}) {
		vocabulary[k] = true
	}

	backtick := func(s string) []string {
		var out []string
		parts := strings.Split(s, "`")
		for i := 1; i < len(parts); i += 2 {
			tok := parts[i]
			if tok != "" && !strings.ContainsAny(tok, " .,:;()[]{}\"'=") {
				out = append(out, tok)
			}
		}
		return out
	}

	checkTool := func(tool string, allowed map[string]bool, texts map[string]string) {
		for level, text := range texts {
			for _, tok := range backtick(text) {
				if !vocabulary[tok] {
					continue // not a known parameter-shaped word at all (a format name, a relation name, ...)
				}
				if !allowed[tok] {
					t.Errorf("%s/%s: names parameter `%s`, which is not in its input struct", tool, level, tok)
				}
			}
		}
	}

	checkTool("get_graph", paramNames(getGraphIn{}), map[string]string{
		"brief": graphToolDescriptions["get_graph"].Brief, "standard": graphToolDescriptions["get_graph"].Standard, "full": graphToolDescriptions["get_graph"].Full,
	})
	// find_node's own text points a caller at `around` (get_graph) and
	// `name` (the narrow tools) as where its result feeds back into — real
	// parameters of those tools, not of find_node itself.
	findNodeAllowed := paramNames(findNodeIn{})
	findNodeAllowed["around"] = true
	findNodeAllowed["name"] = true
	checkTool("find_node", findNodeAllowed, map[string]string{
		"brief": graphToolDescriptions["find_node"].Brief, "standard": graphToolDescriptions["find_node"].Standard, "full": graphToolDescriptions["find_node"].Full,
	})
	// Since PLAN_20260928-7 step 4 (narrow set 3x cheaper), a narrow tool's
	// own description names only its own parameters (name, depth) — the
	// shared facts-line/traps prose that used to explain get_graph's own
	// parameters (around, follow, limit, fanout, lift) moved out to the
	// server's instructions (serverInstructions), so the old allowance for
	// naming get_graph's parameters is gone: a narrow tool's vocabulary is
	// strictly its own struct.
	narrowAllowed := paramNames(narrowIn{})
	for name, nt := range narrowTools {
		d := narrowDescriptions(name, nt)
		checkTool(name, narrowAllowed, map[string]string{"brief": d.Brief, "standard": d.Standard, "full": d.Full})
	}
}

// TestServerInstructionsNoForbiddenReferences: the server Instructions text
// (serverInstructions), at every tools×description combination, names no
// removed parameter's marker (`set=`), no ADR, and no repository
// path/markdown file — an agent reading it works in another repository and
// cannot open ours (step 4 rules, extended to Instructions since the
// line-reading help and the traps moved there).
func TestServerInstructionsNoForbiddenReferences(t *testing.T) {
	forbidden := []string{"set=", "ADR", "docs/", ".md"}
	for _, toolsSet := range []string{"one", "narrow"} {
		for _, level := range []string{"brief", "standard", "full"} {
			text := serverInstructions(toolsSet, level)
			for _, f := range forbidden {
				if strings.Contains(text, f) {
					t.Errorf("%s/%s: instructions contain forbidden %q:\n%s", toolsSet, level, f, text)
				}
			}
		}
	}
}

// TestMCPComboBudgets: the six tools×description combinations, sized the
// way /api/mcp sizes them (name+description+schema of the graph tools, plus
// the server Instructions text) must not exceed a budget on the `narrow`
// set (PLAN_20260928-7 step 4: it exists for small models, so its whole
// point is a small per-session cost). `one` carries no budget here.
func TestMCPComboBudgets(t *testing.T) {
	budgets := map[string]int{"brief": 3000, "standard": 8000, "full": 12000}
	for _, c := range mcpCombos() {
		if c.Tools != "narrow" {
			continue
		}
		if want, ok := budgets[c.Description]; ok && c.Bytes > want {
			t.Errorf("narrow/%s: %d bytes, budget %d", c.Description, c.Bytes, want)
		}
	}
}

// narrowFixtureSession: one project with a present `implements` relation
// (e_a implements e_b) and no source facts, so core.BuildGraph builds the
// graph straight from the registry — enough to exercise a walk.
func narrowFixtureSession(t *testing.T, tools string) *mcp.ClientSession {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, "projects", "p")
	files := map[string]string{
		"project.json":   `{"id":"p","contractVersion":5}`,
		"entities.json":  `{"entities":[{"id":"e_a","name":"A","kind":"class","origin":"code","symbol":"N.A"},{"id":"e_b","name":"B","kind":"interface","origin":"code","symbol":"N.B"}]}`,
		"relations.json": `{"relations":[{"id":"r_a_b_implements","from":"e_a","to":"e_b","type":"implements","origin":"code","status":"present"}]}`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &mcpServer{workspace: ws, sourceRoot: ws}
	s.proj.Mcp = mcpSettings{Tools: tools}.withDefaults()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// TestNarrowToolMatchesGetGraph: what_it_extends(name=A) gives the same
// text as get_graph(around=A, follow=[extends,implements]) — the narrow
// tool is a thin wrapper over the very same walk (step 3).
func TestNarrowToolMatchesGetGraph(t *testing.T) {
	one := narrowFixtureSession(t, "one")
	_, wantText := call(t, one, "get_graph", map[string]any{"around": "A", "follow": []string{"extends", "implements"}})

	narrow := narrowFixtureSession(t, "narrow")
	_, gotText := call(t, narrow, "what_it_extends", map[string]any{"name": "A"})

	if gotText != wantText {
		t.Fatalf("what_it_extends does not match get_graph.\ngot:  %s\nwant: %s", gotText, wantText)
	}
}

// TestMcpSettingsChangeAffectsNewSession: a change of mcp.tools (delivered
// through the settings box, as PUT /api/setup does) changes what a NEW
// session's tool list holds, via the running server's own srv (step 4:
// RemoveTools/AddTool on the live *mcp.Server, notifying listChanged).
func TestMcpSettingsChangeAffectsNewSession(t *testing.T) {
	box := newMcpSettingsBox(mcpSettings{Tools: "one"})
	s := &mcpServer{settings: box}
	srv := s.server()
	box.OnChange(s.rebuildGraphTools(srv))

	listNames := func() map[string]bool {
		ctx := context.Background()
		st, ct := mcp.NewInMemoryTransports()
		if _, err := srv.Connect(ctx, st, nil); err != nil {
			t.Fatal(err)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		res, err := cs.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		have := map[string]bool{}
		for _, tl := range res.Tools {
			have[tl.Name] = true
		}
		return have
	}

	before := listNames()
	if !before["get_graph"] {
		t.Fatalf("expected get_graph before the change, got %+v", before)
	}
	if before["who_extends"] {
		t.Fatalf("did not expect who_extends before the change, got %+v", before)
	}

	box.Set(mcpSettings{Tools: "narrow"})

	after := listNames()
	if after["get_graph"] {
		t.Fatalf("expected get_graph gone after switching to narrow, got %+v", after)
	}
	if !after["who_extends"] {
		t.Fatalf("expected who_extends after switching to narrow, got %+v", after)
	}
}

// TestApiMcpSixCombos: GET /api/mcp reports all six tools×description
// combinations, each with its graph tools and a non-zero size (step 5).
func TestApiMcpSixCombos(t *testing.T) {
	file := writeProject(t, "name: Demo\n")
	models, err := newModelService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	box := newMcpSettingsBox(mcpSettings{})
	api := &toolAPI{file: file, workspace: models.workspace, models: models, settings: box}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/mcp", api.guard(api.getMCP))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var st mcpStatus
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if len(st.Combos) != 6 {
		t.Fatalf("expected 6 combinations, got %d", len(st.Combos))
	}
	seen := map[string]bool{}
	for _, c := range st.Combos {
		seen[c.Tools+"/"+c.Description] = true
		if len(c.GraphTools) == 0 {
			t.Errorf("%s/%s: no graph tools", c.Tools, c.Description)
		}
		if c.Bytes <= 0 || c.EstimateTokens <= 0 {
			t.Errorf("%s/%s: expected a positive size, got bytes=%d tokens=%d", c.Tools, c.Description, c.Bytes, c.EstimateTokens)
		}
	}
	for _, tools := range []string{"one", "narrow"} {
		for _, desc := range []string{"brief", "standard", "full"} {
			if !seen[tools+"/"+desc] {
				t.Errorf("missing combination %s/%s", tools, desc)
			}
		}
	}
}
