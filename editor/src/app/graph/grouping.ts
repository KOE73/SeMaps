import type Graph from "graphology";
import { graphFilterConfig } from "./filterConfig.js";
import { communityGroups, dirOf, innermostContainer } from "./layouts.js";
import type { GraphNode, GroupsResponse } from "./types.js";
import type { GroupBy } from "./viewSettings.js";

/** The group of a node no view of the axis puts in a zone (drawn together, apart from the zones). */
export const OUTSIDE_ZONES = "\u0000outside";

export interface GroupingContext {
  nodeOf(id: string): GraphNode | undefined;
  /** Segments (namespace, folder) or nesting level (containers) that make one group. */
  depth: number;
  /** The axis `axis` reads; empty = the first the project has. */
  axis: string;
  groups: GroupsResponse | undefined;
  /** The edge kinds drawn now: the «связные компоненты» follow them. */
  drawnKinds: ReadonlySet<string>;
}

const segments = (path: string, sep: RegExp, depth: number): string => path.split(sep).filter(Boolean).slice(0, depth).join("/");

/** Namespaces are dotted, folders are slashed: how deep the data goes, for the depth selector. */
export function namespaceDepth(nodes: readonly GraphNode[]): number {
  return Math.max(1, ...nodes.map((n) => (n.namespace ? n.namespace.split(".").length : 0)));
}

export function folderDepth(nodes: readonly GraphNode[]): number {
  return Math.max(1, ...nodes.map((n) => dirOf(n.file).split("/").filter(Boolean).length));
}

/** Nesting of containers.json by `parent`: the longest chain. */
export function containerDepth(groups: GroupsResponse | undefined): number {
  if (!groups) return 1;
  const parent = new Map(groups.containers.map((c) => [c.id, c.parent]));
  let deepest = 1;
  for (const c of groups.containers) {
    let n = 1;
    const seen = new Set([c.id]);
    for (let p = c.parent; p && !seen.has(p); p = parent.get(p)) {
      seen.add(p);
      n++;
    }
    deepest = Math.max(deepest, n);
  }
  return deepest;
}

/**
 * node id -> group key, for the `grouped` layout. A node with no group for the
 * chosen criterion lands in the group "" (ungrouped nodes are kept together, not dropped).
 */
export function groupNodes(by: GroupBy, sub: Graph, ctx: GroupingContext): Map<string, string> {
  const ids = sub.nodes();
  const out = new Map<string, string>();
  const each = (key: (n: GraphNode, id: string) => string) => {
    for (const id of ids) {
      const n = ctx.nodeOf(id);
      out.set(id, n ? key(n, id) : "");
    }
    return out;
  };

  switch (by) {
    case "assembly":
      return each((n) => n.assembly ?? "");
    case "namespace":
      return each((n) => (n.namespace ?? "").split(".").filter(Boolean).slice(0, ctx.depth).join("."));
    case "folder":
      return each((n) => segments(dirOf(n.file), /\//, ctx.depth));
    case "containers": {
      // The innermost container of the node by the declared nesting, then up its parent chain to the wanted level.
      const parent = new Map((ctx.groups?.containers ?? []).map((c) => [c.id, c.parent]));
      return each((n) => {
        const own = innermostContainer(n, parent);
        if (!own) return "";
        const path = [own];
        for (let p = parent.get(own); p && !path.includes(p); p = parent.get(p)) path.unshift(p);
        return path[Math.min(ctx.depth, path.length) - 1] ?? "";
      });
    }
    case "axis": {
      // The zone a node's entity sits in on the views of the axis; not placed there: outside the frames.
      const axis = ctx.groups?.axes.find((a) => a.axis === ctx.axis) ?? ctx.groups?.axes[0];
      return each((n) => (n.entity ? axis?.of[n.entity] : undefined) ?? OUTSIDE_ZONES);
    }
    case "community":
      return communityGroups(sub);
    case "inheritance": {
      // The root base of the tree a node belongs to: follow its first base upwards.
      const bases = new Map<string, string[]>();
      const linked = new Set<string>();
      sub.forEachEdge((_e, attrs, from, to) => {
        if (!graphFilterConfig().hierarchyKinds.includes(attrs.kind as string) || from === to) return;
        (bases.get(from) ?? bases.set(from, []).get(from)!).push(to);
        linked.add(from).add(to);
      });
      const root = (id: string): string => {
        const seen = new Set([id]);
        let at = id;
        for (let next = [...(bases.get(at) ?? [])].sort()[0]; next && !seen.has(next); next = [...(bases.get(at) ?? [])].sort()[0]) {
          seen.add(next);
          at = next;
        }
        return at;
      };
      return each((_n, id) => (linked.has(id) ? root(id) : ""));
    }
    case "components": {
      const parent = new Map<string, string>(ids.map((id) => [id, id]));
      const find = (id: string): string => {
        let r = id;
        while (parent.get(r) !== r) r = parent.get(r)!;
        parent.set(id, r);
        return r;
      };
      sub.forEachEdge((_e, attrs, from, to) => {
        if (ctx.drawnKinds.has(attrs.kind as string)) parent.set(find(from), find(to));
      });
      return each((_n, id) => find(id));
    }
  }
}
