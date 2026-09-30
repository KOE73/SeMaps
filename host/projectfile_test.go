package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProject(t *testing.T, text string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "x.semaps")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestLoadProjectFlatFileStillReads(t *testing.T) {
	file := writeProject(t, "# SeMaps project file.\nversion: 1\nname: \"Project #3\"\nworkspace: workspace        # default: docs/diagrams\nsource_root: ..\nport: 8777\n")
	p, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(file)
	if p.Name != "Project #3" || p.Port != 8777 || p.Workspace != filepath.Join(root, "workspace") || p.SourceRoot != filepath.Dir(root) {
		t.Errorf("%+v", p)
	}
}

func TestLoadProjectEmptyFile(t *testing.T) {
	p, err := loadProject(writeProject(t, "# nothing yet\n"))
	if err != nil || !strings.HasSuffix(filepath.ToSlash(p.Workspace), "docs/diagrams") {
		t.Errorf("%v %+v", err, p)
	}
}

func TestLoadProjectRejects(t *testing.T) {
	for _, text := range []string{
		"colour: red\n",
		"extractors:\n  - id: a\n    language: csharp\n",
		"extractors:\n  - {id: a, language: csharp, project: p}\n  - {id: a, language: typescript, project: p}\n",
		"extractors:\n  - {id: a, language: csharp, project: p, root: C:/src}\n",
		"extractors:\n  - {id: a, language: csharp, project: p, edges: [holds, invalid]}\n",
	} {
		if _, err := loadProject(writeProject(t, text)); err == nil {
			t.Errorf("accepted: %q", text)
		}
	}
}

func TestPatchExtractorKeepsComments(t *testing.T) {
	file := writeProject(t, `# SeMaps project file.
name: Demo   # shown in the console
extractors:
  # the backend
  - id: backend
    language: csharp
    project: core
    include: [src]   # only src
    command: dotnet run --project ../x --
`)
	include := []string{"src", "tools"}
	if err := patchExtractor(file, "backend", extractorPatch{Include: &include}); err != nil {
		t.Fatal(err)
	}
	lang, proj := "typescript", "editor"
	if err := patchExtractor(file, "editor", extractorPatch{Language: &lang, Project: &proj}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	text := string(data)
	for _, want := range []string{"# SeMaps project file.", "# shown in the console", "# the backend", "include: [src, tools] # only src", "command: dotnet run --project ../x --", "id: editor"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	p, err := loadProject(file)
	if err != nil || len(p.Extractors) != 2 || p.Extractors[1].Language != "typescript" || p.Extractors[0].Command == "" {
		t.Errorf("%v %+v", err, p.Extractors)
	}

	// A patch that breaks the file is refused and leaves it as it was.
	empty := ""
	if err := patchExtractor(file, "editor", extractorPatch{Project: &empty}); err == nil {
		t.Error("accepted an entry without project")
	}
	if after, _ := os.ReadFile(file); string(after) != text {
		t.Error("file changed by a refused patch")
	}
}

func TestLoadProjectWithEdges(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: backend
    language: csharp
    project: core
    edges: [holds, injects]
`)
	p, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Extractors) != 1 || len(p.Extractors[0].Edges) != 2 {
		t.Errorf("%+v", p.Extractors)
	}
	if p.Extractors[0].Edges[0] != "holds" || p.Extractors[0].Edges[1] != "injects" {
		t.Errorf("edges mismatch: %v", p.Extractors[0].Edges)
	}
}

// `implements:` lists the external interfaces a Go entry reports (ADR_20260927
// §5); the host passes them as one comma-separated --implements, and Go is a
// shipped language.
func TestLoadProjectWithImplements(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: core
    language: go
    project: core
    edges: [holds]
    implements: [io.Writer, error]
`)
	p, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	e := p.Extractors[0]
	if len(e.Implements) != 2 || e.Implements[0] != "io.Writer" || e.Implements[1] != "error" {
		t.Fatalf("implements: %v", e.Implements)
	}
	args := strings.Join(extractorArgs(extractorTool{argv: []string{"semaps-extract-go"}}, e, "/p"), " ")
	if !strings.Contains(args, "--edges holds") || !strings.HasSuffix(args, "--implements io.Writer,error") {
		t.Errorf("args: %s", args)
	}
	known := false
	for _, l := range knownLanguages {
		known = known || l == "go"
	}
	if !known {
		t.Errorf("go is not among the shipped languages: %v", knownLanguages)
	}
}

func TestPatchExtractorWithEdges(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: backend
    language: csharp
    project: core
`)
	edges := []string{"holds", "uses"}
	if err := patchExtractor(file, "backend", extractorPatch{Edges: &edges}); err != nil {
		t.Fatal(err)
	}
	p, err := loadProject(file)
	if err != nil || len(p.Extractors[0].Edges) != 2 {
		t.Errorf("%v %+v", err, p.Extractors)
	}
}

func TestEdgesRoundTrip(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: api
    language: csharp
    project: core
    edges: [holds, injects]
`)
	data1, _ := os.ReadFile(file)
	p, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Extractors[0].Edges) != 2 {
		t.Errorf("lost edges on load: %v", p.Extractors[0].Edges)
	}
	// Patch with same edges
	edges := p.Extractors[0].Edges
	if err := patchExtractor(file, "api", extractorPatch{Edges: &edges}); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(file)
	if string(data1) != string(data2) {
		t.Errorf("content changed by patch with same edges:\n%s\nvs\n%s", string(data1), string(data2))
	}
}

