package migrate

import (
	"fmt"
	"io"
	"strings"
)

// ProjectReport is what happened to one project.
type ProjectReport struct {
	ID          string
	Skipped     bool // already contract v5
	FromVersion int  // 0 when the project had no contractVersion

	Zones             int // zones found on all views
	ZoneEntities      int // group entities made from zones
	ContainerEntities int // group entities made from containers.json
	Placements        int
	Overrides         int
	ContainsAdded     int
	TextsMoved        int
	TextsRemoved      int

	Files   []string // changed files (relative to the workspace)
	Removed []string // deleted files

	// Items for a human.
	MatchRules         []string
	UnappliedOverrides []string
	KeptStyleIDs       []string
	LostNames          []string
	LostText           []string
	NoName             []string
	UnknownZones       []string
	UnconsumedText     []string
	Notes              []string
}

// Report is the result of Workspace.
type Report struct {
	DryRun   bool
	Projects []*ProjectReport

	Files            []string // changed workspace-level files
	WorkspaceNotes   []string
	StylesNoKind     []string
	StylesNoKindEdge []string

	changed bool
}

// Changed reports whether anything was (or, in dry-run, would be) written.
func (r *Report) Changed() bool { return r != nil && r.changed }

const listLimit = 25

func printList(w io.Writer, indent string, items []string) {
	for i, it := range items {
		if i == listLimit {
			fmt.Fprintf(w, "%s… и ещё %d\n", indent, len(items)-listLimit)
			return
		}
		fmt.Fprintf(w, "%s- %s\n", indent, it)
	}
}

func section(w io.Writer, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s (%d):\n", title, len(items))
	printList(w, "    ", items)
}

// Print writes the human-readable report in Russian.
func (r *Report) Print(w io.Writer) {
	if !r.Changed() {
		fmt.Fprintln(w, "Уже контракт v5, менять нечего.")
		return
	}
	if r.DryRun {
		fmt.Fprintln(w, "Пробный прогон (--dry-run): файлы не изменены; ниже то, что было бы сделано.")
	}
	for _, p := range r.Projects {
		if p.Skipped {
			fmt.Fprintf(w, "\nПроект %s: уже v5\n", p.ID)
			continue
		}
		from := "нет версии"
		if p.FromVersion > 0 {
			from = fmt.Sprintf("%d", p.FromVersion)
		}
		fmt.Fprintf(w, "\nПроект %s\n", p.ID)
		fmt.Fprintf(w, "  зон → сущностей: %d → %d\n", p.Zones, p.ZoneEntities)
		fmt.Fprintf(w, "  контейнеров containers.json → сущностей: %d\n", p.ContainerEntities)
		fmt.Fprintf(w, "  размещений: %d\n", p.Placements)
		fmt.Fprintf(w, "  override создано: %d\n", p.Overrides)
		fmt.Fprintf(w, "  связей contains добавлено: %d\n", p.ContainsAdded)
		fmt.Fprintf(w, "  текстов перенесено: %d, ключей z_/c_ удалено: %d\n", p.TextsMoved, p.TextsRemoved)
		fmt.Fprintf(w, "  версия: %s → %d\n", from, targetVersion)
		if len(p.Files) > 0 {
			fmt.Fprintf(w, "  изменены файлы: %s\n", strings.Join(p.Files, ", "))
		}
		if len(p.Removed) > 0 {
			fmt.Fprintf(w, "  удалены файлы: %s\n", strings.Join(p.Removed, ", "))
		}
		human := len(p.MatchRules)+len(p.UnappliedOverrides)+len(p.KeptStyleIDs)+len(p.LostNames)+
			len(p.LostText)+len(p.NoName)+len(p.UnknownZones)+len(p.UnconsumedText)+len(p.Notes) > 0
		if human {
			fmt.Fprintln(w, "  Решает человек:")
			sub := func(title string, items []string) {
				if len(items) == 0 {
					return
				}
				fmt.Fprintf(w, "    %s (%d):\n", title, len(items))
				printList(w, "      ", items)
			}
			sub("правила match (match, axis и theme контейнеров не переносятся; containers.json удалён)", p.MatchRules)
			sub("overrides не применены", p.UnappliedOverrides)
			sub("styleId оставлены", p.KeptStyleIDs)
			sub("имена на других языках потеряны (имя сущности не переводится)", p.LostNames)
			sub("тексты и поля потеряны", p.LostText)
			sub("зоны без имени (взят хвост id)", p.NoName)
			sub("неизвестные зоны", p.UnknownZones)
			sub("тексты z_/c_ без зоны и контейнера (удалены)", p.UnconsumedText)
			sub("прочее", p.Notes)
		}
	}
	if len(r.Files) > 0 || len(r.WorkspaceNotes) > 0 || len(r.StylesNoKind) > 0 || len(r.StylesNoKindEdge) > 0 {
		fmt.Fprintln(w, "\nОбщие файлы рабочего пространства")
		if len(r.Files) > 0 {
			fmt.Fprintf(w, "  изменены: %s\n", strings.Join(r.Files, ", "))
		}
		printList(w, "  ", r.WorkspaceNotes)
		noKind := func(title string, ids []string) {
			if len(ids) == 0 {
				return
			}
			head := ids
			if len(head) > 8 {
				head = head[:8]
			}
			tail := ""
			if len(ids) > len(head) {
				tail = ", …"
			}
			fmt.Fprintf(w, "  Решает человек: %s: %d (%s%s); forKinds не выдуман\n", title, len(ids), strings.Join(head, ", "), tail)
		}
		noKind("стили без типа", r.StylesNoKind)
		noKind("стили без типа (связи)", r.StylesNoKindEdge)
	}
}
