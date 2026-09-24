#!/usr/bin/env node
/**
 * Self-check for the edge routers and the path search behind them
 * (`src/canvas/routing/`). No test runner in this package — same harness as
 * `check-content-template.mjs`: bundle with esbuild, then run a table of cases.
 *
 * Three properties are checked, in order of how badly their absence shows:
 *
 * 1. **No NaN in a path.** A NaN does not fail loudly; the line simply
 *    vanishes, which reads as "the relation is gone", not "the router broke".
 * 2. **Square ends.** The first and last segment must run along the port's own
 *    normal. This is the invariant the whole grid construction exists to
 *    guarantee, so it is worth checking from the outside.
 * 3. **Solid zones are not entered.** A route may cost more; it may not cheat.
 */

import { build } from "esbuild";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const ROOT = new URL("..", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

async function load(entry, prefix) {
  const result = await build({
    entryPoints: [join(ROOT, entry)],
    bundle: true,
    write: false,
    format: "esm",
    platform: "neutral",
    target: "es2022",
  });
  const dir = mkdtempSync(join(tmpdir(), prefix));
  const file = join(dir, "bundle.mjs");
  writeFileSync(file, result.outputFiles[0].text, "utf8");
  return import(pathToFileURL(file).href);
}

const { OrthogonalRouter, TreeHorizontalRouter, TreeVerticalRouter } =
  await load("src/canvas/routing/routers.ts", "semaps-routers-");
const { solidZone, borderZones, laneZones } = await load("src/canvas/routing/Scene.ts", "semaps-scene-");

const orthogonal = new OrthogonalRouter();
const routers = [orthogonal, new TreeHorizontalRouter(), new TreeVerticalRouter()];

let failures = 0;
function check(name, ok, detail = "") {
  if (!ok) failures++;
  console.log(`[${ok ? "ok" : "FAIL"}] ${name}${ok || detail === "" ? "" : ` — ${detail}`}`);
}

const finitePath = (route) =>
  !route.path.includes("NaN") &&
  Number.isFinite(route.labelAt.x) &&
  Number.isFinite(route.labelAt.y) &&
  (route.points ?? []).every((p) => Number.isFinite(p.x) && Number.isFinite(p.y));

/** Does the segment leaving `points[0]` run along the port's normal? */
function squareEnds(route, fromSide, toSide) {
  const pts = route.points;
  if (!pts || pts.length < 2) return true;
  const axisOf = (side) => (side === "east" || side === "west" ? "y" : "x");
  const first = Math.abs(pts[0][axisOf(fromSide)] - pts[1][axisOf(fromSide)]) < 0.01;
  const n = pts.length;
  const last = Math.abs(pts[n - 1][axisOf(toSide)] - pts[n - 2][axisOf(toSide)]) < 0.01;
  return first && last;
}

function entersRect(points, rect) {
  if (!points) return false;
  const inside = (p) =>
    p.x > rect.x + 0.5 && p.x < rect.x + rect.width - 0.5 &&
    p.y > rect.y + 0.5 && p.y < rect.y + rect.height - 0.5;
  for (let i = 0; i < points.length - 1; i++) {
    const a = points[i];
    const b = points[i + 1];
    const steps = 24;
    for (let s = 0; s <= steps; s++) {
      const t = s / steps;
      if (inside({ x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t })) return true;
    }
  }
  return false;
}

// ------------------------------------------------------------ degenerate input

const degenerate = [
  {
    name: "coincident points",
    req: { from: { x: 50, y: 50 }, to: { x: 50, y: 50 }, fromSide: "east", toSide: "west",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 0, y: 0, width: 100, height: 100 } },
  },
  {
    name: "overlapping rects",
    req: { from: { x: 100, y: 50 }, to: { x: 20, y: 50 }, fromSide: "east", toSide: "west",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 20, y: 20, width: 100, height: 100 } },
  },
  {
    name: "same side both ports",
    req: { from: { x: 100, y: 50 }, to: { x: 100, y: 150 }, fromSide: "east", toSide: "east",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 0, y: 100, width: 100, height: 100 } },
  },
  {
    name: "level ports",
    req: { from: { x: 100, y: 50 }, to: { x: 300, y: 50 }, fromSide: "east", toSide: "west",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 300, y: 0, width: 100, height: 100 } },
  },
  {
    name: "vertical, same x",
    req: { from: { x: 50, y: 100 }, to: { x: 50, y: 300 }, fromSide: "south", toSide: "north",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 0, y: 300, width: 100, height: 100 } },
  },
  {
    name: "marker offsets and insets",
    req: { from: { x: 100, y: 50 }, to: { x: 300, y: 250 }, fromSide: "east", toSide: "west",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 300, y: 200, width: 100, height: 100 },
      fromInset: 8, toInset: 8, fromMarkerOffset: 10, toMarkerOffset: 12 },
  },
  {
    name: "non-finite input",
    req: { from: { x: NaN, y: 50 }, to: { x: 300, y: 50 }, fromSide: "east", toSide: "west",
      fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 300, y: 0, width: 100, height: 100 } },
  },
];

