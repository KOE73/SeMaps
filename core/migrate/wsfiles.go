package migrate

import (
	"fmt"
	"strings"
)

// isFallbackStyle: the three styles every library has for a kind or a type that
// names no style of its own; they need no `forKinds` (CONTRACT §11.5).
func isFallbackStyle(id string) bool {
	return id == "default.node" || id == "default.container" || id == "default.edge"
}

// defaultStyleIDs are the ids of the styles the tool ships (its styles.json).
func defaultStyleIDs(data []byte) (map[string]bool, error) {
	if len(data) == 0 {
		return nil, nil
	}
	t, err := parseTree(data)
	if err != nil {
		return nil, fmt.Errorf("стили по умолчанию: неверный JSON: %w", err)
	}
	root, ok := t.(*obj)
	if !ok {
		return nil, fmt.Errorf("стили по умолчанию: ожидался JSON-объект верхнего уровня")
	}
	ids := map[string]bool{}
	arr, _ := asArr(root.get("styles"))
	for _, it := range arr {
		if o, ok := asObj(it); ok {
			if id, ok := asStr(o.get("id")); ok {
				ids[id] = true
			}
		}
	}
	return ids, nil
}

// migrateStyles brings the workspace styles.json to contract v5. It is
// content-based, so a second run finds nothing to do. It returns the ids of the
// untyped styles it dropped on request: the views may still name them.
//
//   - `kinds` becomes `forKinds`, container styles get `appliesTo: container`,
//     `default.zone` becomes `default.container`;
//   - a style without `forKinds` whose id a shipped default has is dropped: the
//     default wins (the workspace file replaces the library whole, so what stays
//     in it hides the default of its id);
//   - the other styles without `forKinds` are listed («без типа»); with
//     DropUntypedStyles they are dropped too;
//   - a file left without styles is removed, so that the shipped library applies.
func migrateStyles(f *file, rep *Report, defaults map[string]bool, opt Options) (map[string]bool, error) {
	arr, err := f.array("styles")
	if err != nil {
		return nil, err
	}
	var items []*obj
	for i, it := range arr {
		o, ok := asObj(it)
		if !ok {
			return nil, ferr(f.rel, fmt.Sprintf("styles[%d]", i), "ожидался объект")
		}
		items = append(items, o)
	}

	// Snapshot of the original ids and basedOn links: "reaches default.zone"
	// is decided on the names as they were.
	baseOf := map[string]string{}
	ids := map[string]bool{}
	for _, o := range items {
		if id, ok := asStr(o.get("id")); ok {
			ids[id] = true
			b, _, _ := strField(o, "basedOn")
			baseOf[id] = b
		}
	}
	renameDefault := ids["default.zone"] && !ids["default.container"]
	// When both default styles exist, default.zone is a leftover the migration
	// does not touch; styles based on it are not container styles by that link.
	zoneIsContainer := !(ids["default.zone"] && ids["default.container"])
	isContainer := func(id string) bool {
		seen := map[string]bool{}
		for cur := id; cur != "" && !seen[cur]; {
			seen[cur] = true
			if cur == "default.zone" && !zoneIsContainer {
				return false
			}
			if isContainerBase(cur) {
				return true
			}
			b, ok := baseOf[cur]
			if !ok {
				bd, bok := builtinDef(cur)
				if !bok {
					return false
				}
				b = bd.basedOn
			}
			cur = b
		}
		return false
	}
	kindsRenamed, appliesSet := 0, 0
	for _, o := range items {
		origID, _ := asStr(o.get("id"))
		if o.has("kinds") {
			if o.has("forKinds") {
				o.del("kinds")
			} else {
				o.rename("kinds", "forKinds")
			}
			kindsRenamed++
			f.dirty = true
		}
		if renameDefault {
			if origID == "default.zone" {
				o.set("id", jsonStr("default.container"))
				f.dirty = true
				rep.WorkspaceNotes = append(rep.WorkspaceNotes, "styles.json: стиль default.zone переименован в default.container")
			}
			if b, ok, _ := strField(o, "basedOn"); ok && b == "default.zone" {
				o.set("basedOn", jsonStr("default.container"))
				f.dirty = true
			}
		}
		ap, _, _ := strField(o, "appliesTo")
		if (ap == "" || ap == "block" || ap == "container") && origID != "" && isContainer(origID) && ap != "container" {
			o.set("appliesTo", jsonStr("container"))
			appliesSet++
			f.dirty = true
		}
	}
	if kindsRenamed > 0 {
		rep.WorkspaceNotes = append(rep.WorkspaceNotes, fmt.Sprintf("styles.json: kinds → forKinds у %d стилей", kindsRenamed))
	}
	if appliesSet > 0 {
		rep.WorkspaceNotes = append(rep.WorkspaceNotes, fmt.Sprintf("styles.json: appliesTo → container у %d стилей", appliesSet))
	}

	// Styles without a type: the ones a shipped default has the id of are dropped —
	// the default wins; the others are kept and listed (forKinds is never
	// invented) or, on request, dropped.
	rep.StylesNoKind, rep.StylesNoKindEdge = nil, nil
	var kept []any
	droppedUntyped := map[string]bool{}
	for _, o := range items {
		id, ok := asStr(o.get("id"))
		untyped := ok && !o.has("forKinds")
		switch {
		case untyped && defaults[id]:
			rep.StylesLikeDefault = append(rep.StylesLikeDefault, id)
			f.dirty = true
			continue
		case untyped && opt.DropUntypedStyles && !isFallbackStyle(id):
			rep.StylesDropped = append(rep.StylesDropped, id)
			droppedUntyped[id] = true
			f.dirty = true
			continue
		}
		kept = append(kept, o)
		if !untyped {
			continue
		}
		ap, _, _ := strField(o, "appliesTo")
		switch ap {
		case "", "block", "container":
			if id != "default.node" && id != "default.container" {
				rep.StylesNoKind = append(rep.StylesNoKind, id)
			}
		case "edge":
			if id != "default.edge" {
				rep.StylesNoKindEdge = append(rep.StylesNoKindEdge, id)
			}
		}
	}
	if len(droppedUntyped) > 0 {
		// a kept style based on a dropped one loses the link: its base is gone
		for _, it := range kept {
			o := it.(*obj)
			if b, ok, _ := strField(o, "basedOn"); ok && droppedUntyped[b] {
				id, _ := asStr(o.get("id"))
				o.del("basedOn")
				rep.WorkspaceNotes = append(rep.WorkspaceNotes,
					fmt.Sprintf("styles.json: у стиля %s снята ссылка basedOn на удалённый стиль %s", id, b))
			}
		}
	}
	if f.dirty {
		if len(kept) == 0 {
			f.remove = true
			rep.WorkspaceNotes = append(rep.WorkspaceNotes,
				"styles.json: стилей не осталось, файл удалён — действует библиотека стилей по умолчанию (styles.json рабочего пространства заменяет её целиком)")
		} else {
			if len(kept) < len(items) {
				rep.WorkspaceNotes = append(rep.WorkspaceNotes,
					fmt.Sprintf("styles.json: остаётся %d стилей и заменяет библиотеку по умолчанию целиком — стили по умолчанию, которых в нём нет, в этом рабочем пространстве не действуют", len(kept)))
			}
			f.root.set("styles", kept)
		}
	}
	return droppedUntyped, nil
}

