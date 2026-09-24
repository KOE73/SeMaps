import "../styles/editor.css";
import "../styles/shell.css";

import { el } from "../util/dom.js";
import { applySavedTheme, createTopBar } from "../shell/TopBar.js";
import { toolApi, type ExtractorView, type RunInfo, type Setup, type SyncResult } from "../shell/api.js";
import { fmt, t } from "../shell/strings.js";

/**
 * /extract: the extractors of the .semaps file. Per extractor: its
 * parameters, a run with a live log and statistics, the dry-run report of
 * that run, and writing exactly those facts to the registry
 * (ADR_20260924-3 §3).
 */

interface ProjectRef {
  id: string;
  title?: string;
}

let projects: ProjectRef[] = [];
let languages: string[] = [];

async function main(): Promise<void> {
  applySavedTheme();
  const root = document.getElementById("app")!;
  const bar = createTopBar("extract");
  const inner = el("div", { class: "tool-page-inner" }, [el("p", { class: "tool-muted", text: t.loading })]);
  root.append(bar.element, el("div", { class: "shell-body" }, [el("main", { class: "tool-page" }, [inner])]));

  const setup = await bar.setup;
  if (!setup) {
    inner.replaceChildren(el("p", { class: "tool-error", text: t.noProjectFile }));
    return;
  }
  projects = await fetch("/api/workspace", { cache: "no-store" })
    .then((r) => r.json() as Promise<{ projects?: ProjectRef[] }>)
    .then((w) => w.projects ?? [])
    .catch(() => []);
  render(inner, setup);
}

function render(inner: HTMLElement, setup: Setup): void {
  languages = setup.languages;
  const list = el("div", {});
  for (const e of setup.extractors) list.appendChild(extractorCard(e, inner));

  const newId = el("input", { placeholder: t.newId });
  const add = el("button", { class: "tool-btn", text: t.addExtractor });
  const addError = el("span", { class: "tool-error" });
  add.addEventListener("click", async () => {
    const id = newId.value.trim();
    if (!id) return;
    try {
      const next = await toolApi.saveExtractor(id, {
        language: languages[0] ?? "csharp",
        project: projects[0]?.id ?? "",
        root: ".",
        include: ["src"],
      });
      render(inner, next);
    } catch (e) {
      addError.textContent = (e as Error).message;
    }
  });

  inner.replaceChildren(
    el("h1", { text: t.extractTitle }),
    el("p", { class: "tool-hint", text: t.extractHint }),
    list,
    el("section", { class: "tool-card" }, [el("div", { class: "tool-new" }, [newId, add]), addError]),
  );
}

