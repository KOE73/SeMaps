import { el, replaceChildren } from "../util/dom.js";
import { i18n } from "../workbench/i18n/I18nService.js";
import type { DiagramEditor } from "./DiagramEditor.js";

export class BasePanel {
  /** Kinds the list is narrowed to; empty means every kind. */
  private readonly kinds = new Set<string>();

  constructor(
    private readonly searchInput: HTMLInputElement,
    private readonly body: HTMLElement,
    private readonly editor: DiagramEditor,
    /** Where the kind buttons go; none, no buttons. */
    private readonly kindsSlot?: HTMLElement,
  ) {
    if (this.searchInput && typeof this.searchInput.addEventListener === "function") {
      this.searchInput.addEventListener("input", () => this.render());
    }
  }

  render(): void {
    const doc = this.editor.canvas.model;
    const t = i18n.d.panels.base;
    if (!doc) {
      if (this.kindsSlot) replaceChildren(this.kindsSlot);
      replaceChildren(this.body, el("div", { class: "inspector-empty", text: t.emptyNoModel }));
      return;
    }

    const entities = doc.entities;
    if (!entities || entities.length === 0) {
      if (this.kindsSlot) replaceChildren(this.kindsSlot);
      replaceChildren(this.body, el("div", { class: "inspector-empty", text: t.emptyNoEntities }));
      return;
    }

    const query = String(this.searchInput?.value ?? "").toLowerCase().trim();
    const texts = doc.bundle?.text?.entries || {};

    const placedIds = new Set(Array.from(doc.elements()).map((e: any) => e.id));

    const found = entities.filter((e: any) => {
      if (!e) return false;
      const id = String(e.id ?? "");
      const name = String(texts[id]?.name || e.name || id);
      return name.toLowerCase().includes(query) || id.toLowerCase().includes(query);
    });

    // A kind picked in another project, or gone since, must not hide everything.
    const present = new Set(entities.map((e: any) => String(e?.kind ?? "")));
    for (const k of [...this.kinds]) if (!present.has(k)) this.kinds.delete(k);
    this.renderKinds(present, found);

    const matches = this.kinds.size === 0 ? found : found.filter((e: any) => this.kinds.has(String(e.kind ?? "")));
    if (matches.length === 0) {
      replaceChildren(this.body, el("div", { class: "inspector-empty", text: t.noMatches }));
      return;
    }

    replaceChildren(
      this.body,
      ...matches.map((e: any) => {
        const id = String(e?.id ?? "");
        const name = String(texts[id]?.name || e?.name || id);
        const isPlaced = placedIds.has(id);

        const row = document.createElement("div");
        row.style.display = "flex";
        row.style.justifyContent = "space-between";
        row.style.alignItems = "center";
        row.style.padding = "4px 8px";
        row.style.borderBottom = "1px solid var(--line)";
        row.style.gap = "8px";
        row.draggable = true;
        row.addEventListener("dragstart", (ev) => {
          ev.dataTransfer?.setData("application/semaps-entity", JSON.stringify({
            id: e.id,
            entity: e,
            textEntry: texts[e.id]
          }));
        });
        
        const labelCol = document.createElement("div");
        labelCol.style.overflow = "hidden";
        labelCol.style.whiteSpace = "nowrap";
        labelCol.style.textOverflow = "ellipsis";
        
        const title = document.createElement("span");
        title.textContent = name;
        title.style.fontWeight = "600";
        title.style.fontSize = "11px";
        title.style.marginRight = "6px";
        
        const subtitle = document.createElement("span");
        subtitle.textContent = e.kind;
        subtitle.className = "mono muted";
        subtitle.style.fontSize = "10px";
        
        labelCol.appendChild(title);
        labelCol.appendChild(subtitle);
        row.appendChild(labelCol);
        
        if (isPlaced) {
          const b = el("span", { text: "✓", title: "На виде" });
          b.style.color = "var(--accent)";
          b.style.fontSize = "12px";
          b.style.flexShrink = "0";
          row.appendChild(b);
        } else {
          const btn = el("button", { 
            text: "+",
            title: "Добавить на вид",
            on: { click: () => this.placeEntity(e, texts[e.id]) }
          });
          btn.style.padding = "2px 6px";
          btn.style.fontSize = "12px";
          btn.style.flexShrink = "0";
          btn.style.background = "var(--accent-soft)";
          btn.style.border = "none";
          btn.style.color = "#fff";
          btn.style.borderRadius = "4px";
          btn.style.cursor = "pointer";
          row.appendChild(btn);
        }
        return row;
      })
    );
  }

  /**
   * One button per kind present in the project, with how many of the current
   * search results it holds. Built from the data: the set of kinds is open.
   */
  private renderKinds(present: Set<string>, found: any[]): void {
    if (!this.kindsSlot) return;
    const t = i18n.d.panels.base;
    const counts = new Map<string, number>();
    for (const e of found) counts.set(String(e.kind ?? ""), (counts.get(String(e.kind ?? "")) ?? 0) + 1);

    const chip = (label: string, count: number, active: boolean, onClick: () => void, title?: string): HTMLElement =>
      el("button", {
        type: "button",
        class: `kind-chip${active ? " is-active" : ""}${count === 0 ? " is-empty" : ""}`,
        title,
        on: { click: onClick },
      }, [el("span", { text: label }), el("span", { class: "kind-chip-count", text: String(count) })]);

    replaceChildren(
      this.kindsSlot,
      chip(t.allKinds, found.length, this.kinds.size === 0, () => {
        this.kinds.clear();
        this.render();
      }),
      ...[...present].sort().map((kind) =>
        chip(kind || "—", counts.get(kind) ?? 0, this.kinds.has(kind), () => {
          if (this.kinds.has(kind)) this.kinds.delete(kind);
          else this.kinds.add(kind);
          this.render();
        }, t.kindHint),
      ),
    );
  }

  private placeEntity(entity: any, textEntry: any): void {
    const doc = this.editor.canvas.model;
    if (!doc) return;
    
    const at = this.editor.canvas.viewCenter();
    const id = entity.id;
    const name = textEntry?.name || entity.name || id;
    
    const elToAdd = {
      id,
      kind: "node" as const,
      type: entity.kind,
      label: name,
      tags: [],
      metadata: { description: textEntry?.description, codeRef: entity.codeRef },
      x: at.x,
      y: at.y,
      width: 180,
      height: 60,
      parent: null,
      children: [],
      wireOrder: Number.POSITIVE_INFINITY,
      raw: { _entity: entity },
    };

    const target = doc.containerAt({ x: at.x + 90, y: at.y + 30 });
    doc.add(elToAdd, target);
    
    (this.editor as any).commit("place-entity");
    this.editor.canvas.select(id);
    this.render();
  }
}

