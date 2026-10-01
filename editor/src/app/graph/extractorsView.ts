import { el } from "../../util/dom.js";
import { fmt, t } from "../../shell/strings.js";
import { icons } from "../../ui/icons.js";
import { kindIconEl } from "../../ui/kindIcons.js";
import { toolApi, type ExtractorView, type RunInfo } from "../../shell/api.js";
import { graphFilterConfig } from "./filterConfig.js";
import type { GraphResponse } from "./types.js";

/**
 * The «Экстракторы» panel of the graph mode: what the live graph holds and
 * why, per extractor of the current project — one big «Запустить извлечение»,
 * the status of the last run (with a live log while it runs), what the
 * extractor contributes to the graph, the edge kinds it extracts and its
 * watch setting (both saved to the .semaps file the way the Extractors page
 * saves them).
 *
 * The run is `POST /api/runs`: it runs the extractor and keeps its facts
 * under the runs directory, which the live graph reads. It never writes the
 * registry — only `POST /api/runs/{id}/sync` does, and that stays on the
 * Extractors page (linked from every card). Methods and calls live only in
 * the graph.
 */

/** What the extractors can be told to print (graph-filters.json `extractorEdgeKinds`). */
const edgeChips = (): readonly string[] => graphFilterConfig().extractorEdgeKinds;
const LOG_TAIL_LINES = 6;

const time = (iso: string | undefined): string =>
  iso ? new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "";

/** The `--edges` a run was started with. */
function edgesOfRun(run: RunInfo): string[] {
  const at = run.command?.indexOf("--edges") ?? -1;
  const value = at >= 0 ? run.command?.[at + 1] : undefined;
  return value ? value.split(",").filter(Boolean) : [];
}

const sameSet = (a: readonly string[], b: readonly string[]): boolean => a.length === b.length && a.every((x) => b.includes(x));

export class ExtractorsView {
  readonly root = el("div", { class: "graph-ex" });
  /** Called when a run has ended, so the graph reads the new facts at once. */
  onRunFinished: (() => void) | undefined;
  private data: GraphResponse | undefined;
  private project = "";
  private extractors: ExtractorView[] = [];
  private failedToLoad: string | undefined;
  private readonly following = new Set<string>(); // extractor ids whose run is being followed
  private readonly logs = new Map<string, string>(); // live log of a running run, per extractor
  private readonly errors = new Map<string, string>(); // a start or save that failed, per extractor
  private loadToken = 0;

  /** The graph shown now (after a fetch or a live diff): redraws and re-reads the runs. */
  setGraph(data: GraphResponse | undefined, project: string): void {
    this.data = data;
    this.project = project;
    void this.reload();
  }

  private async reload(): Promise<void> {
    const token = ++this.loadToken;
    try {
      const setup = await toolApi.setup();
      if (token !== this.loadToken) return;
      this.extractors = setup.extractors.filter((e) => e.project === this.project);
      this.failedToLoad = undefined;
    } catch (err) {
      if (token !== this.loadToken) return;
      this.failedToLoad = (err as Error).message;
    }
    this.render();
    for (const e of this.extractors) {
      if (e.lastRun?.state === "running") void this.follow(e.id, e.lastRun.id);
    }
  }

  /** Follows a run — its log grows on the panel — until it ends, then reads
   * everything again and tells the graph to fetch the new facts. */
  private async follow(extractor: string, runId: string): Promise<void> {
    if (this.following.has(extractor)) return;
    this.following.add(extractor);
    this.logs.set(extractor, "");
    let offset = 0;
    try {
      for (;;) {
        await new Promise((r) => setTimeout(r, 600));
        const chunk = await toolApi.log(runId, offset);
        offset = chunk.offset;
        if (chunk.text) this.logs.set(extractor, (this.logs.get(extractor) ?? "") + chunk.text);
        this.render();
        if (chunk.state !== "running") break;
      }
    } catch {
      // The host went away or the run was pruned: the reload below shows the truth.
    } finally {
      this.following.delete(extractor);
      this.logs.delete(extractor);
      await this.reload();
      this.onRunFinished?.();
    }
  }

