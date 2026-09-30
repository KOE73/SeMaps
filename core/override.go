package core

// `override`: a partial style of one placement on a view, or of one edge entry
// of a view (CONTRACT §11.6). The two tables below are the one place that says
// what may be overridden; a new field is a line here and a line in the
// contract's table (and editor/src/model/override.ts) — nothing else changes.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// OverrideFields: what a placement may override.
var OverrideFields = []string{
	"fill",         // заливка
	"border.color", // цвет рамки
	"border.dash",  // штрих рамки: SVG stroke-dasharray или none
	"header.fill",  // заливка шапки контейнера
	"icon.glyph",   // значок: ключ реестра иконок
}

// EdgeOverrideFields: what an edge entry of a view may override.
var EdgeOverrideFields = []string{
	"line.color", // цвет линии
	"line.width", // толщина линии
	"line.dash",  // штрих линии: SVG stroke-dasharray или none
}

// CheckOverride refuses any key of a placement's override outside
// OverrideFields. An absent or null override is fine.
func CheckOverride(raw json.RawMessage) error {
	return checkOverride(raw, OverrideFields, "a placement")
}

// CheckEdgeOverride refuses any key of an edge entry's override outside
// EdgeOverrideFields. An absent or null override is fine.
func CheckEdgeOverride(raw json.RawMessage) error {
	return checkOverride(raw, EdgeOverrideFields, "an edge")
}

func checkOverride(raw json.RawMessage, table []string, on string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var o map[string]json.RawMessage
	if err := json.Unmarshal(raw, &o); err != nil {
		return fmt.Errorf("override: an object of style fields (CONTRACT §11.6)")
	}
	return checkOverrideAt("", o, table, on)
}

func checkOverrideAt(prefix string, o map[string]json.RawMessage, table []string, on string) error {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		path := prefix + k
		if slices.Contains(table, path) {
			continue
		}
		var inner map[string]json.RawMessage
		if json.Unmarshal(o[k], &inner) == nil && inner != nil && overrideBranch(path, table) {
			if err := checkOverrideAt(path+".", inner, table, on); err != nil {
				return err
			}
			continue
		}
		return fmt.Errorf("override: %q cannot be overridden on %s; allowed: %s (CONTRACT §11.6)", path, on, strings.Join(table, ", "))
	}
	return nil
}

// overrideBranch: some allowed field lies under path.
func overrideBranch(path string, table []string) bool {
	for _, f := range table {
		if strings.HasPrefix(f, path+".") {
			return true
		}
	}
	return false
}
