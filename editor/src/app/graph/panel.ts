import { el, replaceChildren } from "../../util/dom.js";
import { fmt, t } from "../../shell/strings.js";
import { CodeViewerDialog } from "../../editor/code/CodeViewerDialog.js";
import { kindIconEl } from "../../ui/kindIcons.js";
import type { GraphEdge, GraphNode } from "./types.js";

const CALL_KINDS = ["calls", "constructs"];

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
    // A value that is a kind carries the kind's icon — the same as in the list and the filters.
    const row = (label: string, value: string | undefined | null, icon?: HTMLElement) =>
      value ? el("div", { class: "graph-panel-row" }, [el("span", { class: "graph-panel-label", text: label }), icon ?? null, el("span", { text: value })]) : null;

    const fileRow =
      node.file && el("div", { class: "graph-panel-row" }, [
        el("span", { class: "graph-panel-label", text: t.graphFile }),
        this.sourceLink(node.file, node.line ?? 1, node.name ?? node.id),
      ]);

    const byKind = new Map<string, GraphEdge[]>();
    for (const e of edges) {
      if (e.from !== node.id && e.to !== node.id) continue;
      if (CALL_KINDS.includes(e.kind)) continue; // shown apart, grouped by the other type
      (byKind.get(e.kind) ?? byKind.set(e.kind, []).get(e.kind)!).push(e);
    }
    const edgeGroups = [...byKind.entries()].map(([kind, list]) =>
      el("details", { attrs: { open: "" } }, [
        el("summary", {}, [kindIconEl("edge", kind), ` ${kind} (${list.length})`]),
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
      el("h2", {}, [kindIconEl("symbol", node.nativeKind ?? node.kind), ` ${node.name ?? node.id}`]),
      row(t.graphKind, node.kind, kindIconEl("symbol", node.kind)),
      row(t.graphNativeKind, node.nativeKind, kindIconEl("symbol", node.nativeKind)),
      row(t.graphModifiers, node.modifiers?.join(", ")),
      row(t.graphVisibility, node.visibility, kindIconEl("visibility", node.visibility)),
      fileRow || null,
      row(t.graphEntity, node.entity),
      row(t.graphContainers, node.containers?.join(", ")),
      row(
        t.graphPresence,
        node.presence === "code" ? t.graphPresenceCode : node.presence === "model" ? t.graphPresenceModel : t.graphPresenceBoth,
        kindIconEl("presence", node.presence),
      ),
      edgeGroups.length ? el("h3", { text: t.graphEdgesOf }) : null,
      ...edgeGroups,
      ...this.callSections(node, edges, nodeById),
      this.dynamicSection(node),
    );
  }

  /** Outgoing and incoming calls/constructs, one row per other type: ×N and
   * the methods on each side (lift=types folds method edges into type edges). */
  private callSections(node: GraphNode, edges: readonly GraphEdge[], nodeById: Map<string, GraphNode>): HTMLElement[] {
    const section = (title: string, outgoing: boolean): HTMLElement | null => {
      const rows = edges
        .filter((e) => CALL_KINDS.includes(e.kind) && (outgoing ? e.from === node.id : e.to === node.id))
        .map((e) => ({ e, other: nodeById.get(outgoing ? e.to : e.from) }))
        .sort((a, b) => (b.e.count ?? 1) - (a.e.count ?? 1) || (a.other?.name ?? "").localeCompare(b.other?.name ?? ""));
      if (rows.length === 0) return null;
      return el("details", { attrs: { open: "" } }, [
        el("summary", {}, [kindIconEl("edge", "calls"), ` ${title} (${rows.length})`]),
        el(
          "ul",
          { class: "graph-panel-edges graph-panel-calls" },
          rows.map(({ e, other }) => {
            const mine = (outgoing ? e.fromMethods : e.toMethods) ?? [];
            const theirs = (outgoing ? e.toMethods : e.fromMethods) ?? [];
            const head = `${outgoing ? "→" : "←"} ${other?.name ?? (outgoing ? e.to : e.from)}${e.count && e.count > 1 ? ` ×${e.count}` : ""}${e.kind === "calls" ? "" : ` · ${e.kind}`}`;
            const methods = mine.length || theirs.length ? `${mine.join(", ")} ${outgoing ? "→" : "←"} ${theirs.join(", ")}` : "";
            return el("li", {}, [el("div", { text: head }), methods ? el("div", { class: "graph-panel-methods", text: methods }) : null]);
          }),
        ),
      ]);
    };
    const out = section(t.graphCallsOut, true);
    const inn = section(t.graphCallsIn, false);
    return [out, inn].filter((s): s is HTMLElement => s !== null);
  }

  private dynamicSection(node: GraphNode): HTMLElement | null {
    if (!node.dynamic?.length) return null;
    return el("details", { attrs: { open: "" } }, [
      el("summary", { text: `${t.graphDynamicMarks} (${node.dynamic.length})` }),
      el(
        "ul",
        { class: "graph-panel-edges" },
        node.dynamic.map((d) => el("li", { text: `${d.kind} @${d.line}${d.method ? ` — ${d.method}` : ""}${d.file ? ` (${d.file})` : ""}` })),
      ),
    ]);
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
