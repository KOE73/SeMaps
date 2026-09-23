#!/usr/bin/env node
/**
 * Self-check for the route-nudging pass (`src/canvas/routing/nudge.ts`).
 * Same esbuild-bundle-then-run harness as `check-routers.mjs` — no test
 * runner in this package.
 */

import { build } from "esbuild";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const ROOT = new URL("..", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

async function loadModule() {
  const result = await build({
    entryPoints: [join(ROOT, "src/canvas/routing/nudge.ts")],
    bundle: true,
    write: false,
    format: "esm",
    platform: "neutral",
    target: "es2022",
  });
  const dir = mkdtempSync(join(tmpdir(), "semaps-nudge-"));
  const file = join(dir, "nudge.mjs");
  writeFileSync(file, result.outputFiles[0].text, "utf8");
  return import(pathToFileURL(file).href);
}

const { nudgeRoutes } = await loadModule();

let failures = 0;
function fail(name, detail) {
  failures++;
  console.log(`[FAIL] ${name} :: ${detail}`);
}
function ok(name) {
  console.log(`[ok] ${name}`);
}

function pointsFinite(points) {
  return points.every((p) => Number.isFinite(p.x) && Number.isFinite(p.y));
}

function segAxis(a, b) {
  if (Math.abs(a.y - b.y) < 1e-9) return "y"; // horizontal segment, constant y
  if (Math.abs(a.x - b.x) < 1e-9) return "x"; // vertical segment, constant x
  return null; // not axis-aligned
}

function isOrthogonal(points) {
  for (let i = 0; i + 1 < points.length; i++) {
    if (segAxis(points[i], points[i + 1]) === null) return false;
  }
  return true;
}

// ---------------------------------------------------------------- case 1
// Two coincident horizontal routes must separate into two distinct lanes.
{
  const routes = [
    { id: "a", points: [{ x: 0, y: 100 }, { x: 50, y: 100 }, { x: 50, y: 300 }, { x: 200, y: 300 }, { x: 200, y: 400 }] },
    { id: "b", points: [{ x: 0, y: 150 }, { x: 50, y: 150 }, { x: 50, y: 300 }, { x: 200, y: 300 }, { x: 200, y: 450 }] },
  ];
  // The middle segment of both routes ([1]-[2], vertical at x=50, spans
  // y 100..300 and y 150..300 respectively) overlaps: same corridor.
  const result = nudgeRoutes({ routes, walls: [], gap: 20 });
  const a = result.get("a");
  const b = result.get("b");
  if (!a || !b) {
    fail("two coincident verticals separate", "missing route in result");
  } else {
    const ax = a[1].x;
    const bx = b[1].x;
    if (Math.abs(ax - bx) < 1) fail("two coincident verticals separate", `lanes not separated: ${ax} vs ${bx}`);
    else if (!pointsFinite(a) || !pointsFinite(b)) fail("two coincident verticals separate", "NaN in output");
    else if (!isOrthogonal(a) || !isOrthogonal(b)) fail("two coincident verticals separate", "polyline not orthogonal after nudge");
    else ok("two coincident verticals separate into distinct lanes");
  }
}

// ---------------------------------------------------------------- case 2
// Three routes sharing one horizontal corridor get three distinct lanes.
{
  const routes = [
    { id: "r1", points: [{ x: 0, y: 0 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 0 }] },
    { id: "r2", points: [{ x: 0, y: 10 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 20 }] },
    { id: "r3", points: [{ x: 0, y: 20 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 40 }] },
  ];
  const result = nudgeRoutes({ routes, walls: [], gap: 15 });
  const ys = routes.map((r) => result.get(r.id)?.[1]?.y);
  const uniq = new Set(ys.map((y) => Math.round(y)));
  if (ys.some((y) => y === undefined || !Number.isFinite(y))) {
    fail("three lanes in one corridor", "missing/NaN y");
  } else if (uniq.size !== 3) {
    fail("three lanes in one corridor", `expected 3 distinct lanes, got ${[...uniq]}`);
  } else {
    ok("three routes in one corridor get three distinct lanes");
  }
}

// ---------------------------------------------------------------- case 3
// A lane must not be pushed into a wall when the interval is narrow.
{
  const routes = [
    { id: "a", points: [{ x: 0, y: 100 }, { x: 0, y: 148 }, { x: 200, y: 148 }, { x: 200, y: 100 }] },
    { id: "b", points: [{ x: 0, y: 100 }, { x: 0, y: 152 }, { x: 200, y: 152 }, { x: 200, y: 100 }] },
  ];
  // Corridor is the horizontal segment near y=150, spanning x 0..200.
  // Walls pinch the free interval to y in [140, 160] — narrower than the
  // ideal gap*1 = 40 spread, so lanes must be compressed but stay in-band.
  const walls = [
    { x: 50, y: 0, width: 100, height: 140 }, // occupies y<140 in that x-range... but we need a wall bounding from below at y=140 within span
    { x: 50, y: 160, width: 100, height: 40 },
  ];
  const result = nudgeRoutes({ routes, walls, gap: 40 });
  const a = result.get("a");
  const b = result.get("b");
  const ay = a?.[1]?.y;
  const by = b?.[1]?.y;
  if (ay === undefined || by === undefined || !Number.isFinite(ay) || !Number.isFinite(by)) {
    fail("lane stays out of wall", "missing/NaN y");
  } else if (ay < 140 || ay > 160 || by < 140 || by > 160) {
    fail("lane stays out of wall", `lane escaped bounded interval: ${ay}, ${by}`);
  } else {
    ok("lane compressed to fit between walls instead of crossing them");
  }
}

// ---------------------------------------------------------------- case 4
// Stubs (first and last segment) never move.
{
  const routes = [
    { id: "a", points: [{ x: 0, y: 100 }, { x: 50, y: 100 }, { x: 50, y: 300 }, { x: 200, y: 300 }, { x: 200, y: 400 }] },
    { id: "b", points: [{ x: 0, y: 150 }, { x: 50, y: 150 }, { x: 50, y: 300 }, { x: 200, y: 300 }, { x: 200, y: 450 }] },
  ];
  const result = nudgeRoutes({ routes, walls: [], gap: 20 });
  const a = result.get("a");
  const b = result.get("b");
  const stubsUnchanged =
    a && b &&
    a[0].x === 0 && a[0].y === 100 && a[a.length - 1].x === 200 && a[a.length - 1].y === 400 &&
    b[0].x === 0 && b[0].y === 150 && b[b.length - 1].x === 200 && b[b.length - 1].y === 450;
  if (!stubsUnchanged) fail("stubs never move", "first/last point changed");
  else ok("first and last segment endpoints are untouched");
}

// ---------------------------------------------------------------- case 5
// Degenerate inputs never produce NaN.
{
  const degenerateCases = [
    { name: "two-point route", routes: [{ id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 0 }] }], walls: [], gap: 10 },
    { name: "zero-length segment", routes: [{ id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 0 }, { x: 0, y: 0 }, { x: 10, y: 0 }] }], walls: [], gap: 10 },
    { name: "empty walls", routes: [{ id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 50 }, { x: 100, y: 50 }, { x: 100, y: 100 }] }], walls: [], gap: 10 },
    {
      name: "coincident routes",
      routes: [
        { id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 50 }, { x: 100, y: 50 }, { x: 100, y: 100 }] },
        { id: "b", points: [{ x: 0, y: 0 }, { x: 0, y: 50 }, { x: 100, y: 50 }, { x: 100, y: 100 }] },
      ],
      walls: [],
      gap: 10,
    },
    {
      name: "gap = 0",
      routes: [
        { id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 50 }, { x: 100, y: 50 }, { x: 100, y: 100 }] },
        { id: "b", points: [{ x: 0, y: 5 }, { x: 0, y: 50 }, { x: 100, y: 50 }, { x: 100, y: 105 }] },
      ],
      walls: [],
      gap: 0,
    },
    { name: "empty routes list", routes: [], walls: [{ x: 0, y: 0, width: 10, height: 10 }], gap: 10 },
    {
      name: "wall with non-finite fields",
      routes: [{ id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 50 }, { x: 100, y: 50 }, { x: 100, y: 100 }] }],
      walls: [{ x: NaN, y: 0, width: 10, height: 10 }],
      gap: 10,
    },
  ];
  let anyNaN = false;
  for (const c of degenerateCases) {
    const result = nudgeRoutes({ routes: c.routes, walls: c.walls, gap: c.gap });
    for (const [id, pts] of result) {
      if (!pointsFinite(pts)) {
        anyNaN = true;
        fail("degenerate inputs never NaN", `${c.name} :: route ${id} has non-finite point`);
      }
    }
  }
  if (!anyNaN) ok("degenerate/edge-case inputs all produce finite output");
}

