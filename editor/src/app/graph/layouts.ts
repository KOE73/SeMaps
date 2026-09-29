import type Graph from "graphology";
import forceAtlas2 from "graphology-layout-forceatlas2";
import FA2Layout from "graphology-layout-forceatlas2/worker.js";
import louvain from "graphology-communities-louvain";
import noverlap from "graphology-layout-noverlap";
import circlepack from "graphology-layout/circlepack";
import circular from "graphology-layout/circular";
import random from "graphology-layout/random";
import * as dagre from "@dagrejs/dagre";
import type { GraphNode } from "./types.js";

export type LayoutKind =
  | "force"
  | "folder"
  | "namespace"
  | "community"
  | "container"
  | "hierarchy"
  | "radial"
  | "circlepack"
  | "circular"
  | "random";

function dirOf(file: string | undefined): string {
  if (!file) return "";
  const i = file.replace(/\\/g, "/").lastIndexOf("/");
  return i < 0 ? "" : file.slice(0, i);
}

/** The grouping key of a node for a grouped layout; "" groups ungrouped
 * nodes together rather than dropping them. */
export function groupKey(n: GraphNode, kind: "folder" | "namespace" | "container"): string {
  if (kind === "folder") return dirOf(n.file);
  if (kind === "namespace") return n.namespace ?? "";
  return n.containers?.[0] ?? "";
}

/** Runs Louvain on the current graph and returns node id -> community id. */
export function communityGroups(graph: Graph): Map<string, string> {
  const communities = louvain(graph, { resolution: 1 });
  const out = new Map<string, string>();
  for (const [node, c] of Object.entries(communities)) out.set(node, String(c));
  return out;
}

/** Scatters nodes on a large circle so a layout never starts them stacked at
 * the origin (the probe: nodes need initial coordinates before first
 * render). Deterministic by node order, not random, so re-entering the mode
 * looks the same before a layout runs. */
export function seedCircle(graph: Graph): void {
  const nodes = graph.nodes();
  const n = Math.max(1, nodes.length);
  const r = 10 + Math.sqrt(n) * 6;
  nodes.forEach((node, i) => {
    const a = (i / n) * Math.PI * 2;
    graph.setNodeAttribute(node, "x", r * Math.cos(a));
    graph.setNodeAttribute(node, "y", r * Math.sin(a));
  });
}

/** Places nodes group by group: groups on a circle sized by group count,
 * each group's members on a small circle sized by its member count, then a
 * global noverlap pass so labels and node bodies stop touching. */
export function applyGroupedLayout(graph: Graph, groupOf: Map<string, string>): void {
  const byGroup = new Map<string, string[]>();
  for (const node of graph.nodes()) {
    const g = groupOf.get(node) ?? "";
    (byGroup.get(g) ?? byGroup.set(g, []).get(g)!).push(node);
  }
  const groups = [...byGroup.entries()];
  const groupCount = Math.max(1, groups.length);
  const groupRingR = 60 + groupCount * 22;

  groups.forEach(([, members], gi) => {
    const angle = (gi / groupCount) * Math.PI * 2;
    const cx = groupRingR * Math.cos(angle);
    const cy = groupRingR * Math.sin(angle);
    const memberR = 6 + Math.sqrt(members.length) * 5;
    members.forEach((node, mi) => {
      const a = (mi / Math.max(1, members.length)) * Math.PI * 2;
      graph.setNodeAttribute(node, "x", cx + memberR * Math.cos(a));
      graph.setNodeAttribute(node, "y", cy + memberR * Math.sin(a));
    });
  });

  noverlap.assign(graph, {
    maxIterations: 200,
    settings: { margin: 4, ratio: 1 },
  });
}

const INHERITANCE = ["implements", "extends"];

/** Layers by inheritance: bases on top, their children below (an edge runs
 * child -> base). Uses dagre — synchronous, ~40 kB, made for layered DAGs,
 * where elkjs would add ~1.4 MB for nothing this needs. Sigma's y axis points
 * up, hence the sign. Nodes with no inheritance edge go in a grid below. */
export function hierarchyLayout(graph: Graph): void {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: "TB", nodesep: 10, ranksep: 28, marginx: 0, marginy: 0 });
  g.setDefaultEdgeLabel(() => ({}));
  const inTree = new Set<string>();
  graph.forEachEdge((_e, attrs, from, to) => {
    if (!INHERITANCE.includes(attrs.kind as string) || from === to) return;
    for (const id of [from, to]) {
      if (inTree.has(id)) continue;
      inTree.add(id);
      g.setNode(id, { width: 14 + (graph.getNodeAttribute(id, "size") as number) * 1.5, height: 10 });
    }
    g.setEdge(to, from); // base above child
  });

  let bottom = 0;
  if (inTree.size > 0) {
    dagre.layout(g);
    for (const id of inTree) {
      const p = g.node(id);
      graph.setNodeAttribute(id, "x", p.x);
      graph.setNodeAttribute(id, "y", -p.y);
      bottom = Math.min(bottom, -p.y);
    }
  }

  const rest = graph.nodes().filter((id) => !inTree.has(id));
  const cols = Math.max(6, Math.ceil(Math.sqrt(rest.length * 2)));
  const startY = inTree.size > 0 ? bottom - 50 : 0;
  rest.forEach((id, i) => {
    graph.setNodeAttribute(id, "x", (i % cols) * 22);
    graph.setNodeAttribute(id, "y", startY - Math.floor(i / cols) * 22);
  });
}

