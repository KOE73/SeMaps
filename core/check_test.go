package core

import (
	"path/filepath"
	"strings"
	"testing"
)

// A zone is named under its own id or, failing that, under the container
// `c_<x>` a zone `z_<x>` renders — the order the editor reads them in. A
// decorative frame (no container) is named like any other zone.
func TestCheckZoneNames(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "projects", "p")
	writeFile(t, filepath.Join(proj, "project.json"),
		`{"id":"p","title":"P","contractVersion":3,"defaultAxis":"axis_a","languages":["ru"]}`)
	writeFile(t, filepath.Join(proj, "entities.json"), `{"contractVersion":3,"entities":[]}`)
	writeFile(t, filepath.Join(proj, "relations.json"), `{"contractVersion":3,"relations":[]}`)
	writeFile(t, filepath.Join(proj, "relation-types.json"), `{"contractVersion":3,"relationTypes":[]}`)
	writeFile(t, filepath.Join(proj, "views", "v_main.view.json"), `{
		"id":"v_main","project":"p","axis":"axis_a",
		"zones":[
			{"id":"z_own","container":null,"x":0,"y":0,"width":200,"height":100},
			{"id":"z_bycontainer","container":"c_bycontainer","x":0,"y":200,"width":200,"height":100},
			{"id":"z_nameless","container":null,"x":0,"y":400,"width":200,"height":100}
		],
		"nodes":[]}`)
	name := `{"v":"%s","at":"2026-09-24T00:00:00Z","origin":"authored"}`
	writeFile(t, filepath.Join(proj, "text.ru.json"), `{"contractVersion":3,"language":"ru","entries":{
		"v_main":{"name":`+strings.Replace(name, "%s", "Главный", 1)+`},
		"z_own":{"name":`+strings.Replace(name, "%s", "Своя подпись", 1)+`},
		"c_bycontainer":{"name":`+strings.Replace(name, "%s", "Подпись контейнера", 1)+`}
	}}`)

	var missing []string
	for _, f := range Check(ws, ws) {
		if f.Kind == "недостача" {
			missing = append(missing, f.Message)
		}
	}
	if len(missing) != 1 || !strings.HasPrefix(missing[0], "z_nameless:") {
		t.Fatalf("want only z_nameless reported as unnamed, got %q", missing)
	}
}
