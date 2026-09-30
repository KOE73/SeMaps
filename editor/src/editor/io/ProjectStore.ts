import type { ParsedTextCatalog } from "../../model/text-provenance.js";
import { parseTextCatalog } from "../../model/text-provenance.js";
import type {
  EntityCatalog,
  EntityEntry,
  ProjectBundle,
  ProjectManifest,
  ModelIssue,
  RelationCatalog,
  TextCatalog,
  ViewDocument,
  ViewPlacement,
  WireDocument,
  WireEdge,
  WirePlacement,
} from "../../model/wire-types.js";
import type { ModelStore, SaveTarget } from "./types.js";
import { relationShownByDefault } from "../../model/relationVisibility.js";
import { KindCatalog } from "../../model/KindCatalog.js";
import { entityDisplayName } from "../../model/entityName.js";
import { realizationsOf } from "../../model/realizations.js";
import { EDGE_OVERRIDE_FIELDS, OVERRIDE_FIELDS, parseOverride } from "../../model/override.js";

/** Contract version this editor reads and writes (CONTRACT.md, ADR_20260927-6). */
export const CONTRACT_VERSION = 5;

/**
 * Keys of an older contract that a view of contract 5 must not have. Such a
 * view is named, not read (ADR_20260927-3): the loader does not migrate; that
 * is `semaps migrate`.
 */
const OLD_VIEW_KEYS = ["zones", "nodes"] as const;
const OLD_PLACEMENT_KEYS = ["zone", "container", "id"] as const;

/**
 * Reads multi-file project models over HTTP:
 * - `project.json`
 * - `entities.json`
 * - `relations.json`
 * - `text.<lang>.json`
 * - `views/<view_id>.view.json`
 *
 * Writing goes through the host's working model (`HostModelStore`); this
 * store is what `npm run dev` uses to read the example workspace without Go.
 */
export class HttpProjectStore implements ModelStore {
  constructor(private readonly baseUrl: string = "./") {}

  async load(file: string): Promise<WireDocument> {
    const res = await fetch(new URL(file, new URL(this.baseUrl, location.href)));
    if (!res.ok) throw new Error(`Не удалось загрузить ${file}: HTTP ${res.status}`);
    const data = await res.json();
    return this.loadProjectBundle(file, data);
  }

