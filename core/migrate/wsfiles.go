package migrate

import (
	"fmt"
	"strings"
)

// migrateStyles brings the workspace styles.json to contract v5. It is
// content-based, so a second run finds nothing to do.
func migrateStyles(f *file, rep *Report) error {
	arr, err := f.array("styles")
	if err != nil {
		return err
	}
	var items []*obj
	for i, it := range arr {
		o, ok := asObj(it)
		if !ok {
			return ferr(f.rel, fmt.Sprintf("styles[%d]", i), "ожидался объект")
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

	// Styles without a kind are kept and listed: forKinds is never invented.
	rep.StylesNoKind, rep.StylesNoKindEdge = nil, nil
	for _, o := range items {
		id, ok := asStr(o.get("id"))
		if !ok || o.has("forKinds") {
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
	return nil
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
func migrateWorkspaceFiles(ws string, styles *file, rep *Report) ([]fileOut, error) {
	var outs []fileOut
	if styles != nil {
		if err := migrateStyles(styles, rep); err != nil {
			return nil, err
		}
		o, changed, err := styles.output()
		if err != nil {
			return nil, err
		}
		if changed {
			outs = append(outs, o)
			rep.Files = append(rep.Files, styles.rel)
		}
	}
	canvas, err := loadFile(ws, "canvas.json", false)
	if err != nil {
		return nil, err
	}
	if canvas != nil {
		if err := migrateCanvas(canvas, rep); err != nil {
			return nil, err
		}
		o, changed, err := canvas.output()
		if err != nil {
			return nil, err
		}
		if changed {
			outs = append(outs, o)
			rep.Files = append(rep.Files, canvas.rel)
		}
	}
	return outs, nil
}
