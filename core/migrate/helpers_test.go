package migrate

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// defaultKinds is the tool's shipped dictionary, the one the relation types of a
// project are compared with.
func defaultKinds() []byte {
	b, err := os.ReadFile(filepath.Join("..", "..", "host", "defaults", "kinds.json"))
	if err != nil {
		panic(err)
	}
	return b
}

// fixedNow pins the timestamps the migration writes.
func fixedNow(t *testing.T) {
	t.Helper()
	old := timeNow
	timeNow = func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { timeNow = old })
}

// copyFixture copies testdata/v3/<name>/workspace into a temp dir; fixtures are
// never modified.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", "v3", name, "workspace")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// mkws writes a workspace from a map of slash-separated paths.
func mkws(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, c := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// baseProject is a minimal v3 project "p" with one entity; extra files replace
// or add to it.
func baseProject(extra map[string]string) map[string]string {
	files := map[string]string{
		"projects/p/project.json":        `{"id":"p","title":"P","contractVersion":3,"languages":["ru"]}`,
		"projects/p/entities.json":       `{"entities":[{"id":"e_a","name":"A","kind":"class"}]}`,
		"projects/p/relations.json":      `{"relations":[]}`,
		"projects/p/relation-types.json": `{"relationTypes":[]}`,
		"projects/p/text.ru.json":        `{"contractVersion":3,"language":"ru","entries":{}}`,
	}
	for k, v := range extra {
		if v == "" {
			delete(files, k)
			continue
		}
		files[k] = v
	}
	return files
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func run(t *testing.T, ws string, opt Options) *Report {
	t.Helper()
	rep, err := Workspace(ws, opt)
	if err != nil {
		t.Fatalf("Workspace: %v", err)
	}
	return rep
}

func readTree(t *testing.T, ws, rel string) *obj {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ws, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	v, err := parseTree(b)
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	o, ok := v.(*obj)
	if !ok {
		t.Fatalf("%s: not an object", rel)
	}
	return o
}

func exists(ws, rel string) bool {
	_, err := os.Stat(filepath.Join(ws, filepath.FromSlash(rel)))
	return err == nil
}

// at walks an obj/array tree by keys and indexes.
func at(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, p := range path {
		switch k := p.(type) {
		case string:
			o, ok := v.(*obj)
			if !ok {
				t.Fatalf("path %v: %q applied to %T", path, k, v)
			}
			nv, ok := o.lookup(k)
			if !ok {
				t.Fatalf("path %v: no key %q (keys %v)", path, k, o.keys)
			}
			v = nv
		case int:
			a, ok := v.([]any)
			if !ok || k >= len(a) {
				t.Fatalf("path %v: index %d applied to %T", path, k, v)
			}
			v = a[k]
		}
	}
	return v
}

func has(v any, key string) bool {
	o, ok := v.(*obj)
	return ok && o.has(key)
}

func str(t *testing.T, v any, path ...any) string {
	t.Helper()
	s, ok := asStr(at(t, v, path...))
	if !ok {
		t.Fatalf("path %v: not a string", path)
	}
	return s
}

// js is the compact JSON of a subtree.
func js(t *testing.T, v any, path ...any) string {
	t.Helper()
	return compactJSON(at(t, v, path...))
}

func keysOf(t *testing.T, v any, path ...any) []string {
	t.Helper()
	o, ok := at(t, v, path...).(*obj)
	if !ok {
		t.Fatalf("path %v: not an object", path)
	}
	return o.keys
}

func arrLen(t *testing.T, v any, path ...any) int {
	t.Helper()
	a, ok := at(t, v, path...).([]any)
	if !ok {
		t.Fatalf("path %v: not an array", path)
	}
	return len(a)
}

// entityIDs lists the ids in entities.json of project p.
func entityIDs(t *testing.T, ws, proj string) []string {
	t.Helper()
	root := readTree(t, ws, "projects/"+proj+"/entities.json")
	var ids []string
	for _, e := range at(t, root, "entities").([]any) {
		ids = append(ids, str(t, e, "id"))
	}
	return ids
}

func entityByID(t *testing.T, ws, proj, id string) *obj {
	t.Helper()
	root := readTree(t, ws, "projects/"+proj+"/entities.json")
	for _, e := range at(t, root, "entities").([]any) {
		o := e.(*obj)
		if s, _ := asStr(o.get("id")); s == id {
			return o
		}
	}
	t.Fatalf("entity %s not found", id)
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func anyContains(list []string, sub string) bool {
	for _, x := range list {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

func printed(r *Report) string {
	var b bytes.Buffer
	r.Print(&b)
	return b.String()
}

func mustJSONValid(t *testing.T, s string) {
	t.Helper()
	if !json.Valid([]byte(s)) {
		t.Fatalf("invalid JSON: %s", s)
	}
}