  private async start(e: ExtractorView): Promise<void> {
    this.errors.delete(e.id);
    try {
      const run = await toolApi.startRun(e.id);
      void this.follow(e.id, run.id);
    } catch (err) {
      this.errors.set(e.id, (err as Error).message);
    }
    this.render();
    void this.reload();
  }

  private async save(e: ExtractorView, patch: { edges?: string[]; watch?: boolean }): Promise<void> {
    try {
      await toolApi.saveExtractor(e.id, patch);
      this.errors.delete(e.id);
    } catch (err) {
      this.errors.set(e.id, (err as Error).message);
    }
    void this.reload();
  }

  private render(): void {
    this.root.replaceChildren(
      el("div", { class: "graph-ex-summary" }, [this.summary()]),
      ...(this.failedToLoad ? [el("p", { class: "tool-error", text: this.failedToLoad })] : []),
      ...(this.extractors.length === 0 && !this.failedToLoad ? [el("p", { class: "tool-muted", text: t.graphExNoExtractors })] : []),
      ...this.extractors.map((e) => this.card(e)),
    );
  }

  /** One line: what the graph shows now. */
  private summary(): string {
    if (this.extractors.length === 0) return t.graphExSummaryNone;
    const used = (this.data?.facts ?? []).filter((f) => f.finished);
    if (used.length > 0) return fmt(t.graphExSummaryFacts, { list: used.map((f) => `${f.language}, ${time(f.finished)}`).join("; ") });
    if (this.extractors.some((e) => e.lastRun?.state === "failed")) return t.graphExSummaryFailed;
    return t.graphExSummaryNever;
  }

  private counts(id: string): { nodes: number; edges: number } {
    const nodes = new Set<string>();
    for (const n of this.data?.nodes ?? []) if (n.extractor === id && n.presence !== "model") nodes.add(n.id);
    let edges = 0;
    for (const e of this.data?.edges ?? []) if (e.presence !== "model" && nodes.has(e.from)) edges++;
    return { nodes: nodes.size, edges };
  }

  private badge(run: RunInfo | undefined, running: boolean): HTMLElement {
    let state = "never";
    let icon: string = icons.clock;
    let text: string = t.graphExNever;
    let detail = "";
    if (running) {
      state = "running";
      icon = icons.loader2;
      text = t.graphExRunning;
    } else if (run?.state === "done") {
      state = "ok";
      icon = icons.circleCheck;
      text = t.graphExOk;
      detail = time(run.finished ?? run.started);
    } else if (run?.state === "failed") {
      state = "failed";
      icon = icons.circleX;
      text = t.graphExFailed;
      detail = time(run.finished ?? run.started);
    }
    const iconEl = el("span", { class: "graph-ex-badge-icon" });
    iconEl.innerHTML = icon;
    return el("div", { class: `graph-ex-badge is-${state}` }, [
      iconEl,
      el("span", { class: "graph-ex-badge-text", text }),
      detail ? el("span", { class: "graph-ex-badge-detail", text: detail }) : null,
    ]);
  }

