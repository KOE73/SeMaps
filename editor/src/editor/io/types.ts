import type { WireStyleSheet } from "../../model/style-types.js";
import type { WireDocument } from "../../model/wire-types.js";

export interface SaveTarget {
  /** File name / path as the host knows it, e.g. "projects/core/views/v_semantic_atlas.view.json". */
  readonly file: string;
}

export interface ModelStore {
  load(file: string): Promise<WireDocument>;
  save(target: SaveTarget, wire: WireDocument): Promise<void>;
}

export interface StyleStore {
  load(): Promise<WireStyleSheet>;
  save(sheet: WireStyleSheet): Promise<void>;
}

/**
 * What the workspace holds, as the host found it on disk: projects, and the
 * views in each. There is no list file (ADR_20260923-7).
 */
export interface WorkspaceIndex {
  readonly projects: readonly ProjectEntry[];
}

export interface ProjectEntry {
  readonly id: string;
  readonly title: string;
  readonly subtitle?: string;
  readonly icon?: string;
  readonly theme?: string;
  readonly languages: readonly string[];
  readonly views: readonly ViewEntry[];
  readonly error?: string;
}

export interface ViewEntry {
  readonly id: string;
  /** Workspace-relative path of the `.view.json`. Unique across the workspace, unlike `id`. */
  readonly file: string;
  readonly axis?: string;
  readonly icon?: string;
  readonly theme?: string;
  /** `name` under the view's id in each `text.<lang>.json`. */
  readonly names: Readonly<Record<string, string>>;
  readonly error?: string;
}

/** What a person sets on a project; `language` only matters when creating it. */
export interface NewProject {
  readonly id: string;
  readonly title: string;
  readonly subtitle?: string;
  readonly icon?: string;
  readonly theme?: string;
  readonly language: string;
}

/** What a person sets on a view; `name` is written in `language`. */
export interface NewView {
  readonly project: string;
  readonly id: string;
  readonly name: string;
  readonly axis: string;
  readonly icon?: string;
  readonly theme?: string;
  readonly language: string;
}

export interface WorkspaceStore {
  load(): Promise<WorkspaceIndex>;
  createProject(project: NewProject): Promise<void>;
  /** Returns the new view's file. */
  createView(view: NewView): Promise<string>;
  /** A new `id` renames the folder and rewrites `project` in every view. */
  updateProject(oldId: string, project: NewProject): Promise<void>;
  /** A new `id` renames the file and moves its texts in every language. Returns the view's file. */
  updateView(oldId: string, view: NewView, languages: readonly string[]): Promise<string>;
}