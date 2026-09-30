package migrate

import (
	"reflect"
	"strings"
	"testing"
)

// The final shape is reached from a project of contract 5 of the earlier form
// too, decided by the content (ADR_20260930-4, ADR_20260930-5).

const prov0 = `{"v":"%s","at":"2026-09-01T10:00:00Z","origin":"authored"}`

func prov(v string) string { return strings.Replace(prov0, "%s", v, 1) }

// v5Old is a project of contract 5 in the earlier form: codeRef and symbol on the
// entities, a via on the relation, codeRef in the evidence, the name of authored
// entities in entities.json.
func v5Old(extra map[string]string) map[string]string {
	files := map[string]string{
		"projects/p/project.json": `{"id":"p","title":"P","contractVersion":5,"languages":["ru","en"]}`,
		"projects/p/entities.json": `{"entities":[
			{"id":"e_a","name":"A","kind":"class","origin":"code","status":"present","namespace":"N","codeRef":"src/A.cs","symbol":"N.A","members":[]},
			{"id":"e_b","name":"B","kind":"class","origin":"code","status":"present","codeRef":"src/B.cs","symbol":"N.B"},
			{"id":"e_io","name":"Writer","kind":"external","origin":"code","symbol":"io.Writer"},
			{"id":"e_hand","name":"Doc","kind":"database","origin":"authored","codeRef":"docs/schema.sql"},
			{"id":"e_grp","name":"Группа","kind":"group","origin":"authored","status":"present"},
			{"id":"e_plain","name":"Plain","kind":"class"}]}`,
		"projects/p/relations.json": `{"contractVersion":5,"relations":[
			{"id":"r_a_b_items","from":"e_a","to":"e_b","type":"holds.many","origin":"code","status":"present",
			 "via":{"member":"items","cardinality":"many"},
			 "evidence":[{"codeRef":"src/A.cs","symbol":"N.A","line":12}]},
			{"id":"r_a_io_impl","from":"e_a","to":"e_io","type":"implements","origin":"code",
			 "evidence":[{"symbol":"N.A"}]},
			{"id":"r_only_via","from":"e_b","to":"e_a","type":"uses","via":{"member":"x"}},
			{"id":"r_plain","from":"e_a","to":"e_b","type":"depends","origin":"code","evidence":[{"codeRef":"src/A.csproj"}]},
			{"id":"r_new","from":"e_a","to":"e_b","type":"depends","origin":"authored"}]}`,
		"projects/p/relation-types.json": `{"contractVersion":5,"relationTypes":[]}`,
		"projects/p/text.ru.json":        `{"contractVersion":5,"language":"ru","entries":{"e_grp":{"description":` + prov("Описание") + `}}}`,
		"projects/p/text.en.json":        `{"contractVersion":5,"language":"en","entries":{}}`,
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

func csharp() Options {
	return Options{Extractors: []Extractor{{Project: "p", Language: "csharp"}}}
}

func TestV5EarlierFormIsBroughtToTheFinalShape(t *testing.T) {
	fixedNow(t)
	ws := mkws(t, v5Old(nil))
	rep := run(t, ws, csharp())
	p := rep.Projects[0]
	if p.Skipped || p.FromVersion != 5 || p.CodeEntries != 4 || p.EvidenceConverted != 4 || p.NamesMoved != 2 {
		t.Fatalf("report: %+v", *p)
	}
	ents := func(id string) *obj { return entityByID(t, ws, "p", id) }
	if got := js(t, ents("e_a"), "code"); got != `[{"lang":"csharp","ref":"src/A.cs","symbol":"N.A"}]` {
		t.Errorf("e_a: %s", got)
	}
	if got := strings.Join(ents("e_a").keys, ","); got != "id,name,kind,origin,status,namespace,code,members" {
		t.Errorf("e_a keys: %s", got)
	}
	// an external symbol: a realization without ref
	if got := js(t, ents("e_io"), "code"); got != `[{"lang":"csharp","symbol":"io.Writer"}]` {
		t.Errorf("e_io: %s", got)
	}
	// a hand-written file link: no language, no symbol
	if got := js(t, ents("e_hand"), "code"); got != `[{"ref":"docs/schema.sql"}]` {
		t.Errorf("e_hand: %s", got)
	}
	if has(ents("e_plain"), "code") || js(t, ents("e_plain"), "name") != `"Plain"` {
		t.Errorf("an entity of no origin keeps its name and gets no code: %s", js(t, ents("e_plain")))
	}
	// the name of an authored entity: a text of the main language, authored, at = now; no name in entities.json
	for _, id := range []string{"e_hand", "e_grp"} {
		if has(ents(id), "name") {
			t.Errorf("%s keeps a name in entities.json", id)
		}
	}
	ru := readTree(t, ws, "projects/p/text.ru.json")
	if got := js(t, ru, "entries", "e_grp"); got != `{"name":{"v":"Группа","at":"2026-09-30T12:00:00Z","origin":"authored"},"description":`+prov("Описание")+`}` {
		t.Errorf("e_grp text: %s", got)
	}
	if got := str(t, ru, "entries", "e_hand", "name", "v"); got != "Doc" {
		t.Errorf("e_hand name: %s", got)
	}
	if has(at(t, readTree(t, ws, "projects/p/text.en.json"), "entries"), "e_grp") {
		t.Error("the other language gets no invented name")
	}

	rels := readTree(t, ws, "projects/p/relations.json")
	r := func(i int) any { return at(t, rels, "relations", i) }
	if got := js(t, r(0), "evidence"); got != `[{"lang":"csharp","ref":"src/A.cs:12","symbol":"N.A","via":{"member":"items","cardinality":"many"}}]` || has(r(0), "via") {
		t.Errorf("r_a_b_items: %s", js(t, r(0)))
	}
	if got := strings.Join(r(0).(*obj).keys, ","); got != "id,from,to,type,origin,status,evidence" {
		t.Errorf("r_a_b_items keys: %s", got)
	}
	if got := js(t, r(1), "evidence"); got != `[{"lang":"csharp","symbol":"N.A"}]` {
		t.Errorf("a symbol without a file: %s", got)
	}
	// a via with no evidence at all: an entry of its own, no language (no symbol)
	if got := js(t, r(2), "evidence"); got != `[{"via":{"member":"x"}}]` || has(r(2), "via") {
		t.Errorf("r_only_via: %s", js(t, r(2)))
	}
	if got := js(t, r(3), "evidence"); got != `[{"ref":"src/A.csproj"}]` {
		t.Errorf("a file link as evidence: %s", got)
	}
	if got := js(t, r(4)); got != `{"id":"r_new","from":"e_a","to":"e_b","type":"depends","origin":"authored"}` {
		t.Errorf("a relation without realizations stays: %s", got)
	}
	checkV5Shape(t, ws)

	// idempotent, and nothing left to do says so
	before := snapshot(t, ws)
	second := run(t, ws, csharp())
	if second.Changed() || printed(second) != "Уже контракт v5, менять нечего.\n" || !second.Projects[0].Skipped {
		t.Fatalf("second run: %q", printed(second))
	}
	if !reflect.DeepEqual(before, snapshot(t, ws)) {
		t.Error("the second run wrote files")
	}
}

func TestV5EarlierFormNeedsALanguageAndAsks(t *testing.T) {
	for name, opt := range map[string]Options{
		"no extractor":    {},
		"another project": {Extractors: []Extractor{{Project: "other", Language: "go"}}},
		"two languages":   {Extractors: []Extractor{{Project: "p", Language: "go"}, {Project: "p", Language: "csharp"}}},
	} {
		ws := mkws(t, v5Old(nil))
		before := snapshot(t, ws)
		_, err := Workspace(ws, opt)
		if err == nil || !strings.Contains(err.Error(), "проект p: язык реализаций не определён") ||
			!strings.Contains(err.Error(), "e_a: symbol N.A") || !strings.Contains(err.Error(), "ничего не записано") {
			t.Errorf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(before, snapshot(t, ws)) {
			t.Errorf("%s: files were written despite the error", name)
		}
	}
	// two extractors of ONE language are not a guess
	ws := mkws(t, v5Old(nil))
	if _, err := Workspace(ws, Options{Extractors: []Extractor{{Project: "p", Language: "csharp"}, {Project: "p", Language: "csharp"}}}); err != nil {
		t.Errorf("one language, two extractors: %v", err)
	}
	// without a symbol nothing needs a language: a hand link and a name are enough
	ws = mkws(t, v5Old(map[string]string{
		"projects/p/entities.json":  `{"entities":[{"id":"e_hand","name":"Doc","kind":"database","origin":"authored","codeRef":"docs/schema.sql"}]}`,
		"projects/p/relations.json": `{"relations":[]}`,
	}))
	if _, err := Workspace(ws, Options{}); err != nil {
		t.Errorf("no symbol, no language needed: %v", err)
	}
	// the evidence of a relation asks for the language the same way
	ws = mkws(t, v5Old(map[string]string{
		"projects/p/entities.json": `{"entities":[]}`,
	}))
	if _, err := Workspace(ws, Options{}); err == nil || !strings.Contains(err.Error(), "r_a_b_items: evidence symbol N.A") {
		t.Errorf("evidence symbol: %v", err)
	}
}

// A name that already has a text is not overwritten: the name of entities.json is
// dropped and named in the report.
func TestAuthoredNameWithATextAlreadyThere(t *testing.T) {
	ws := mkws(t, v5Old(map[string]string{
		"projects/p/entities.json":  `{"entities":[{"id":"e_grp","name":"Старое","kind":"group","origin":"authored"}]}`,
		"projects/p/relations.json": `{"relations":[]}`,
		"projects/p/text.ru.json":   `{"contractVersion":5,"language":"ru","entries":{"e_grp":{"name":` + prov("Уже есть") + `}}}`,
	}))
	rep := run(t, ws, csharp())
	if got := str(t, readTree(t, ws, "projects/p/text.ru.json"), "entries", "e_grp", "name", "v"); got != "Уже есть" {
		t.Errorf("text overwritten: %s", got)
	}
	if !anyContains(rep.Projects[0].Notes, "name «Старое» из entities.json отброшено") {
		t.Errorf("notes: %v", rep.Projects[0].Notes)
	}
	// an empty name is dropped and reported; the entity then has no name text: `check` says so
	ws = mkws(t, v5Old(map[string]string{
		"projects/p/entities.json":  `{"entities":[{"id":"e_g","name":" ","kind":"group","origin":"authored"}]}`,
		"projects/p/relations.json": `{"relations":[]}`,
	}))
	rep = run(t, ws, csharp())
	if has(entityByID(t, ws, "p", "e_g"), "name") || !anyContains(rep.Projects[0].Notes, "e_g: пустое name") {
		t.Errorf("empty name: %v", rep.Projects[0].Notes)
	}
}

// The `name` text of an entity from code that repeats its name word for word is
// dead (the name of such an entity is the code's) and is removed; one that says
// something else stays, for a human (`semaps check`: «лишнее имя»).
func TestNameTextsOfCodeEntitiesThatOnlyRepeatAreDropped(t *testing.T) {
	ws := mkws(t, v5Old(map[string]string{
		"projects/p/entities.json": `{"entities":[
			{"id":"e_same","name":"Same","kind":"class","origin":"code"},
			{"id":"e_alone","name":"Alone","kind":"class","origin":"code"},
			{"id":"e_other","name":"Other","kind":"class","origin":"code"},
			{"id":"e_auth","kind":"group","origin":"authored"}]}`,
		"projects/p/relations.json": `{"relations":[]}`,
		"projects/p/text.ru.json": `{"contractVersion":5,"language":"ru","entries":{
			"e_same":{"name":` + prov("Same") + `,"description":` + prov("d") + `},
			"e_alone":{"name":` + prov("Alone") + `},
			"e_other":{"name":` + prov("Иначе") + `},
			"e_auth":{"name":` + prov("Группа") + `}}}`,
	}))
	rep := run(t, ws, Options{})
	entries := at(t, readTree(t, ws, "projects/p/text.ru.json"), "entries")
	if got := strings.Join(keysOf(t, entries), ","); got != "e_same,e_other,e_auth" {
		t.Errorf("entries: %s", got)
	}
	if has(at(t, entries, "e_same"), "name") || !has(at(t, entries, "e_same"), "description") {
		t.Errorf("e_same: %s", js(t, entries, "e_same"))
	}
	if got := str(t, entries, "e_other", "name", "v"); got != "Иначе" {
		t.Errorf("a name text that differs must stay: %s", got)
	}
	if str(t, entries, "e_auth", "name", "v") != "Группа" {
		t.Error("the name of an authored entity is the text itself")
	}
	if rep.Projects[0].CodeNameTexts != 2 || !strings.Contains(printed(rep), "убрано: 2") {
		t.Errorf("report: %+v", *rep.Projects[0])
	}
	if run(t, ws, Options{}).Changed() {
		t.Error("not idempotent")
	}
}

// A text catalogue of the main language is made when the project has none, so
// that the names of its authored entities have a place.
func TestNameNeedsACatalogue(t *testing.T) {
	fixedNow(t)
	ws := mkws(t, map[string]string{
		"projects/p/project.json":  `{"id":"p","contractVersion":5,"languages":["en","ru"]}`,
		"projects/p/entities.json": `{"entities":[{"id":"e_a","name":"Alpha","kind":"app","origin":"authored"}]}`,
	})
	run(t, ws, Options{})
	if got := js(t, readTree(t, ws, "projects/p/text.en.json"), "entries", "e_a", "name"); got != `{"v":"Alpha","at":"2026-09-30T12:00:00Z","origin":"authored"}` {
		t.Errorf("name: %s", got)
	}
	if got := js(t, readTree(t, ws, "projects/p/text.en.json")); !strings.HasPrefix(got, `{"contractVersion":5,"language":"en","entries":`) {
		t.Errorf("catalogue: %s", got)
	}
	if exists(ws, "projects/p/text.ru.json") {
		t.Error("a catalogue of the other language was made")
	}
}
