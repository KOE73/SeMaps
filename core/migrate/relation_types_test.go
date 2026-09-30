package migrate

import (
	"reflect"
	"strings"
	"testing"
)

// Relation types are the dictionary's (ADR_20260930-6): relation-types.json and
// the rt_ texts are removed; what the dictionary does not already say goes into
// the workspace kinds.json, once for the workspace.

func provText(v string) string {
	return `{"v":"` + v + `","at":"2026-09-26T09:39:53Z","origin":"authored"}`
}

// v5Project is a project of the current shape with a relation-types.json and
// the entries of its Russian text catalogue.
func v5Project(id, types, entries string) map[string]string {
	p := "projects/" + id + "/"
	return map[string]string{
		p + "project.json":        `{"id":"` + id + `","title":"` + id + `","contractVersion":5,"languages":["ru"]}`,
		p + "entities.json":       `{"contractVersion":5,"entities":[]}`,
		p + "relations.json":      `{"contractVersion":5,"relations":[]}`,
		p + "relation-types.json": types,
		p + "text.ru.json":        `{"contractVersion":5,"language":"ru","entries":{` + entries + `}}`,
	}
}

func merged(parts ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range parts {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// What the dictionary says, and the project says the same: nothing is written to
// kinds.json; the file and the texts are gone.
func TestRelationTypesLikeTheDictionaryNeedNothing(t *testing.T) {
	ws := mkws(t, v5Project("p",
		`{"contractVersion":5,"relationTypes":[{"id":"extends","origin":"code","visibility":"visible"},{"id":"uses","origin":"code","visibility":"hidden"},{"id":"call","origin":"authored"}]}`,
		`"rt_extends":{"name":`+provText("наследует")+`},"rt_call":{"description":`+provText("Одна часть вызывает другую и ждёт ответа.")+`}`))
	opt := Options{DefaultKinds: defaultKinds()}
	rep := run(t, ws, opt)
	if exists(ws, "kinds.json") {
		t.Errorf("the dictionary already says it: no kinds.json, got %s", snapshot(t, ws)["kinds.json"])
	}
	if exists(ws, "projects/p/relation-types.json") || !contains(rep.Projects[0].Removed, "projects/p/relation-types.json") {
		t.Error("relation-types.json must be removed and reported")
	}
	if got := js(t, readTree(t, ws, "projects/p/text.ru.json"), "entries"); got != `{}` {
		t.Errorf("rt_ texts must go: %s", got)
	}
	if rep.Projects[0].RelationTypeTexts != 2 || !strings.Contains(printed(rep), "текстов rt_ убрано") {
		t.Errorf("report: %d\n%s", rep.Projects[0].RelationTypeTexts, printed(rep))
	}
	if run(t, ws, opt).Changed() {
		t.Error("the second run changes something")
	}
}

// A visibility or a text the dictionary does not have is carried over: a copy of
// the dictionary's entry replaces it whole, a type outside the dictionary gets a
// new one; the group is one, the report lists every entry.
func TestRelationTypesDifferingFromTheDictionaryGoToKinds(t *testing.T) {
	ws := mkws(t, v5Project("p",
		`{"contractVersion":5,"relationTypes":[`+
			`{"id":"extends","origin":"code","visibility":"hidden"},`+
			`{"id":"holds.many","origin":"code","visibility":"visible"},`+
			`{"id":"flows","origin":"authored","visibility":"hidden"},`+
			`{"id":"bare","origin":"authored"}]}`,
		`"rt_flows":{"name":`+provText("поток")+`,"description":`+provText("Как текут данные.")+`},`+
			`"rt_only":{"name":`+provText("только имя")+`},`+
			`"rt_call":{"name":`+provText("вызов")+`}`))
	opt := Options{DefaultKinds: defaultKinds()}
	rep := run(t, ws, opt)
	kinds := readTree(t, ws, "kinds.json")
	if got := strings.Join(keysOf(t, kinds), ","); got != "contractVersion,relationGroups" || arrLen(t, kinds, "relationGroups") != 1 {
		t.Fatalf("kinds.json: %s", js(t, kinds))
	}
	if str(t, kinds, "relationGroups", 0, "id") != "relations.project" {
		t.Errorf("group: %s", js(t, kinds, "relationGroups", 0))
	}
	var ids []string
	for _, tv := range at(t, kinds, "relationGroups", 0, "types").([]any) {
		ids = append(ids, str(t, tv, "id"))
	}
	// holds.many is the dictionary's own; the rest is in the order the project claimed it, texts last
	if want := []string{"extends", "flows", "bare", "only", "call"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("types: %v, want %v", ids, want)
	}
	if got := str(t, kinds, "relationGroups", 0, "types", 0, "visibility"); got != "hidden" ||
		str(t, kinds, "relationGroups", 0, "types", 0, "name", "en") != "extends" {
		t.Errorf("extends: a copy of the dictionary entry with the visibility of the project: %s", js(t, kinds, "relationGroups", 0, "types", 0))
	}
	if got := js(t, kinds, "relationGroups", 0, "types", 1); got != `{"id":"flows","visibility":"hidden","name":{"ru":"поток"},"description":{"ru":"Как текут данные."}}` {
		t.Errorf("flows: %s", got)
	}
	if got := js(t, kinds, "relationGroups", 0, "types", 2); got != `{"id":"bare","name":{"ru":"bare"}}` {
		t.Errorf("bare: nobody named it, its id is the name: %s", got)
	}
	if got := js(t, kinds, "relationGroups", 0, "types", 4, "name"); got != `{"ru":"вызов","en":"calls"}` {
		t.Errorf("call: only the language the project wrote is replaced: %s", got)
	}
	if got := js(t, kinds, "relationGroups", 0, "types", 3); got != `{"id":"only","name":{"ru":"только имя"}}` {
		t.Errorf("only: %s", got)
	}
	out := printed(rep)
	for _, frag := range []string{
		"тип связи «extends» → kinds.json, группа relations.project: запись словаря по умолчанию заменена целиком; visibility visible → hidden",
		"тип связи «flows» → kinds.json, группа relations.project: типа нет в словаре; visibility не задана → hidden; name.ru «поток»; description.ru «Как текут данные.»",
		"тип связи «bare» → kinds.json, группа relations.project: типа нет в словаре; name.ru «bare» (имени не было, взят id)",
		"kinds.json",
	} {
		if !strings.Contains(out, frag) {
			t.Errorf("report lacks %q:\n%s", frag, out)
		}
	}
	if strings.Contains(out, "holds.many") {
		t.Errorf("holds.many is the dictionary's own and is not reported:\n%s", out)
	}
	if again := run(t, ws, opt); again.Changed() {
		t.Errorf("the second run changes something: %s", printed(again))
	}
}

// A type the workspace kinds.json already has is changed where it is: no second
// entry in another group (an id is unique in the dictionary), the other sections stay.
func TestRelationTypesChangeTheWorkspaceEntryInPlace(t *testing.T) {
	ws := mkws(t, merged(v5Project("p",
		`{"contractVersion":5,"relationTypes":[{"id":"flows","origin":"authored","visibility":"visible"},{"id":"quiet","origin":"authored"}]}`,
		`"rt_flows":{"description":`+provText("Как текут данные.")+`}`),
		map[string]string{
			"kinds.json": `{"contractVersion":5,"groups":[{"id":"g","name":{"ru":"Г"},"kinds":[]}],"relationGroups":[` +
				`{"id":"mine","name":{"ru":"Мои"},"types":[` +
				`{"id":"flows","name":{"ru":"поток"}},` +
				`{"id":"quiet","visibility":"hidden","name":{"ru":"тихая"}}]}]}`,
		}))
	run(t, ws, Options{DefaultKinds: defaultKinds()})
	kinds := readTree(t, ws, "kinds.json")
	if arrLen(t, kinds, "relationGroups") != 1 || arrLen(t, kinds, "groups") != 1 {
		t.Fatalf("no new group, other sections stay: %s", js(t, kinds))
	}
	if got := js(t, kinds, "relationGroups", 0, "types", 0); got != `{"id":"flows","visibility":"visible","name":{"ru":"поток"},"description":{"ru":"Как текут данные."}}` {
		t.Errorf("flows: %s", got)
	}
	// the project recorded no visibility for `quiet`: the view decides, and the entry no longer says hidden
	if got := js(t, kinds, "relationGroups", 0, "types", 1); got != `{"id":"quiet","name":{"ru":"тихая"}}` {
		t.Errorf("quiet: %s", got)
	}
}

// The default visibility is one per workspace: the first project that recorded
// one wins, the report names the projects that disagree.
func TestRelationTypesOneVisibilityPerWorkspace(t *testing.T) {
	ws := mkws(t, merged(
		v5Project("p", `{"relationTypes":[{"id":"extends","visibility":"hidden"},{"id":"zzz","visibility":"visible"}]}`, ``),
		v5Project("q", `{"relationTypes":[{"id":"extends","visibility":"visible"},{"id":"zzz","visibility":"hidden"}]}`, `"rt_zzz":{"name":`+provText("ззз")+`}`)))
	rep := run(t, ws, Options{DefaultKinds: defaultKinds()})
	kinds := readTree(t, ws, "kinds.json")
	if got := str(t, kinds, "relationGroups", 0, "types", 0, "visibility"); got != "hidden" {
		t.Errorf("extends: the first project wins: %s", js(t, kinds, "relationGroups", 0, "types", 0))
	}
	if got := str(t, kinds, "relationGroups", 0, "types", 1, "visibility"); got != "visible" {
		t.Errorf("zzz: %s", js(t, kinds, "relationGroups", 0, "types", 1))
	}
	if !anyContains(rep.WorkspaceNotes, "тип связи «extends»: проект p записывал visibility hidden, проект q — visible; в рабочем пространстве она одна, оставлена hidden (проекта p)") ||
		!anyContains(rep.WorkspaceNotes, "тип связи «zzz»: проект p записывал visibility visible, проект q — hidden") {
		t.Errorf("notes: %v", rep.WorkspaceNotes)
	}
}

// A field an rt_ text has that the dictionary does not is not carried over, and
// the report says so; an empty relation-types.json is removed like any other.
func TestRelationTypesFieldsAndEmptyFile(t *testing.T) {
	ws := mkws(t, v5Project("p", `{"contractVersion":5,"relationTypes":[]}`,
		`"rt_x":{"name":`+provText("икс")+`,"fromLabel":`+provText("1")+`}`))
	rep := run(t, ws, Options{DefaultKinds: defaultKinds()})
	if !anyContains(rep.Projects[0].Notes, "text.ru.json: rt_x.fromLabel не переносится") {
		t.Errorf("notes: %v", rep.Projects[0].Notes)
	}
	if str(t, readTree(t, ws, "kinds.json"), "relationGroups", 0, "types", 0, "name", "ru") != "икс" {
		t.Error("the name of a type only a text names is written")
	}

	ws = mkws(t, v5Project("p", `{"contractVersion":5,"relationTypes":[]}`, ``))
	rep = run(t, ws, Options{DefaultKinds: defaultKinds()})
	if exists(ws, "kinds.json") || exists(ws, "projects/p/relation-types.json") || !rep.Changed() {
		t.Errorf("an empty file goes and nothing is written: %v", snapshot(t, ws))
	}
}

// Without a dictionary of the tool every type is outside it: nothing is claimed
// to be the dictionary's own.
func TestRelationTypesWithoutADictionary(t *testing.T) {
	ws := mkws(t, v5Project("p", `{"relationTypes":[{"id":"extends","visibility":"visible"}]}`, ``))
	run(t, ws, Options{})
	if got := js(t, readTree(t, ws, "kinds.json"), "relationGroups", 0, "types", 0); got != `{"id":"extends","visibility":"visible","name":{"ru":"extends"}}` {
		t.Errorf("extends: %s", got)
	}
}