for (const router of routers) {
  for (const c of degenerate) {
    const route = router.route(c.req);
    check(`${router.id} :: ${c.name} :: finite`, finitePath(route), route.path);
    check(
      `${router.id} :: ${c.name} :: square ends`,
      squareEnds(route, c.req.fromSide, c.req.toSide),
      JSON.stringify(route.points),
    );
  }
}

// ------------------------------------------------------------------- the scene

const wall = { x: 160, y: 0, width: 80, height: 200 };
const between = {
  from: { x: 100, y: 100 }, to: { x: 400, y: 100 }, fromSide: "east", toSide: "west",
  fromRect: { x: 0, y: 60, width: 100, height: 80 },
  toRect: { x: 400, y: 60, width: 100, height: 80 },
  zones: [solidZone(wall, "blocker")],
};

const around = orthogonal.route(between);
check("orthogonal :: goes around a solid zone", !entersRect(around.points, wall), JSON.stringify(around.points));
check("orthogonal :: still square after detour", squareEnds(around, "east", "west"));
check("orthogonal :: finite after detour", finitePath(around));

const straight = orthogonal.route({ ...between, zones: [] });
check(
  "orthogonal :: no detour when nothing is in the way",
  (straight.points ?? []).length <= 3,
  JSON.stringify(straight.points),
);

// A container band is expensive to travel along, so a route with room to spare
// should not spend its length inside one.
const band = borderZones({ x: 150, y: 90, width: 300, height: 200 }, "zone");
const hugging = orthogonal.route({
  from: { x: 100, y: 95 }, to: { x: 600, y: 95 }, fromSide: "east", toSide: "west",
  fromRect: { x: 0, y: 60, width: 100, height: 70 },
  toRect: { x: 600, y: 60, width: 100, height: 70 },
  zones: band,
});
check("orthogonal :: finite across a border band", finitePath(hugging));
check("orthogonal :: square ends across a border band", squareEnds(hugging, "east", "west"));

// ------------------------------------------------------------------- stability

const first = orthogonal.route(between);
const second = orthogonal.route(between);
check(
  "orthogonal :: deterministic",
  JSON.stringify(first.points) === JSON.stringify(second.points),
);

const nudged = orthogonal.route({
  ...between,
  from: { x: 100, y: 101 },
  toRect: { x: 400, y: 61, width: 100, height: 80 },
});
check(
  "orthogonal :: a one-pixel move does not reshape the route",
  (nudged.points ?? []).length === (first.points ?? []).length,
  `${(first.points ?? []).length} → ${(nudged.points ?? []).length}`,
);

// --------------------------------------------------------------- sliding ends
//
// Ends may slide along their side (rectangles): the search then picks the
// ports together with the path. The cases are taken from a real view.

const top = { x: 0, y: 0, width: 1000, height: 80 };
const blockA = { x: 70, y: 390, width: 200, height: 50 };
const blockB = { x: 105, y: 235, width: 155, height: 53 };
const lower = { x: 88, y: 550, width: 290, height: 50 };
const upSlide = (rect) => ({ lo: rect.x + 14, hi: rect.x + rect.width - 14 });
const upward = {
  from: { x: 233, y: 550 }, to: { x: 233, y: 80 }, fromSide: "north", toSide: "south",
  fromRect: lower, toRect: top,
  zones: [solidZone(blockA, "a"), solidZone(blockB, "b")],
};

const fixed = orthogonal.route(upward);
const slid = orthogonal.route({ ...upward, fromSlide: upSlide(lower), toSlide: upSlide(top) });
check("sliding :: a straight corridor gives a straight line", (slid.points ?? []).length === 2, JSON.stringify(slid.points));
check("sliding :: fewer bends than with fixed ports", (slid.points ?? []).length < (fixed.points ?? []).length,
  `${(fixed.points ?? []).length} → ${(slid.points ?? []).length}`);
check("sliding :: goes past, not through", !entersRect(slid.points, blockA) && !entersRect(slid.points, blockB), JSON.stringify(slid.points));
check("sliding :: square ends", squareEnds(slid, "north", "south"));
check("sliding :: ends stay on their sides",
  slid.points[0].y === 550 && slid.points.at(-1).y === 80 &&
  slid.points[0].x >= lower.x && slid.points[0].x <= lower.x + lower.width, JSON.stringify(slid.points));