// ---------------------------------------------------------------- case 6
// Idempotence: nudging an already-nudged set of routes again changes nothing.
{
  const routes = [
    { id: "r1", points: [{ x: 0, y: 0 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 0 }] },
    { id: "r2", points: [{ x: 0, y: 10 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 20 }] },
    { id: "r3", points: [{ x: 0, y: 20 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 40 }] },
  ];
  const walls = [{ x: -50, y: 300, width: 600, height: 50 }];
  const gap = 15;
  const first = nudgeRoutes({ routes, walls, gap });
  const firstAsRoutes = routes.map((r) => ({ id: r.id, points: first.get(r.id) ?? [] }));
  const second = nudgeRoutes({ routes: firstAsRoutes, walls, gap });

  let stable = true;
  for (const r of routes) {
    const a = first.get(r.id) ?? [];
    const b = second.get(r.id) ?? [];
    if (a.length !== b.length) { stable = false; break; }
    for (let i = 0; i < a.length; i++) {
      if (Math.abs(a[i].x - b[i].x) > 1e-6 || Math.abs(a[i].y - b[i].y) > 1e-6) { stable = false; break; }
    }
  }
  if (!stable) fail("idempotent on its own output", "second pass moved points that the first pass already settled");
  else ok("re-running on already-nudged routes is a no-op (idempotent)");
}

// ---------------------------------------------------------------- case 7
// Stable lane order regardless of input array order (no Map/Set iteration
// dependence): same routes, different array order, same lane assignment.
{
  const routeA = { id: "a", points: [{ x: 0, y: 0 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 0 }] };
  const routeB = { id: "b", points: [{ x: 0, y: 10 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 20 }] };
  const routeC = { id: "c", points: [{ x: 0, y: 20 }, { x: 0, y: 200 }, { x: 400, y: 200 }, { x: 400, y: 40 }] };
  const forward = nudgeRoutes({ routes: [routeA, routeB, routeC], walls: [], gap: 15 });
  const reversed = nudgeRoutes({ routes: [routeC, routeB, routeA], walls: [], gap: 15 });
  let same = true;
  for (const id of ["a", "b", "c"]) {
    const yf = forward.get(id)?.[1]?.y;
    const yr = reversed.get(id)?.[1]?.y;
    if (yf === undefined || yr === undefined || Math.abs(yf - yr) > 1e-6) { same = false; break; }
  }
  if (!same) fail("order-independent lane assignment", "input array order changed the result");
  else ok("lane assignment is independent of input array order");
}

console.log(failures === 0 ? "\nAll cases clean." : `\n${failures} FAILURES`);
process.exit(failures === 0 ? 0 : 1);
