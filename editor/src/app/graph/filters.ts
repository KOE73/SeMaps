import { el } from "../../util/dom.js";
import { t } from "../../shell/strings.js";
import type { GraphNode } from "./types.js";

/**
 * Filter state and its UI. Filtering only hides nodes/edges already drawn —
 * it never refetches (PLAN_20260928-2 step 4).
 */
export interface FilterState {
  edgeKinds: Set<string>;
  symbolKinds: Set<string>;
  visibility: Set<string>;
  presence: Set<string>;
  container: string; // "" = any
}

export function allFilters(nodes: readonly GraphNode[], edgeKinds: readonly string[]): FilterState {
  return {
    edgeKinds: new Set(edgeKinds),
    symbolKinds: new Set(nodes.map((n) => n.kind).filter((k): k is string => !!k)),
    visibility: new Set(nodes.map((n) => n.visibility).filter((v): v is string => !!v)),
    presence: new Set(["code", "model", "both"]),
    container: "",
  };
}

function distinct<T>(values: Iterable<T | undefined>): T[] {
  return [...new Set([...values].filter((v): v is T => v !== undefined))];
}

export function nodeVisible(n: GraphNode, f: FilterState): boolean {
  if (!f.presence.has(n.presence)) return false;
  if (n.kind && !f.symbolKinds.has(n.kind)) return false;
  if (n.visibility && !f.visibility.has(n.visibility)) return false;
  if (f.container && !(n.containers ?? []).includes(f.container)) return false;
  return true;
}

export interface FiltersUi {
  readonly root: HTMLElement;
}

/** Builds the filters panel; calls `onChange` whenever a control changes. */
export function buildFilters(
  nodes: readonly GraphNode[],
  edgeKinds: readonly string[],
  state: FilterState,
  onChange: () => void,
): FiltersUi {
  const group = (title: string, values: readonly string[], set: Set<string>, labelOf: (v: string) => string = (v) => v) => {
    const boxes = values.map((v) => {
      const cb = el("input", { type: "checkbox" }) as HTMLInputElement;
      cb.checked = set.has(v);
      cb.addEventListener("change", () => {
        if (cb.checked) set.add(v);
        else set.delete(v);
        onChange();
      });
      return el("label", { class: "tool-check" }, [cb, labelOf(v)]);
    });
    return el("div", { class: "graph-filter-group" }, [el("h3", { text: title }), el("div", { class: "tool-checks" }, boxes)]);
  };

  const containers = distinct(nodes.flatMap((n) => n.containers ?? []));
  const containerSelect = el("select", {});
  containerSelect.appendChild(el("option", { value: "", text: t.graphAnyContainer }));
  for (const c of containers) containerSelect.appendChild(el("option", { value: c, text: c }));
  containerSelect.value = state.container;
  containerSelect.addEventListener("change", () => {
    state.container = containerSelect.value;
    onChange();
  });

  const root = el("div", { class: "graph-filters" }, [
    el("h2", { text: t.graphFilters }),
    group(t.graphFilterEdgeKinds, edgeKinds, state.edgeKinds),
    group(t.graphFilterSymbolKinds, distinct(nodes.map((n) => n.kind)), state.symbolKinds),
    group(t.graphFilterVisibility, distinct(nodes.map((n) => n.visibility)), state.visibility),
    group(t.graphFilterPresence, ["code", "model", "both"], state.presence, (v) =>
      v === "code" ? t.graphPresenceCode : v === "model" ? t.graphPresenceModel : t.graphPresenceBoth,
    ),
    el("div", { class: "graph-filter-group" }, [
      el("h3", { text: t.graphFilterContainer }),
      containerSelect,
    ]),
  ]);

  return { root };
}