  private card(e: ExtractorView): HTMLElement {
    const run = e.lastRun;
    const fact = this.data?.facts.find((f) => f.extractor === e.id);
    const { nodes, edges } = this.counts(e.id);
    const tool = e.tool;
    const toolBroken = !tool.found || (tool.runtime !== undefined && !tool.runtime.ok);
    const running = run?.state === "running" || this.following.has(e.id);

    // The big button: run, and see it run.
    const runBtn = el("button", { class: "tool-btn is-primary graph-ex-run", title: t.graphExRefreshHint, disabled: running || toolBroken });
    const runIcon = el("span", { class: `graph-ex-run-icon${running ? " is-spinning" : ""}` });
    runIcon.innerHTML = running ? icons.loader2 : icons.playerPlay;
    runBtn.append(runIcon, running ? t.graphExRunning : t.graphExRun);
    runBtn.addEventListener("click", () => void this.start(e));

    // Edge kinds changed since the last run: the graph does not show them yet.
    const pending = run !== undefined && !running && !sameSet(edgesOfRun(run), e.edges);

    const log = this.logs.get(e.id);
    const tail = log ? log.split(/\r?\n/).filter((l) => l.trim()).slice(-LOG_TAIL_LINES).join("\n") : "";

    // What the last run gave, and what of it the graph holds.
    const result =
      run?.state === "done" && run.stats
        ? fmt(t.graphExResult, { nodes: String(run.stats.symbols), edges: String(run.stats.edges), seconds: String(Math.round(run.seconds ?? 0)) })
        : "";
    const contributes = fact?.finished ? fmt(t.graphExContributes, { nodes: String(nodes), edges: String(edges) }) : t.graphExNoFacts;
    const stale = run?.state === "failed" && fact?.finished ? fmt(t.graphExStale, { time: time(fact.finished) }) : "";

    const chips = el(
      "div",
      { class: "graph-ex-chips" },
      edgeChips().map((kind) => {
        const on = e.edges.includes(kind);
        const chip = el("button", {
          class: `graph-ex-chip${on ? " is-on" : ""}`,
          title: graphFilterConfig().callKinds[0] === kind ? t.graphExCallsHint : undefined,
          attrs: { "aria-pressed": String(on) },
        });
        chip.append(kindIconEl("edge", kind), kind);
        if (on) {
          const mark = el("span", { class: "graph-ex-chip-mark" });
          mark.innerHTML = icons.check;
          chip.append(mark);
        }
        chip.addEventListener("click", () => void this.save(e, { edges: on ? e.edges.filter((k) => k !== kind) : [...e.edges, kind] }));
        return chip;
      }),
    );

    const watch = el("input", { type: "checkbox" }) as HTMLInputElement;
    watch.checked = e.watch;
    watch.addEventListener("change", () => void this.save(e, { watch: watch.checked }));

    const reconcile = el("a", { class: "graph-ex-reconcile", text: t.graphExReconcile });
    reconcile.href = "#extract";

    const problems = [tool.problem, tool.runtime && !tool.runtime.ok ? tool.runtime.hint : undefined, !tool.found ? t.graphExToolMissing : undefined].filter(Boolean);
    const error = this.errors.get(e.id);

    return el("section", { class: "graph-ex-card" }, [
      el("div", { class: "graph-ex-head" }, [el("strong", { text: e.id }), el("span", { class: "tool-muted", text: e.language })]),
      el("div", { class: "graph-ex-runrow" }, [runBtn, pending ? el("span", { class: "graph-ex-pending", text: t.graphExPending }) : null]),
      tail ? el("pre", { class: "graph-ex-log", text: tail }) : null,
      this.badge(run, running),
      result ? el("div", { class: "graph-ex-line", text: result }) : null,
      el("div", { class: "graph-ex-line", text: contributes }),
      stale ? el("div", { class: "graph-ex-line is-warn", text: stale }) : null,
      run?.state === "failed" && run.error && !running
        ? el("details", { class: "graph-ex-error", attrs: { open: "" } }, [el("summary", { text: t.graphExErrorText }), el("pre", { text: run.error })])
        : null,
      problems.length ? el("p", { class: "tool-error", text: problems.join("\n") }) : null,
      error ? el("p", { class: "tool-error", text: error }) : null,
      el("div", { class: "graph-ex-label", text: t.graphExKindsTitle }),
      chips,
      el("p", { class: "tool-hint", text: t.graphExKindsChanged }),
      el("label", { class: "graph-ex-switch" }, [watch, el("span", { class: "graph-ex-switch-track" }), el("span", { class: "graph-ex-switch-text", text: `${t.graphExWatch} — ${t.graphExWatchHint}` })]),
      el("div", { class: "graph-ex-actions" }, [reconcile]),
    ]);
  }
}