func TestLoadProjectWithWatch(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: backend
    language: csharp
    project: core
    watch: true
  - id: frontend
    language: typescript
    project: core
`)
	p, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Extractors[0].Watch {
		t.Errorf("expected watch: true on backend, got %+v", p.Extractors[0])
	}
	if p.Extractors[1].Watch {
		t.Errorf("expected watch to default to false, got %+v", p.Extractors[1])
	}
}

func TestLoadProjectRejectsNonBooleanWatch(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: backend
    language: csharp
    project: core
    watch: yesplease
`)
	if _, err := loadProject(file); err == nil {
		t.Error("accepted a non-boolean watch")
	}
}

func TestPatchExtractorSetsWatch(t *testing.T) {
	file := writeProject(t, `extractors:
  - id: backend
    language: csharp
    project: core
`)
	watch := true
	if err := patchExtractor(file, "backend", extractorPatch{Watch: &watch}); err != nil {
		t.Fatal(err)
	}
	p, err := loadProject(file)
	if err != nil || !p.Extractors[0].Watch {
		t.Errorf("%v %+v", err, p.Extractors)
	}
}

func TestLoadProjectMcpDefaults(t *testing.T) {
	p, err := loadProject(writeProject(t, "# nothing yet\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := mcpSettings{Tools: "one", Description: "standard", Format: "facts", ListCap: 50, Limit: 200}
	if p.Mcp != want {
		t.Errorf("got %+v, want %+v", p.Mcp, want)
	}
}

func TestLoadProjectMcpValues(t *testing.T) {
	p, err := loadProject(writeProject(t, "mcp:\n  tools: narrow\n  description: full\n  format: lines\n  list_cap: 10\n  limit: 20\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := mcpSettings{Tools: "narrow", Description: "full", Format: "lines", ListCap: 10, Limit: 20}
	if p.Mcp != want {
		t.Errorf("got %+v, want %+v", p.Mcp, want)
	}
}

func TestLoadProjectRejectsBadMcp(t *testing.T) {
	for _, text := range []string{
		"mcp:\n  tools: wide\n",
		"mcp:\n  description: verbose\n",
		"mcp:\n  format: no-such-format\n",
		"mcp:\n  list_cap: -1\n",
		"mcp:\n  limit: -1\n",
		"mcp:\n  bogus: 1\n",
	} {
		if _, err := loadProject(writeProject(t, text)); err == nil {
			t.Errorf("accepted: %q", text)
		}
	}
}

func TestPatchMcpKeepsCommentsAndKeyOrder(t *testing.T) {
	file := writeProject(t, `# SeMaps project file.
name: Demo   # shown in the console
mcp:
  tools: one   # default
  format: lines
`)
	tools, listCap := "narrow", 25
	if err := patchMcp(file, mcpPatch{Tools: &tools, ListCap: &listCap}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "tools: narrow # default") {
		t.Errorf("expected the comment to survive on the changed key, got:\n%s", text)
	}
	if !strings.Contains(text, "format: lines") {
		t.Errorf("expected the untouched key to survive, got:\n%s", text)
	}
	p, err := loadProject(file)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mcp.Tools != "narrow" || p.Mcp.Format != "lines" || p.Mcp.ListCap != 25 {
		t.Errorf("%+v", p.Mcp)
	}
}

func TestPatchMcpRejectsBadValue(t *testing.T) {
	file := writeProject(t, "name: Demo\n")
	bad := "wide"
	if err := patchMcp(file, mcpPatch{Tools: &bad}); err == nil {
		t.Error("expected patchMcp to refuse an invalid mcp.tools")
	}
	// Left as it was: the file still loads with the tools default.
	p, err := loadProject(file)
	if err != nil || p.Mcp.Tools != "one" {
		t.Errorf("%v %+v", err, p.Mcp)
	}
}

func TestFindProjectFileSkipsTheSemapsFolder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".semaps", "logs"), 0o755)
	os.WriteFile(filepath.Join(dir, "x.semaps"), []byte("version: 1\n"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	f, ok, err := findProjectFile(sub)
	if err != nil || !ok || filepath.Base(f) != "x.semaps" {
		t.Fatalf("%q %v %v", f, ok, err)
	}
}
