import type Graph from "graphology";
import forceAtlas2 from "graphology-layout-forceatlas2";
import FA2Layout from "graphology-layout-forceatlas2/worker.js";
import louvain from "graphology-communities-louvain";
import noverlap from "graphology-layout-noverlap";
import type { GraphNode } from "./types.js";

export type LayoutKind = "force" | "folder" | "namespace" | "community" | "container";

function dirOf(file: string | undefined): string {
  if (!file) return "";
  const i = file.replace(/\\/g, "/").lastIndexOf("/");
  return i < 0 ? "" : file.slice(0, i);
}

/** The grouping key of a node for a grouped layout; "" groups ungrouped
 * nodes together rather than dropping them. */
export function groupKey(n: GraphNode, kind: Exclude<LayoutKind, "force" | "community">): string {
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

/** Runs ForceAtlas2 in its web worker and stops it on its own: the probe
 * showed defaults never settle within 5s, so this polls average per-node
 * displacement and stops once it drops below a threshold, with a hard time
 * cap. `onDone` fires once, whichever criterion stops it first. */
export function runForceLayout(graph: Graph, onDone: () => void): { stop: () => void } {
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
    onDone();
  };

  layout.start();
  const poll = setInterval(() => {
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