function extractorCard(e: ExtractorView, inner: HTMLElement): HTMLElement {
  const listValue = (v: string[]) => v.join(", ");
  const parseList = (s: string) =>
    s
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);

  const language = el("select", {});
  for (const l of new Set([...languages, e.language])) {
    const o = el("option", { text: l, value: l });
    if (l === e.language) o.selected = true;
    language.appendChild(o);
  }
  const project = el("select", {});
  for (const p of projects.some((p) => p.id === e.project) ? projects : [{ id: e.project }, ...projects]) {
    const o = el("option", { text: p.title && p.title !== p.id ? `${p.id} — ${p.title}` : p.id, value: p.id });
    if (p.id === e.project) o.selected = true;
    project.appendChild(o);
  }
  const root = el("input", { value: e.root, placeholder: "." });
  const include = el("input", { value: listValue(e.include), placeholder: t.listHint });
  const exclude = el("input", { value: listValue(e.exclude), placeholder: t.listHint });

  const edgeKinds = ["holds", "uses", "injects"];
  const edgeChecks: Record<string, HTMLInputElement> = {};
  const edgeCheckboxes: HTMLElement[] = [];
  for (const kind of edgeKinds) {
    const cb = el("input", { type: "checkbox" }) as HTMLInputElement;
    cb.checked = e.edges.includes(kind);
    edgeChecks[kind] = cb;
    edgeCheckboxes.push(el("label", { class: "tool-check" }, [cb, kind]));
  }

  const formRows = [
    el("label", { text: t.language }),
    language,
    el("label", { text: t.modelProject }),
    project,
    el("label", { text: t.root }),
    root,
    el("label", { text: t.include }),
    include,
    el("label", { text: t.exclude }),
    exclude,
    el("label", { text: t.edgeKinds }),
    el("div", { class: "tool-checks" }, edgeCheckboxes),
  ];
  if (e.command) {
    const cmd = el("input", { value: e.command });
    cmd.readOnly = true;
    formRows.push(el("label", { text: t.command }), cmd);
  }

  const status = el("span", { class: "tool-muted" });
  const save = el("button", { class: "tool-btn", text: t.save });
  save.addEventListener("click", async () => {
    save.disabled = true;
    try {
      const edges = edgeKinds.filter((k) => edgeChecks[k]?.checked);
      const next = await toolApi.saveExtractor(e.id, {
        language: language.value,
        project: project.value,
        root: root.value.trim(),
        include: parseList(include.value),
        exclude: parseList(exclude.value),
        edges: edges.length > 0 ? edges : undefined,
      });
      render(inner, next);
    } catch (err) {
      status.className = "tool-error";
      status.textContent = (err as Error).message;
      save.disabled = false;
    }
  });
  const remove = el("button", { class: "tool-btn is-danger", text: t.remove });
  remove.addEventListener("click", async () => {
    if (!confirm(fmt(t.confirmRemove, { id: e.id }))) return;
    try {
      render(inner, await toolApi.removeExtractor(e.id));
    } catch (err) {
      status.className = "tool-error";
      status.textContent = (err as Error).message;
    }
  });

  const tool = e.tool;
  const toolBadge = tool.found
    ? el("span", { class: "tool-badge is-ok", text: `${t.extractorFound}: ${tool.source}` })
    : el("span", { class: "tool-badge is-bad", text: t.extractorMissing });
  const runtimeBadge = tool.runtime
    ? el("span", {
        class: "tool-badge " + (tool.runtime.ok ? "is-ok" : "is-bad"),
        text: `${tool.runtime.name} ${tool.runtime.ok ? tool.runtime.version ?? "" : t.runtimeMissing}`,
      })
    : null;
  const problems = [tool.problem, tool.runtime && !tool.runtime.ok ? tool.runtime.hint : undefined].filter(Boolean);

  const runPanel = el("div", {});
  const extract = el("button", { class: "tool-btn is-primary", text: t.extract });
  extract.disabled = !tool.found || (tool.runtime !== undefined && !tool.runtime.ok);
  extract.addEventListener("click", async () => {
    extract.disabled = true;
    try {
      const run = await toolApi.startRun(e.id);
      await followRun(run, runPanel);
    } catch (err) {
      runPanel.replaceChildren(el("p", { class: "tool-error", text: (err as Error).message }));
    } finally {
      extract.disabled = false;
    }
  });
  if (e.lastRun) showRun(e.lastRun, runPanel, "");

  return el("section", { class: "tool-card" }, [
    el("div", { class: "tool-card-head" }, [
      el("h2", { text: e.id }),
      toolBadge,
      runtimeBadge,
      el("span", { class: "spacer" }),
      remove,
    ]),
    problems.length ? el("p", { class: "tool-error", text: problems.join("\n") }) : null,
    el("div", { class: "tool-form" }, formRows),
    el("div", { class: "tool-actions" }, [save, extract, status]),
    runPanel,
  ]);
}

/** Show the log as it grows, then the run's result. */
async function followRun(run: RunInfo, panel: HTMLElement): Promise<void> {
  const log = el("pre", { class: "tool-log", text: "" });
  panel.replaceChildren(el("p", { class: "tool-muted", text: t.running }), log);
  let offset = 0;
  for (;;) {
    const chunk = await toolApi.log(run.id, offset);
    if (chunk.text) {
      log.textContent += chunk.text;
      log.scrollTop = log.scrollHeight;
    }
    offset = chunk.offset;
    if (chunk.state !== "running") break;
    await new Promise((r) => setTimeout(r, 400));
  }
  showRun(await toolApi.run(run.id), panel, log.textContent ?? "");
}

