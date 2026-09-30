package main

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"semaps/core"
)

type defaultStyle struct {
	ID        string   `json:"id"`
	AppliesTo string   `json:"appliesTo"`
	ForKinds  []string `json:"forKinds"`
	BasedOn   string   `json:"basedOn"`
	Icon      struct {
		Glyph string `json:"glyph"`
	} `json:"icon"`
}

func defaultStyles(t *testing.T) []defaultStyle {
	t.Helper()
	var doc struct {
		Styles []defaultStyle `json:"styles"`
	}
	if err := json.Unmarshal(mustRead(t, "defaults/styles.json"), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Styles
}

// The shipped styles and the shipped dictionary agree: every style belongs to
// types that exist (ADR_20260927-7, ADR_20260930-2), a container style to
// container kinds and a block style to the others; each type's base style is
// one of its own; the fallbacks are the only styles without a type.
func TestDefaultStylesBelongToTheDictionary(t *testing.T) {
	catalog, err := core.ParseKinds(defaultKinds(), nil)
	if err != nil {
		t.Fatal(err)
	}
	styles := defaultStyles(t)
	byID := map[string]defaultStyle{}
	for _, s := range styles {
		byID[s.ID] = s
	}
	for id, sort := range map[string]string{"default.node": "block", "default.container": "container", "default.edge": "edge"} {
		if s, ok := byID[id]; !ok || s.AppliesTo != sort {
			t.Errorf("%s: a fallback of %s styles is missing: %+v", id, sort, s)
		}
	}
	for _, s := range styles {
		if strings.HasPrefix(s.ID, "default.") {
			if len(s.ForKinds) != 0 {
				t.Errorf("%s: a fallback has no forKinds", s.ID)
			}
			continue
		}
		if strings.HasPrefix(s.ID, "zone.") || strings.HasPrefix(s.ID, "container.") {
			t.Errorf("%s: a colour is an override of a placement, not a style (ADR_20260927-7)", s.ID)
		}
		if len(s.ForKinds) == 0 {
			t.Errorf("%s: forKinds is mandatory (ADR_20260927-7)", s.ID)
		}
		for _, k := range s.ForKinds {
			switch s.AppliesTo {
			case "edge":
				if _, ok := catalog.LookupRelation(k); !ok {
					t.Errorf("%s: forKinds %q is no relation type of the dictionary", s.ID, k)
				}
			case "block", "container":
				kind, ok := catalog.Lookup(k)
				switch {
				case !ok:
					t.Errorf("%s: forKinds %q is no kind of the dictionary", s.ID, k)
				case kind.Container != (s.AppliesTo == "container"):
					t.Errorf("%s: appliesTo %s, but kind %s has container=%v", s.ID, s.AppliesTo, k, kind.Container)
				}
			default:
				t.Errorf("%s: appliesTo %q", s.ID, s.AppliesTo)
			}
		}
		if g := s.Icon.Glyph; g != "" {
			for _, r := range g {
				if r > unicode.MaxASCII {
					t.Errorf("%s: icon.glyph %q is not a registry key (ADR_20260929_editor_icons-are-registry-keys)", s.ID, g)
				}
			}
		}
	}
	owns := func(style, typeID string) bool {
		s, ok := byID[style]
		if !ok {
			return false
		}
		for _, k := range s.ForKinds {
			if k == typeID {
				return true
			}
		}
		return false
	}
	for _, g := range catalog.Groups {
		for _, k := range g.Kinds {
			if k.Style != "" && !owns(k.Style, k.ID) {
				t.Errorf("kind %s: base style %q is not a style of that kind", k.ID, k.Style)
			}
			if s, ok := byID[k.ID]; ok && k.Style == "" && !owns(s.ID, k.ID) {
				t.Errorf("kind %s: the style with its id is not a style of that kind", k.ID)
			}
		}
	}
	for _, g := range catalog.RelationGroups {
		for _, ty := range g.Types {
			if ty.Style != "" && !owns(ty.Style, ty.ID) {
				t.Errorf("relation type %s: base style %q is not a style of that type", ty.ID, ty.Style)
			}
			if s, ok := byID[ty.ID]; ok && ty.Style == "" && !owns(s.ID, ty.ID) {
				t.Errorf("relation type %s: the style with its id is not a style of that type", ty.ID)
			}
			if _, ok := byID[ty.ID]; ok && byID[ty.ID].AppliesTo != "edge" {
				t.Errorf("relation type %s: the style with its id is not an edge style", ty.ID)
			}
		}
	}
	// the relation types sync derives (CONTRACT §5) are all in the dictionary
	for _, id := range []string{"extends", "implements", "contains", "depends", "uses", "injects",
		"holds.one", "holds.optional", "holds.many", "holds.many.ro", "holds.keyed", "holds.keyed.ro",
		"holds.one.internal", "holds.optional.internal", "holds.many.internal", "holds.many.ro.internal", "holds.keyed.internal", "holds.keyed.ro.internal"} {
		if _, ok := catalog.LookupRelation(id); !ok {
			t.Errorf("sync derives %s, the dictionary lacks it", id)
		}
	}
}

// The kinds the extractors print (docs/extractors/*.md) are in the dictionary,
// and the containers among them are containers.
func TestDefaultKindsCoverWhatExtractorsPrint(t *testing.T) {
	catalog, err := core.ParseKinds(defaultKinds(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"class", "interface", "struct", "record", "record-struct", "enum", "delegate",
		"abstract-class", "static-class", "sealed-class", "abstract-record", "sealed-record", "readonly-struct", "ref-struct",
		"func", "map", "slice", "array", "chan", "pointer", "alias", "int", "string", "bool", "float64",
		"function", "method", "constructor", "const", "var", "constant", "variable",
		"type-alias", "trait", "union", "impl", "static", "protocol", "object", "data class", "sealed class", "fun"} {
		if _, ok := catalog.Lookup(k); !ok {
			t.Errorf("an extractor prints nativeKind %q, the dictionary lacks it", k)
		}
	}
	for _, k := range []string{"assembly", "namespace", "package", "module", "crate", "file"} {
		if !catalog.IsContainer(k) {
			t.Errorf("%s holds other symbols: a container kind", k)
		}
	}
	for _, k := range []string{"class", "interface", "component", "service", "actor", "store", "note"} {
		if catalog.IsContainer(k) {
			t.Errorf("%s is no container", k)
		}
	}
	if !catalog.IsContainer("group") {
		t.Error("group is the container of a picture")
	}
	for _, g := range catalog.Groups {
		for _, k := range g.Kinds {
			if len(k.Name) == 0 || k.Name["ru"] == "" || k.Name["en"] == "" {
				t.Errorf("kind %s: names in ru and en are expected", k.ID)
			}
		}
	}
	for _, g := range catalog.RelationGroups {
		for _, ty := range g.Types {
			if ty.Name["ru"] == "" || ty.Name["en"] == "" {
				t.Errorf("relation type %s: names in ru and en are expected", ty.ID)
			}
		}
	}
}