// migrateCanvas renames zone → container (top level and gap).
func migrateCanvas(f *file, rep *Report) error {
	var did []string
	if f.root.has("zone") && !f.root.has("container") {
		f.root.rename("zone", "container")
		did = append(did, "zone → container")
	}
	if gv, ok := f.root.lookup("gap"); ok {
		g, ok := asObj(gv)
		if !ok {
			return ferr(f.rel, "gap", "ожидался объект")
		}
		if g.has("zone") && !g.has("container") {
			g.rename("zone", "container")
			did = append(did, "gap.zone → gap.container")
		}
	}
	if len(did) > 0 {
		f.dirty = true
		rep.WorkspaceNotes = append(rep.WorkspaceNotes, "canvas.json: "+strings.Join(did, ", "))
	}
	return nil
}

// migrateWorkspaceFiles handles the workspace-level styles.json and canvas.json.
// The ids of the untyped styles dropped on request come back for the views.
func migrateWorkspaceFiles(ws string, styles *file, rep *Report, defaults map[string]bool, opt Options) ([]fileOut, map[string]bool, error) {
	var outs []fileOut
	var dropped map[string]bool
	if styles != nil {
		var err error
		if dropped, err = migrateStyles(styles, rep, defaults, opt); err != nil {
			return nil, nil, err
		}
		o, changed, err := styles.output()
		if err != nil {
			return nil, nil, err
		}
		if changed {
			outs = append(outs, o)
			if o.remove {
				rep.Removed = append(rep.Removed, styles.rel)
			} else {
				rep.Files = append(rep.Files, styles.rel)
			}
		}
	}
	canvas, err := loadFile(ws, "canvas.json", false)
	if err != nil {
		return nil, nil, err
	}
	if canvas != nil {
		if err := migrateCanvas(canvas, rep); err != nil {
			return nil, nil, err
		}
		o, changed, err := canvas.output()
		if err != nil {
			return nil, nil, err
		}
		if changed {
			outs = append(outs, o)
			rep.Files = append(rep.Files, canvas.rel)
		}
	}
	return outs, dropped, nil
}