/** Rings by breadth-first distance from `center` over edges of the given
 * kinds (both directions); what cannot be reached sits on the outermost ring.
 * Within a ring nodes follow the angle of the node they were reached from,
 * which keeps most edges short. */
export function radialLayout(graph: Graph, center: string, kinds: ReadonlySet<string>): void {
  const adj = new Map<string, string[]>();
  graph.forEachEdge((_e, attrs, a, b) => {
    if (!kinds.has(attrs.kind as string)) return;
    (adj.get(a) ?? adj.set(a, []).get(a)!).push(b);
    (adj.get(b) ?? adj.set(b, []).get(b)!).push(a);
  });
  const rings: string[][] = [[center]];
  const parent = new Map<string, string>();
  const seen = new Set([center]);
  for (let frontier = [center]; frontier.length > 0; ) {
    const next: string[] = [];
    for (const id of frontier) {
      for (const n of adj.get(id) ?? []) {
        if (seen.has(n)) continue;
        seen.add(n);
        parent.set(n, id);
        next.push(n);
      }
    }
    if (next.length > 0) rings.push(next);
    frontier = next;
  }
  const unreached = graph.nodes().filter((id) => !seen.has(id));
  if (unreached.length > 0) rings.push(unreached);

  const angle = new Map<string, number>();
  let radius = 0;
  rings.forEach((ring, d) => {
    if (d === 0) {
      graph.setNodeAttribute(center, "x", 0);
      graph.setNodeAttribute(center, "y", 0);
      angle.set(center, 0);
      return;
    }
    radius = Math.max(radius + 26, (ring.length * 14) / (2 * Math.PI));
    const ordered = [...ring].sort((a, b) => (angle.get(parent.get(a) ?? "") ?? 0) - (angle.get(parent.get(b) ?? "") ?? 0));
    ordered.forEach((id, i) => {
      const a = (i / ordered.length) * Math.PI * 2;
      angle.set(id, a);
      graph.setNodeAttribute(id, "x", radius * Math.cos(a));
      graph.setNodeAttribute(id, "y", radius * Math.sin(a));
    });
  });
}

/** Circles nested folder -> namespace (graphology-layout's circlepack); node
 * sizes are the leaf radii. */
export function circlePackLayout(graph: Graph, hierarchyOf: (id: string) => [string, string]): void {
  graph.forEachNode((id) => {
    const [folder, namespace] = hierarchyOf(id);
    graph.setNodeAttribute(id, "folder", folder || "—");
    graph.setNodeAttribute(id, "namespace", namespace || "—");
  });
  circlepack.assign(graph, { hierarchyAttributes: ["folder", "namespace"] });
}

export function circularLayout(graph: Graph): void {
  circular.assign(graph, { scale: Math.max(40, graph.order * 1.4) });
}

export function randomLayout(graph: Graph): void {
  random.assign(graph, { scale: 20 + Math.sqrt(graph.order) * 12 });
}

export { dirOf };

/** Runs ForceAtlas2 in its web worker and stops it on its own: the probe
 * showed defaults never settle within 5s, so this polls average per-node
 * displacement and stops once it drops below a threshold, with a hard time
 * cap. `onDone` fires once, whichever criterion stops it first. */
export function runForceLayout(graph: Graph, onDone: () => void, onTick?: () => void): { stop: () => void } {
  const settings = forceAtlas2.inferSettings(graph);
  const layout = new FA2Layout(graph, { settings });
  const start = performance.now();
  const HARD_CAP_MS = 4000;
  const SETTLE_BELOW = 0.001;
  let last = new Map<string, [number, number]>();
  let stopped = false;

  const finish = () => {
    if (stopped) return;
    stopped = true;
    clearInterval(poll);
    layout.stop();
    layout.kill();
    onTick?.();
    onDone();
  };

  layout.start();
  const poll = setInterval(() => {
    onTick?.();
    if (performance.now() - start > HARD_CAP_MS) {
      finish();
      return;
    }
    let total = 0;
    let count = 0;
    graph.forEachNode((node, attrs) => {
      const prev = last.get(node);
      if (prev) {
        total += Math.hypot(attrs.x - prev[0], attrs.y - prev[1]);
        count++;
      }
      last.set(node, [attrs.x, attrs.y]);
    });
    if (count > 0 && total / count < SETTLE_BELOW) finish();
  }, 150);

  return { stop: finish };
}
