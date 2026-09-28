import { el, replaceChildren } from "../../util/dom.js";
import { fmt, t } from "../../shell/strings.js";
import { CodeViewerDialog } from "../../editor/code/CodeViewerDialog.js";
import type { GraphEdge, GraphNode } from "./types.js";

/**
 * The click-selection side panel: name, kind, file/line, entity, containers,
 * presence, and its edges grouped by kind, each with a link that opens the
 * source at the line (PLAN_20260928-2 step 4).
 */
export class GraphPanel {
  readonly root: HTMLElement;
  private dialog: CodeViewerDialog | undefined;

  constructor() {
    this.root = el("aside", { class: "graph-panel" }, [el("p", { class: "tool-muted", text: t.graphPanelEmpty })]);
  }

  clear(): void {
    replaceChildren(this.root, el("p", { class: "tool-muted", text: t.graphPanelEmpty }));
  }

  show(node: GraphNode, edges: readonly GraphEdge[], nodeById: Map<string, GraphNode>): void {
    const row = (label: string, value: string | undefined | null) =>
      value ? el("div", { class: "graph-panel-row" }, [el("span", { class: "graph-panel-label", text: label }), el("span", { text: value })]) : null;

    const fileRow =
      node.file && el("div", { class: "graph-panel-row" }, [
        el("span", { class: "graph-panel-label", text: t.graphFile }),
        this.sourceLink(node.file, node.line ?? 1, node.name ?? node.id),
      ]);

    const byKind = new Map<string, GraphEdge[]>();
    for (const e of edges) {
      if (e.from !== node.id && e.to !== node.id) continue;
      (byKind.get(e.kind) ?? byKind.set(e.kind, []).get(e.kind)!).push(e);
    }
    const edgeGroups = [...byKind.entries()].map(([kind, list]) =>
      el("details", { attrs: { open: "" } }, [
        el("summary", { text: `${kind} (${list.length})` }),
        el(
          "ul",
          { class: "graph-panel-edges" },
          list.map((e) => {
            const other = nodeById.get(e.from === node.id ? e.to : e.from);
            const dir = e.from === node.id ? "→" : "←";
            const label = `${dir} ${other?.name ?? (e.from === node.id ? e.to : e.from)}`;
            return el("li", { text: e.via?.member ? `${label} · ${e.via.member}` : label });
          }),
        ),
      ]),
    );

    replaceChildren(
      this.root,
      el("h2", { text: node.name ?? node.id }),
      row(t.graphKind, node.kind),
      row(t.graphNativeKind, node.nativeKind),
      row(t.graphVisibility, node.visibility),
      fileRow || null,
      row(t.graphEntity, node.entity),
      row(t.graphContainers, node.containers?.join(", ")),
      row(
        t.graphPresence,
        node.presence === "code" ? t.graphPresenceCode : node.presence === "model" ? t.graphPresenceModel : t.graphPresenceBoth,
      ),
      edgeGroups.length ? el("h3", { text: t.graphEdgesOf }) : null,
      ...edgeGroups,
    );
  }

  private sourceLink(file: string, line: number, title: string): HTMLElement {
    const a = el("a", { class: "tool-btn", text: t.graphOpenSource });
    a.href = "#";
    a.addEventListener("click", (e) => {
      e.preventDefault();
      this.dialog ??= new CodeViewerDialog();
      void this.dialog.openAt(file, line, title);
    });
    return el("span", {}, [el("code", { text: `${file}:${line}` }), " ", a]);
  }
}

export function statsLine(nodes: number, edges: number, hiddenMissing?: { nodes: number; edges: number }): string {
  const base = fmt(t.graphStats, { nodes: String(nodes), edges: String(edges) });
  if (!hiddenMissing || (hiddenMissing.nodes === 0 && hiddenMissing.edges === 0)) return base;
  return `${base} · ${fmt(t.graphHiddenMissing, { nodes: String(hiddenMissing.nodes), edges: String(hiddenMissing.edges) })}`;
}