  protected async loadProjectBundle(viewFile: string, viewData: ViewDocument, provided?: {
    project: ProjectManifest;
    entities: EntityCatalog;
    relations: RelationCatalog;
    texts: Record<string, unknown>;
  }): Promise<WireDocument> {
    if (typeof viewData?.project !== "string") {
      throw new Error(`${viewFile}: это не вид — нет поля «project» (CONTRACT.md §8).`);
    }
    checkViewShape(viewFile, viewData);

    const dir = viewFile.substring(0, viewFile.lastIndexOf("/") + 1) + "../";
    const url = (path: string): URL => new URL(dir + path, new URL(this.baseUrl, location.href));
    const projectManifest: ProjectManifest = provided?.project ?? await fetch(url("project.json"))
      .then((r) => r.json())
      .catch(() => ({ id: viewData.project || "unknown", title: "Архитектурная схема" }));
    const version = projectManifest.contractVersion;
    if (typeof version === "number" && version < CONTRACT_VERSION) {
      throw new Error(
        `${dir}project.json: contractVersion ${version} — проект старой формы; редактор читает версию ` +
          `${CONTRACT_VERSION} (ADR_20260927-6). Переведите проект командой «semaps migrate» (ADR_20260927-3).`,
      );
    }

    const languages = (projectManifest.languages?.length ? projectManifest.languages : ["ru"]) as string[];

    const [entitiesRes, relationsRes, ...rawTexts] = provided ? [
      provided.entities, provided.relations,
      ...languages.map((lang) => provided.texts[lang] ?? {}),
    ] : await Promise.all([
      fetch(url("entities.json"))
        .then((r) => r.json())
        .catch(() => ({ entities: [] })) as Promise<EntityCatalog>,
      fetch(url("relations.json"))
        .then((r) => r.json())
        .catch(() => ({ relations: [] })) as Promise<RelationCatalog>,
      ...languages.map(
        (lang) =>
          fetch(url(`text.${lang}.json`))
            .then((r) => (r.ok ? r.json() : {}))
            .catch(() => ({})) as Promise<unknown>,
      ),
    ]);

    // Provenance stays with the file; the rest of the editor sees plain strings.
    const textFiles: Record<string, ParsedTextCatalog> = {};
    const textRegistries: Record<string, TextCatalog> = {};
    languages.forEach((lang, i) => {
      const parsed = parseTextCatalog(rawTexts[i], lang);
      textFiles[lang] = parsed;
      textRegistries[lang] = { entries: parsed.entries };
    });

    const primary = languages[0] ?? "ru";
    const textRes: TextCatalog = textRegistries[primary] ?? { entries: {} };
    const kinds = KindCatalog.active;
    const issues: ModelIssue[] = [];
    const entityById = new Map((entitiesRes.entities ?? []).map((e) => [e.id, e]));

    const placements: WirePlacement[] = (viewData.placements ?? []).map((vp: ViewPlacement) => {
      const entityId = vp.entity;
      const e: EntityEntry = entityById.get(entityId) ?? { id: entityId, kind: "" };
      const t = textRes.entries?.[entityId];
      const code = realizationsOf(e);
      const override = parseOverride(vp.override, OVERRIDE_FIELDS);
      if (override.rejected.length > 0) {
        issues.push({
          kind: "override-field",
          message:
            `${viewFile}: у размещения «${entityId}» в «override» поля вне таблицы ` +
            `(${override.rejected.join(", ")}) — они не прочитаны (CONTRACT.md §11.6).`,
        });
      }
      return {
        id: entityId,
        // Frame or block is the kind's to say, not the file's (§8.2).
        container: kinds.isContainer(e.kind),
        // The editor puts the viewer's data language over this (`DiagramDocument.refreshNames`).
        label: entityDisplayName(e, entityId, textRegistries, primary, languages),
        type: e.kind,
        parent: vp.parent ?? null,
        x: vp.x,
        y: vp.y,
        width: vp.width || 170,
        height: vp.height || 50,
        ...(vp.styleId === undefined ? {} : { styleId: vp.styleId }),
        ...(override.value === undefined ? {} : { override: override.value }),
        ...(vp.collapsed === undefined ? {} : { collapsed: vp.collapsed }),
        metadata: {
          ...(code.length > 0 ? { code } : {}),
          description: t?.doc || t?.description,
          // Content template chosen for this one placement, overriding the
          // style's. The exception, not the rule: one node that must show more
          // (or less) than its kind normally does (ADR_20260903 §2.2).
          template: vp.template,
        },
        raw: { _entity: e },
      };
    });

    // No edge list of its own: the registry's relations, as the view's and
    // the dictionary's visibility defaults allow (CONTRACT.md §8.5).
    const rawEdges = Array.isArray(viewData.edges)
      ? viewData.edges
      : (relationsRes.relations || []).filter((r) =>
          relationShownByDefault(r, viewData.relations as any, kinds));

    // A view's own edge entries don't repeat `origin` — only the relation
    // registry does — so look it up by id to know whether this edge is
    // allowed any text at all (ADR_20260831 §2.13).
    const relationOriginById = new Map<string, "code" | "authored" | undefined>(
      (relationsRes.relations || []).map((r) => [r.id, r.origin]),
    );

    const translatedEdges: WireEdge[] = rawEdges.map((ve: any, i: number) => {
      const id = ve.id || `edge_${i}`;
      const origin = relationOriginById.get(id) ?? ve.origin;
      const text = origin === "code" ? undefined : textRes.entries?.[id];
      const override = parseOverride(ve.override, EDGE_OVERRIDE_FIELDS);
      if (override.rejected.length > 0) {
        issues.push({
          kind: "override-field",
          message:
            `${viewFile}: у связи «${id}» в «override» поля вне таблицы ` +
            `(${override.rejected.join(", ")}) — они не прочитаны (CONTRACT.md §11.6).`,
        });
      }
      return {
        id,
        from: ve.from,
        to: ve.to,
        type: ve.type ?? "",
        // Text of a relation lives in the text catalogue under its own id; a
        // generated relation has none, and its meaning is carried by its type.
        label: text?.name || text?.title || "",
        fromLabel: text?.fromLabel,
        toLabel: text?.toLabel,
        styleId: ve.styleId,
        ...(override.value === undefined ? {} : { override: override.value }),
        points: ve.points || [],
        ...(origin === undefined ? {} : { origin }),
        // Line shape picked for this one edge. Only the choice: the polyline
        // is recomputed every repaint and never written back.
        ...(ve.routing === undefined ? {} : { routing: ve.routing }),
      };
    });

    /**
     * A view says what its containers classify; failing that, the project says
     * what to assume. Neither is not fatal — the diagram still draws, and the
     * editor says so — but the containment in it must not be read as an
     * assertion about anything.
     */
    const resolvedAxis = viewData.axis ?? projectManifest.defaultAxis;
    if (!resolvedAxis) {
      issues.unshift({
        kind: "view-without-axis",
        message:
          `Вид «${viewData.id || viewFile}» не объявляет ось классификации, и у проекта нет ` +
          `запасной (defaultAxis). Схема открыта, но вложенность размещений в контейнеры ` +
          `здесь ничего не утверждает и не проверяется. ` +
          `См. ADR_20260831_diagrams_text-provenance-and-view-axes.`,
      });
    }

    const bundle: ProjectBundle = {
      project: projectManifest,
      entities: entitiesRes,
      relations: relationsRes,
      text: textRes,
      textRegistries,
      textFiles,
      view: viewData,
      // Resolved, not written back: a view that inherits its axis keeps
      // inheriting it, and saving does not mint a field the author never wrote.
      ...(resolvedAxis ? { resolvedAxis } : {}),
      ...(issues.length > 0 ? { issues } : {}),
    };

    return {
      metadata: {
        title: projectManifest.title,
        subtitle: projectManifest.subtitle,
        // The picture's own convention for line shape, sitting between the
        // relation type's choice and a single edge's override.
        ...(viewData.routing === undefined ? {} : { routing: viewData.routing }),
      },
      placements,
      edges: translatedEdges,
      views: [],
      bundle,
    };
  }

