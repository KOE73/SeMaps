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