function showRun(run: RunInfo, panel: HTMLElement, logText: string): void {
  const when = new Date(run.started).toLocaleString();
  const head =
    run.state === "done" && run.stats
      ? `${t.lastRun}: ${when} · ${run.stats.symbols} ${t.symbols}, ${run.stats.edges} ${t.edges} · ${run.seconds ?? 0}s`
      : `${t.lastRun}: ${when} · ${run.state === "failed" ? t.failed : run.state}`;
  const children: (HTMLElement | null)[] = [el("p", { class: "tool-muted", text: head })];
  if (run.error) children.push(el("p", { class: "tool-error", text: run.error }));
  if (run.stats) {
    const stats = el("div", { class: "tool-stats" });
    for (const [k, n] of Object.entries(run.stats.edgeKinds)) stats.appendChild(el("span", { class: "tool-badge", text: `${k} ${n}` }));
    for (const [k, n] of Object.entries(run.stats.symbolKinds)) stats.appendChild(el("span", { class: "tool-badge", text: `${k} ${n}` }));
    children.push(stats);
  }
  const log = el("pre", { class: "tool-log", text: logText });
  if (!logText) {
    log.hidden = true;
    void toolApi.log(run.id, 0).then((c) => {
      log.textContent = c.text;
    });
  }
  const showLog = el("button", { class: "tool-btn", text: "log" });
  showLog.addEventListener("click", () => (log.hidden = !log.hidden));

  const report = el("div", { class: "tool-report" });
  const noRenames = el("input", { type: "checkbox" });
  const dry = el("button", { class: "tool-btn", text: t.dryRun });
  const apply = el("button", { class: "tool-btn is-primary", text: t.apply });
  apply.disabled = true;
  const canSync = run.state === "done";
  dry.disabled = !canSync;
  dry.addEventListener("click", async () => {
    dry.disabled = apply.disabled = true;
    try {
      const res = await toolApi.sync(run.id, true, noRenames.checked);
      showReport(res, report);
      apply.disabled = res.empty;
    } catch (err) {
      report.replaceChildren(el("p", { class: "tool-error", text: (err as Error).message }));
    } finally {
      dry.disabled = false;
    }
  });
  apply.addEventListener("click", async () => {
    dry.disabled = apply.disabled = true;
    try {
      showReport(await toolApi.sync(run.id, false, noRenames.checked), report);
    } catch (err) {
      report.replaceChildren(el("p", { class: "tool-error", text: (err as Error).message }));
    } finally {
      dry.disabled = false;
    }
  });

  children.push(
    el("div", { class: "tool-actions" }, [
      dry,
      apply,
      el("label", { class: "tool-check" }, [noRenames, t.noRenames]),
      el("span", { class: "spacer" }),
      showLog,
    ]),
    log,
    report,
  );
  panel.replaceChildren(...children.filter((c): c is HTMLElement => c !== null));
}

function showReport(res: SyncResult, target: HTMLElement): void {
  const r = res.report;
  const out: HTMLElement[] = [];
  if (res.empty) out.push(el("p", { class: "tool-muted", text: t.reportEmpty }));
  const groups: [keyof typeof t.groups, string[] | null][] = [
    ["broken", r.broken],
    ["ambiguous", r.ambiguous],
    ["renames", r.renames],
    ["gone", r.gone],
    ["added", r.added],
    ["changed", r.changed],
  ];
  for (const [key, items] of groups) {
    if (!items?.length) continue;
    const details = el("details", {}, [
      el("summary", { text: `${t.groups[key]} (${items.length})` }),
      el(
        "ul",
        {},
        items.slice(0, 500).map((i) => el("li", { text: i })),
      ),
    ]);
    if (key === "broken" || key === "ambiguous" || key === "renames") details.open = true;
    out.push(details);
  }
  if (r.written?.length) {
    const open = el("a", { class: "tool-btn", text: t.openEditor });
    open.setAttribute("href", "./");
    out.push(el("p", {}, [el("span", { class: "tool-muted", text: `${t.written}: ${r.written.join(", ")}  ` }), open]));
  }
  target.replaceChildren(...out);
}

void main();
