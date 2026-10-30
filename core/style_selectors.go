package core

import (
	"encoding/json"
	"slices"
	"strings"
)

// StyleSelector is a versioned, declarative predicate tree, never executable code.
// Exactly one of all/any/not or field+op+value is present.
type StyleSelector struct {
	All   []StyleSelector `json:"all,omitempty"`
	Any   []StyleSelector `json:"any,omitempty"`
	Not   *StyleSelector  `json:"not,omitempty"`
	Field string          `json:"field,omitempty"`
	Op    string          `json:"op,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

type SelectorField struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Operators   []string `json:"operators"`
}

type selectorContext struct{ values map[string][]string }
type selectorField struct {
	SelectorField
	value func(selectorContext) ([]string, bool)
}

// Adding a selectable attribute is one registry entry and its context provider.
// The UI and MCP discover the same descriptor; there is no second field grammar.
var selectorFields = makeSelectorFields()

func makeSelectorFields() map[string]selectorField {
	fields := map[string]selectorField{}
	add := func(id, name, description string, namespace bool) {
		ops := []string{"eq", "neq", "in", "notIn", "prefix", "contains", "exists"}
		if namespace {
			ops = append(ops, "within")
		}
		fields[id] = selectorField{SelectorField{id, name, description, ops}, func(c selectorContext) ([]string, bool) { v, ok := c.values[id]; return v, ok }}
	}
	add("project", "Проект SeMaps", "Идентификатор проекта карты, не проекта исходного кода.", false)
	add("view", "Схема", "Идентификатор вида, на котором находится размещение.", false)
	add("family", "Семейство оформления", "block, container или edge.", false)
	for _, prefix := range []string{"", "source.", "target."} {
		label := ""
		if prefix == "source." {
			label = "Источник связи: "
		}
		if prefix == "target." {
			label = "Цель связи: "
		}
		for _, f := range []struct{ id, name, description string }{
			{"id", "Идентификатор", "Идентификатор сущности; без префикса у связи — идентификатор связи."},
			{"kind", "Тип", "kind сущности; без префикса у связи — её type."},
			{"name", "Имя из кода", "Поле name из реестра, не переведённая подпись."},
			{"namespace", "Namespace", "Поле namespace; within включает точное имя и потомков через точку."},
			{"origin", "Происхождение", "code или authored, только если известно."},
			{"status", "Состояние", "present, missing или planned, только если известно."},
			{"assembly", "Сборка", "Идентификаторы сущностей-сборок: сама сборка и предки по contains; положение на схеме не учитывается."},
			{"library", "Внешняя библиотека", "Точные идентификаторы из code[].library, без угадывания по имени типа."},
			{"language", "Язык кода", "Значения code[].lang."},
			{"code.ref", "Файл кода", "Пути code[].ref от source_root; несколько реализаций дают несколько значений."},
		} {
			add(prefix+f.id, label+f.name, f.description, f.id == "namespace")
		}
	}
	return fields
}

func StyleSelectorFields() []SelectorField {
	out := []SelectorField{}
	for _, f := range selectorFields {
		out = append(out, f.SelectorField)
	}
	slices.SortFunc(out, func(a, b SelectorField) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func parseSelector(raw json.RawMessage) (StyleSelector, error) {
	var s StyleSelector
	if len(raw) == 0 {
		return s, refuse("selector is required")
	}
	if err := validateSelectorJSON(raw, 0); err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, refuse("invalid selector: %v", err)
	}
	return s, nil
}

func validateSelectorJSON(raw json.RawMessage, depth int) error {
	if depth > 16 {
		return refuse("selector depth exceeds 16")
	}
	var o map[string]json.RawMessage
	if json.Unmarshal(raw, &o) != nil || o == nil {
		return refuse("selector must be an object")
	}
	for k := range o {
		if !slices.Contains([]string{"all", "any", "not", "field", "op", "value"}, k) {
			return refuse("unknown selector key %s", k)
		}
	}
	branches := 0
	for _, k := range []string{"all", "any", "not", "field"} {
		if _, ok := o[k]; ok {
			branches++
		}
	}
	if branches != 1 {
		return refuse("selector needs exactly one of all, any, not, field")
	}
	for _, k := range []string{"all", "any"} {
		if raw, ok := o[k]; ok {
			if len(o) != 1 {
				return refuse("%s selector cannot contain leaf fields", k)
			}
			var children []json.RawMessage
			if json.Unmarshal(raw, &children) != nil || len(children) == 0 || len(children) > 64 {
				return refuse("%s requires 1..64 conditions", k)
			}
			for _, child := range children {
				if err := validateSelectorJSON(child, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if raw, ok := o["not"]; ok {
		if len(o) != 1 {
			return refuse("not cannot contain leaf fields")
		}
		return validateSelectorJSON(raw, depth+1)
	}
	var field, op string
	_ = json.Unmarshal(o["field"], &field)
	_ = json.Unmarshal(o["op"], &op)
	f, ok := selectorFields[field]
	if !ok {
		return refuse("unknown selector field %q; use style_selector_fields", field)
	}
	if !slices.Contains(f.Operators, op) {
		return refuse("unsupported selector operation %q for %s", op, field)
	}
	if op == "exists" {
		var v bool
		if json.Unmarshal(o["value"], &v) != nil || string(o["value"]) == "null" {
			return refuse("exists requires a boolean value")
		}
		return nil
	}
	if op == "in" || op == "notIn" {
		var v []string
		if json.Unmarshal(o["value"], &v) != nil || len(v) == 0 {
			return refuse("%s requires a non-empty string array", op)
		}
		return nil
	}
	var v string
	if json.Unmarshal(o["value"], &v) != nil || v == "" {
		return refuse("%s requires a non-empty string", op)
	}
	return nil
}

// Unknown attributes propagate through NOT: unavailable assembly facts must
// never silently match a negative condition. EXISTS explicitly tests absence.
type selectorTruth int

const (
	selectorUnknown selectorTruth = -1
	selectorFalse   selectorTruth = 0
	selectorTrue    selectorTruth = 1
)

func selectorMatch(s StyleSelector, c selectorContext) selectorTruth {
	if s.Not != nil {
		v := selectorMatch(*s.Not, c)
		if v == selectorUnknown {
			return v
		}
		return 1 - v
	}
	if s.All != nil || s.Any != nil {
		children := s.All
		all := true
		if s.Any != nil {
			children = s.Any
			all = false
		}
		unknown := false
		for _, child := range children {
			v := selectorMatch(child, c)
			if all && v == selectorFalse {
				return selectorFalse
			}
			if !all && v == selectorTrue {
				return selectorTrue
			}
			unknown = unknown || v == selectorUnknown
		}
		if unknown {
			return selectorUnknown
		}
		if all {
			return selectorTrue
		}
		return selectorFalse
	}
	f, ok := selectorFields[s.Field]
	if !ok {
		return selectorUnknown
	}
	values, known := f.value(c)
	if s.Op == "exists" {
		var wanted bool
		_ = json.Unmarshal(s.Value, &wanted)
		if known == wanted {
			return selectorTrue
		}
		return selectorFalse
	}
	if !known {
		return selectorUnknown
	}
	var wanted string
	var candidates []string
	if s.Op == "in" || s.Op == "notIn" {
		_ = json.Unmarshal(s.Value, &candidates)
	} else {
		_ = json.Unmarshal(s.Value, &wanted)
		candidates = []string{wanted}
	}
	matched := false
	for _, v := range values {
		for _, candidate := range candidates {
			switch s.Op {
			case "eq", "neq", "in", "notIn":
				matched = matched || v == candidate
			case "prefix":
				matched = matched || strings.HasPrefix(v, candidate)
			case "contains":
				matched = matched || strings.Contains(v, candidate)
			case "within":
				matched = matched || v == candidate || strings.HasPrefix(v, candidate+".")
			}
		}
	}
	if s.Op == "neq" || s.Op == "notIn" {
		matched = !matched
	}
	if matched {
		return selectorTrue
	}
	return selectorFalse
}