// No corridor at the start (a block right above covers the whole side), but
// the end is free: the line dodges once and arrives head-on, no jog at the top.
const small = { x: 67, y: 460, width: 155, height: 50 };
const cover = { x: 60, y: 340, width: 175, height: 50 };
const blocked = orthogonal.route({
  from: { x: 144, y: 460 }, to: { x: 500, y: 80 }, fromSide: "north", toSide: "south",
  fromRect: small, toRect: top, zones: [solidZone(cover, "cover")],
  fromSlide: upSlide(small), toSlide: upSlide(top),
});
const bp = blocked.points ?? [];
check("sliding :: blocked start still arrives straight", bp.length >= 2 && bp.at(-1).x === bp.at(-2).x, JSON.stringify(bp));
check("sliding :: blocked start, at most two bends", bp.length <= 4, JSON.stringify(bp));
check("sliding :: blocked start does not cut through", !entersRect(bp, cover), JSON.stringify(bp));

// Two lines into the same side: the second takes another spot.
const one = orthogonal.route({ ...upward, fromSlide: upSlide(lower), toSlide: upSlide(top) });
const two = orthogonal.route({
  ...upward, fromSlide: upSlide(lower),
  toSlide: { ...upSlide(top), taken: [one.points.at(-1).x] },
});
check("sliding :: a taken spot is not reused", Math.abs(one.points.at(-1).x - two.points.at(-1).x) >= 12,
  `${one.points.at(-1).x} / ${two.points.at(-1).x}`);

// Nothing to gain: the assigned ports stay where they were.
const plain = {
  from: { x: 50, y: 100 }, to: { x: 50, y: 300 }, fromSide: "south", toSide: "north",
  fromRect: { x: 0, y: 0, width: 100, height: 100 }, toRect: { x: 0, y: 300, width: 100, height: 100 }, zones: [],
};
const plainSlid = orthogonal.route({ ...plain, fromSlide: { lo: 14, hi: 86 }, toSlide: { lo: 14, hi: 86 } });
check("sliding :: nothing in the way, ports unchanged",
  JSON.stringify(plainSlid.points) === JSON.stringify(orthogonal.route(plain).points), JSON.stringify(plainSlid.points));

// A line already drawn is a lane: the next one runs beside it, not on top.
// The real case: Model_Inference goes straight up the only corridor left of
// Op_Track; Model_YoloSeg sits under Op_Track and has to dodge into that
// same corridor.
const inferBox = { x: 10, y: 700, width: 250, height: 50 };
const segBox = { x: 95, y: 550, width: 180, height: 50 };
const trackBox = { x: 80, y: 400, width: 220, height: 50 };
const firstLine = orthogonal.route({
  from: { x: 135, y: 700 }, to: { x: 135, y: 80 }, fromSide: "north", toSide: "south",
  fromRect: inferBox, toRect: top, zones: [solidZone(segBox, "seg"), solidZone(trackBox, "track")],
  fromSlide: upSlide(inferBox), toSlide: upSlide(top),
});
const secondReq = {
  from: { x: 185, y: 550 }, to: { x: 185, y: 80 }, fromSide: "north", toSide: "south",
  fromRect: segBox, toRect: top,
  fromSlide: upSlide(segBox), toSlide: upSlide(top),
};
const secondBlind = orthogonal.route({ ...secondReq, zones: [solidZone(trackBox, "track"), solidZone(inferBox, "infer")] });
const secondLine = orthogonal.route({
  ...secondReq,
  zones: [solidZone(trackBox, "track"), solidZone(inferBox, "infer"), ...laneZones(firstLine.points, "lane:first")],
});
const collinear = (a, b) => {
  for (let i = 0; i < a.length - 1; i++) for (let j = 0; j < b.length - 1; j++) {
    const [p, q, r, t] = [a[i], a[i + 1], b[j], b[j + 1]];
    if (p.x === q.x && r.x === t.x && Math.abs(p.x - r.x) < 5 &&
        Math.min(Math.max(p.y, q.y), Math.max(r.y, t.y)) - Math.max(Math.min(p.y, q.y), Math.min(r.y, t.y)) > 20) return true;
    if (p.y === q.y && r.y === t.y && Math.abs(p.y - r.y) < 5 &&
        Math.min(Math.max(p.x, q.x), Math.max(r.x, t.x)) - Math.max(Math.min(p.x, q.x), Math.min(r.x, t.x)) > 20) return true;
  }
  return false;
};
check("lanes :: the case is real (without lanes they merge)", collinear(firstLine.points, secondBlind.points),
  `${JSON.stringify(firstLine.points)} / ${JSON.stringify(secondBlind.points)}`);
check("lanes :: with lanes the second runs beside the first",
  !collinear(firstLine.points, secondLine.points), `${JSON.stringify(firstLine.points)} / ${JSON.stringify(secondLine.points)}`);
check("lanes :: and still clear of the blocks",
  !entersRect(secondLine.points, trackBox) && !entersRect(secondLine.points, inferBox), JSON.stringify(secondLine.points));

const again = orthogonal.route({ ...upward, fromSlide: upSlide(lower), toSlide: upSlide(top) });
check("sliding :: deterministic", JSON.stringify(again.points) === JSON.stringify(slid.points));

console.log(failures === 0 ? "\nВсе случаи прошли." : `\n${failures} провалов`);
process.exit(failures === 0 ? 0 : 1);