  /**
   * The host's `/api/save` no longer writes project files (it answers 410):
   * a project is changed through the working model, `HostModelStore`.
   */
  async save(target: SaveTarget, _wire: WireDocument): Promise<void> {
    throw new Error(
      `Сохранение «${target.file}» идёт через хост (semaps): этот режим только читает модель.`,
    );
  }
}

/** Refuse a view of an older contract, naming the file and the key. */
function checkViewShape(file: string, view: ViewDocument): void {
  for (const key of OLD_VIEW_KEYS) {
    if (key in view) {
      throw new Error(
        `${file}: ключ «${key}» — форма вида старого контракта. В контракте 5 блоки и контейнеры — ` +
          `один массив «placements» (CONTRACT.md §8.3, ADR_20260927-6); переведите проект командой «semaps migrate».`,
      );
    }
  }
  (view.placements ?? []).forEach((p, i) => {
    for (const key of OLD_PLACEMENT_KEYS) {
      if (key in p) {
        throw new Error(
          `${file}: placements[${i}] — поле «${key}» старого контракта. Размещение называет сущность ` +
            `полем «entity», контейнер — полем «parent» (CONTRACT.md §8.3).`,
        );
      }
    }
    if (typeof p.entity !== "string" || p.entity === "") {
      throw new Error(`${file}: placements[${i}] без «entity» (CONTRACT.md §8.2).`);
    }
  });
}
