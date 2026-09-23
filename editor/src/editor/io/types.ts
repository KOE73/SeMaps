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

export interface CatalogEntry {
  readonly id: string;
  readonly file: string;
  readonly title: string;
  readonly subtitle?: string;
  readonly icon?: string;
  readonly theme?: string;
}

export interface CatalogFile {
  schemas?: CatalogEntry[];
}
