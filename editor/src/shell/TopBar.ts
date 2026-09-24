import { el } from "../util/dom.js";
import { toolApi, type Setup } from "./api.js";
import { t } from "./strings.js";

export type ShellPage = "editor" | "extract" | "setup";

/**
 * The strip on top of every page of the tool — the editor, extractors,
 * settings: one place and one look for the project name and the way between
 * pages. Links are relative, so they work under the host's /app/ and in dev.
 *
 * When the host has no .semaps file (started with --workspace) the tool pages
 * have nothing to show and their links are left out.
 */
export function createTopBar(active: ShellPage): { element: HTMLElement; setup: Promise<Setup | undefined> } {
  const title = el("span", { class: "shell-project", text: "" });
  const nav = el("nav", { class: "shell-nav" });
  const pages: { id: ShellPage; href: string; label: string }[] = [
    { id: "editor", href: "./", label: t.navEditor },
    { id: "extract", href: "./extract.html", label: t.navExtract },
    { id: "setup", href: "./setup.html", label: t.navSetup },
  ];
  const links = new Map<ShellPage, HTMLElement>();
  for (const p of pages) {
    const a = el("a", { class: "shell-nav-link" + (p.id === active ? " is-active" : ""), text: p.label });
    a.setAttribute("href", p.href);
    if (p.id !== "editor") a.hidden = true; // until the host says there is a project file
    links.set(p.id, a);
    nav.appendChild(a);
  }

  const element = el("header", { class: "shell-topbar" }, [
    el("span", { class: "shell-brand", text: "SeMaps" }),
    title,
    nav,
  ]);

  const setup = toolApi.setup().then(
    (s) => {
      title.textContent = s.name || s.projectFile;
      document.title = `${s.name || s.projectFile} — SeMaps`;
      for (const [id, a] of links) if (id !== "editor") a.hidden = false;
      return s;
    },
    () => undefined,
  );
  return { element, setup };
}

/** The theme the editor saved, so every page looks the same. */
export function applySavedTheme(): void {
  try {
    const theme = localStorage.getItem("semaps:theme") || "cream";
    document.documentElement.setAttribute("data-theme", theme);
  } catch {
    document.documentElement.setAttribute("data-theme", "cream");
  }
}
