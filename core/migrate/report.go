package migrate

import (
	"fmt"
	"io"
	"strings"
)

// ProjectReport is what happened to one project.
type ProjectReport struct {
	ID          string
	Skipped     bool // already the current shape: nothing to change
	FromVersion int  // 0 when the project had no contractVersion

	Zones             int // zones found on all views
	ZoneEntities      int // group entities made from zones
	ContainerEntities int // group entities made for containers of containers.json that a zone shows
	Placements        int
	Overrides         int
	ContainsAdded     int
	TextsMoved        int
	TextsRemoved      int
	NamesMoved        int // names of authored entities written as texts (any language)
	CodeNameTexts     int // `name` texts of entities from code that only repeated their name: removed
	CodeEntries       int // entities whose codeRef/symbol became code[]
	EvidenceConverted int // relations whose via/evidence took the current shape

	Files   []string // changed files (relative to the workspace)
	Removed []string // deleted files

	// Items for a human.
	MatchRules         []string
	UnplacedContainers []string
	UnappliedOverrides []string
	KeptStyleIDs       []string
	DroppedStyleRefs   []string
	NameConflicts      []string
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
	Removed          []string // deleted workspace-level files
	WorkspaceNotes   []string
	StylesNoKind     []string
	StylesNoKindEdge []string
	// StylesLikeDefault are the styles dropped because a shipped default has the
	// same id and they name no type; StylesDropped are the untyped ones dropped
	// on request (--drop-untyped-styles).
	StylesLikeDefault []string
	StylesDropped     []string

	changed bool
}

// Changed reports whether anything was (or, in dry-run, would be) written.
func (r *Report) Changed() bool { return r != nil && r.changed }

// listLimit is high on purpose: the `match` rules and the containers of
// containers.json exist only in this report once the file is gone.
const listLimit = 200

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
			fmt.Fprintf(w, "\nПроект %s: уже в текущей форме\n", p.ID)
			continue
		}
		from := "нет версии"
		if p.FromVersion > 0 {
			from = fmt.Sprintf("%d", p.FromVersion)
		}
		fmt.Fprintf(w, "\nПроект %s\n", p.ID)
		if p.FromVersion < targetVersion {
			fmt.Fprintf(w, "  зон → сущностей: %d → %d\n", p.Zones, p.ZoneEntities)
			fmt.Fprintf(w, "  контейнеров containers.json, размещённых на видах → сущностей: %d\n", p.ContainerEntities)
			fmt.Fprintf(w, "  контейнеров containers.json без размещения (сущности нет): %d\n", len(p.UnplacedContainers))
			fmt.Fprintf(w, "  размещений: %d\n", p.Placements)
			fmt.Fprintf(w, "  override создано: %d\n", p.Overrides)
			fmt.Fprintf(w, "  связей contains добавлено: %d\n", p.ContainsAdded)
			fmt.Fprintf(w, "  текстов перенесено: %d, ключей z_/c_ удалено: %d\n", p.TextsMoved, p.TextsRemoved)
		}
		fmt.Fprintf(w, "  имён нарисованных сущностей записано в тексты: %d\n", p.NamesMoved)
		if p.CodeNameTexts > 0 {
			fmt.Fprintf(w, "  текстов name у сущностей из кода, дословно повторявших их имя (не читаются), убрано: %d\n", p.CodeNameTexts)
		}
		fmt.Fprintf(w, "  сущностей: codeRef/symbol → code[]: %d; связей: via/evidence в новой форме: %d\n", p.CodeEntries, p.EvidenceConverted)
		fmt.Fprintf(w, "  версия: %s → %d\n", from, targetVersion)
		if len(p.Files) > 0 {
			fmt.Fprintf(w, "  изменены файлы: %s\n", strings.Join(p.Files, ", "))
		}
		if len(p.Removed) > 0 {
			fmt.Fprintf(w, "  удалены файлы: %s\n", strings.Join(p.Removed, ", "))
		}
		human := len(p.MatchRules)+len(p.UnplacedContainers)+len(p.UnappliedOverrides)+len(p.KeptStyleIDs)+len(p.DroppedStyleRefs)+
			len(p.NameConflicts)+len(p.LostText)+len(p.NoName)+len(p.UnknownZones)+len(p.UnconsumedText)+len(p.Notes) > 0
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
			sub("контейнеры containers.json, не размещённые ни на одном виде (сущностей нет, их тексты z_/c_ удалены)", p.UnplacedContainers)
			sub("overrides не применены", p.UnappliedOverrides)
			sub("styleId оставлены", p.KeptStyleIDs)
			sub("styleId сняты: стиль удалён из styles.json (--drop-untyped-styles)", p.DroppedStyleRefs)
			sub("у зоны и контейнера в одном языке разные имена (взято имя зоны)", p.NameConflicts)
			sub("тексты и поля потеряны", p.LostText)
			sub("зоны без имени (взят хвост id)", p.NoName)
			sub("неизвестные зоны", p.UnknownZones)
			sub("тексты z_/c_ без зоны и контейнера (удалены)", p.UnconsumedText)
			sub("прочее", p.Notes)
		}
	}
	if len(r.Files) > 0 || len(r.Removed) > 0 || len(r.WorkspaceNotes) > 0 || len(r.StylesNoKind) > 0 || len(r.StylesNoKindEdge) > 0 ||
		len(r.StylesLikeDefault) > 0 || len(r.StylesDropped) > 0 {
		fmt.Fprintln(w, "\nОбщие файлы рабочего пространства")
		if len(r.Files) > 0 {
			fmt.Fprintf(w, "  изменены: %s\n", strings.Join(r.Files, ", "))
		}
		if len(r.Removed) > 0 {
			fmt.Fprintf(w, "  удалены: %s\n", strings.Join(r.Removed, ", "))
		}
		printList(w, "  ", r.WorkspaceNotes)
		ids := func(title string, list []string) {
			if len(list) == 0 {
				return
			}
			head := list
			if len(head) > 8 {
				head = head[:8]
			}
			tail := ""
			if len(list) > len(head) {
				tail = ", …"
			}
			fmt.Fprintf(w, "  %s: %d (%s%s)\n", title, len(list), strings.Join(head, ", "), tail)
		}
		ids("удалены стили, совпадающие по id со стилями по умолчанию и без типа (действует стиль по умолчанию)", r.StylesLikeDefault)
		ids("удалены стили без типа (--drop-untyped-styles)", r.StylesDropped)
		noKind := func(title string, list []string) {
			if len(list) == 0 {
				return
			}
			head := list
			if len(head) > 8 {
				head = head[:8]
			}
			tail := ""
			if len(list) > len(head) {
				tail = ", …"
			}
			fmt.Fprintf(w, "  Решает человек: %s: %d (%s%s); forKinds не выдуман (назначьте тип или запустите с --drop-untyped-styles)\n", title, len(list), strings.Join(head, ", "), tail)
		}
		noKind("стили без типа", r.StylesNoKind)
		noKind("стили без типа (связи)", r.StylesNoKindEdge)
	}
}
