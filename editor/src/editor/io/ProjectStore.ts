import type { ParsedTextCatalog } from "../../model/text-provenance.js";
import { parseTextCatalog, serializeTextCatalog } from "../../model/text-provenance.js";
import type {
  EntityCatalog,
  EntityEntry,
  ProjectBundle,
  ProjectManifest,
  ModelIssue,
  RelationCatalog,
  RelationTypeCatalog,
  TextCatalog,
  ViewDocument,
  ViewEdgePlacement,
  ViewNodePlacement,
  ViewZonePlacement,
  WireDocument,
} from "../../model/wire-types.js";
import type { ModelStore, SaveTarget } from "./types.js";

/**
 * Reads and writes multi-file project models over HTTP.
 *
 * Symmetrically coordinates:
 * - `project.json`
 * - `entities.json`
 * - `relations.json`
 * - `text.ru.json`
 * - `views/<view_id>.view.json`
 */
export class HttpProjectStore implements ModelStore {
  constructor(private readonly baseUrl: string = "./") {}

  async load(file: string): Promise<WireDocument> {
    const res = await fetch(new URL(file, new URL(this.baseUrl, location.href)));
    if (!res.ok) throw new Error(`Не удалось загрузить ${file}: HTTP ${res.status}`);
    const data = await res.json();
    return this.loadProjectBundle(file, data);
  }

  private async loadProjectBundle(viewFile: string, viewData: any): Promise<WireDocument> {
    const isView = !!viewData.project;
    let projectManifest: ProjectManifest = viewData;
    let dir = viewFile.substring(0, viewFile.lastIndexOf("/") + 1);

    if (isView) {
      dir = dir + "../";
      projectManifest = await fetch(new URL(dir + "project.json", new URL(this.baseUrl, location.href)))
        .then((r) => r.json())
        .catch(() => ({ id: viewData.project || "unknown", title: "Архитектурная схема" }));
    }

    const languages = (projectManifest.languages?.length ? projectManifest.languages : ["ru"]) as string[];

    const [entitiesRes, relationsRes, relationTypesRes, ...rawTexts] = await Promise.all([
      fetch(new URL(dir + "entities.json", new URL(this.baseUrl, location.href)))
        .then((r) => r.json())
        .catch(() => ({ entities: [] })) as Promise<EntityCatalog>,
      fetch(new URL(dir + "relations.json", new URL(this.baseUrl, location.href)))
        .then((r) => r.json())
        .catch(() => ({ relations: [] })) as Promise<RelationCatalog>,
      fetch(new URL(dir + "relation-types.json", new URL(this.baseUrl, location.href)))
        .then((r) => r.json())
        .catch(() => ({ relationTypes: [] })) as Promise<RelationTypeCatalog>,
      ...languages.map(
        (lang) =>
          fetch(new URL(dir + `text.${lang}.json`, new URL(this.baseUrl, location.href)))
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

    const placements = viewData.placements || viewData.nodes || [];
    const translatedNodes = placements.map((vn: any) => {
      const entityId = vn.entity || vn.id;
      const e: EntityEntry = entitiesRes.entities?.find?.((x) => x.id === entityId) ?? {
        id: entityId,
        name: entityId,
        kind: vn.type || "Component",
        codeRef: "",
      };
      const t = textRes.entries?.[entityId] ?? {
        name: e.name || entityId,
        title: e.name || entityId,
        doc: "",
        description: "",
      };
      return {
        id: entityId,
        label: t.name || t.title || e.name || entityId,
        type: vn.type || e.kind || "Component",
        zone: vn.zone || vn.container || null,
        x: vn.x,
        y: vn.y,
        width: vn.width || 170,
        height: vn.height || 50,
        styleId: vn.styleId,
        metadata: {
          codeRef: e.codeRef,
          description: t.doc || t.description,
          // Content template chosen for this one placement, overriding the
          // style's. The exception, not the rule: one node that must show more
          // (or less) than its kind normally does (ADR_20260903 §2.2).
          template: vn.template,
        },
        raw: { _entity: e },
      };
    });

    /**
     * A view's zone id was minted by the old layout generator as `z_<x>` from
     * the container `c_<x>` it renders, so text
     * keyed to the container is not found under the zone's own id. Resolve
     * through both. The real fix belongs in the generator — a zone should name
     * the container it stands for, the way a node placement names its entity.
     */
    const zoneTextKey = (id: string) =>
      textRes.entries?.[id] ? id : id.startsWith("z_") ? "c_" + id.slice(2) : id;

    const translatedZones = (viewData.zones || []).map((vz: any) => {
      const textKey = zoneTextKey(vz.id);
      const zName = textRes.entries?.[textKey]?.name || textRes.entries?.[textKey]?.title || vz.name || vz.id;
      return {
        id: vz.id,
        // A zone's caption is `name` on the wire (`label` is a node's); emitting
        // the wrong one is why zone captions used to render as raw ids.
        name: zName,
        type: vz.type || "zone",
        zone: vz.parent || vz.container || null,
        x: vz.x,
        y: vz.y,
        width: vz.width,
        height: vz.height,
        styleId: vz.styleId,
        metadata: { description: textRes.entries?.[textKey]?.doc || textRes.entries?.[textKey]?.description },
      };
    });

    const rawEdges = Array.isArray(viewData.edges)
      ? viewData.edges
      : relationsRes.relations || [];

    // A view's own edge placements don't repeat `origin` — only the relation
    // registry does — so look it up by id to know whether this edge is
    // allowed any text at all (ADR_20260831 §2.13).
    const relationOriginById = new Map<string, "code" | "authored" | undefined>(
      (relationsRes.relations || []).map((r) => [r.id, r.origin]),
    );

    const translatedEdges = rawEdges.map((ve: any, i: number) => {
      const id = ve.id || `edge_${i}`;
      const origin = relationOriginById.get(id) ?? ve.origin;
      const text = origin === "code" ? undefined : textRes.entries?.[id];
      return {
        id,
        from: ve.from || ve.source,
        to: ve.to || ve.target,
        type: ve.type || ve.relation || "relates",
        // Text of a relation lives in the text catalogue under its own id; a
        // generated relation has none, and its meaning is carried by its type.
        label: text?.name || text?.title || "",
        fromLabel: text?.fromLabel,
        toLabel: text?.toLabel,
        styleId: ve.styleId,
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
    const resolvedAxis = (viewData as ViewDocument).axis ?? projectManifest.defaultAxis;
    const issues: ModelIssue[] = [];
    if (isView && !resolvedAxis) {
      issues.push({
        kind: "view-without-axis",
        message:
          `Вид «${viewData.id || viewFile}» не объявляет ось классификации, и у проекта нет ` +
          `запасной (defaultAxis). Схема открыта, но вложенность блоков в контейнеры ` +
          `здесь ничего не утверждает и не проверяется. ` +
          `См. ADR_20260831_diagrams_text-provenance-and-view-axes.`,
      });
    }

    const bundle: ProjectBundle = {
      project: projectManifest,
      entities: entitiesRes,
      relations: relationsRes,
      relationTypes: relationTypesRes,
      text: textRes,
      textRegistries,
      textFiles,
      view: isView
        ? (viewData as ViewDocument)
        : { id: "v_main", project: projectManifest.id, zones: viewData.zones, nodes: viewData.nodes, edges: viewData.edges },
      // Resolved, not written back: a view that inherits its axis keeps
      // inheriting it, and saving does not mint a field the author never wrote.
      ...(resolvedAxis ? { resolvedAxis } : {}),
      ...(issues.length > 0 ? { issues } : {}),
    };

    const wire: WireDocument = {
      metadata: {
        title: projectManifest.title,
        subtitle: projectManifest.subtitle,
        // The picture's own convention for line shape, sitting between the
        // relation type's choice and a single edge's override.
        ...(viewData.routing === undefined ? {} : { routing: viewData.routing }),
      },
      zones: translatedZones,
      nodes: translatedNodes,
      edges: translatedEdges,
      views: [],
      bundle,
    };

    return wire;
  }

  async save(target: SaveTarget, wire: WireDocument): Promise<void> {
    const bundle = wire.bundle;

    // If this is a project-based model, save clean view layout to target.file
    if (bundle && bundle.project && bundle.view) {
      await this.saveProjectBundle(target.file, wire, bundle);
      return;
    }

    // Fallback for standalone/legacy single-file JSON models
    const res = await fetch(`/api/save?file=${encodeURIComponent(target.file)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(wire, null, 2),
    });
    if (!res.ok) throw new Error(`Сервер ответил HTTP ${res.status}`);
  }

  private async saveProjectBundle(viewFile: string, wire: WireDocument, bundle: ProjectBundle): Promise<void> {
    // 1. Construct clean ViewDocument matching CONTRACT.md
    const zones: ViewZonePlacement[] = (wire.zones || []).map((z) => ({
      id: z.id,
      container: (z as any).zone || (z as any).container || null,
      parent: (z as any).parent || null,
      x: z.x,
      y: z.y,
      width: z.width,
      height: z.height,
      styleId: z.styleId,
    }));

    const nodes: ViewNodePlacement[] = (wire.nodes || []).map((n) => ({
      id: n.id,
      container: (n as any).zone || (n as any).container || null,
      x: n.x,
      y: n.y,
      width: n.width,
      height: n.height,
      styleId: n.styleId,
      // Written back because the author put it there. A field the loader reads
      // but the saver drops is worse than one that never existed: the first
      // save quietly deletes a hand-made choice.
      ...(typeof n.metadata?.template === "string" ? { template: n.metadata.template } : {}),
    }));

    const edges: ViewEdgePlacement[] = (wire.edges || []).map((e) => ({
      id: e.id,
      from: e.from,
      to: e.to,
      type: e.type,
      styleId: e.styleId,
      points: e.points || [],
      ...(e.routing === undefined ? {} : { routing: e.routing }),
    }));

    const cleanView: ViewDocument = {
      id: bundle.view.id || "v_main",
      project: bundle.project.id,
      axis: bundle.view.axis,
      ...(bundle.view.icon === undefined ? {} : { icon: bundle.view.icon }),
      ...(bundle.view.theme === undefined ? {} : { theme: bundle.view.theme }),
      ...(bundle.view.order === undefined ? {} : { order: bundle.view.order }),
      ...(bundle.view.routing === undefined ? {} : { routing: bundle.view.routing }),
      ...(bundle.view.relations ? { relations: bundle.view.relations } : {}),
      zones,
      nodes,
      edges,
    };

    // Save the view file
    const res = await fetch(`/api/save?file=${encodeURIComponent(viewFile)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(cleanView, null, 2),
    });
    if (!res.ok) throw new Error(`Не удалось сохранить ${viewFile}: HTTP ${res.status}`);

    // Update text and entities if any new items were created/modified
    const dir = viewFile.substring(0, viewFile.lastIndexOf("/") + 1) + "../";
    let textModified = false;
    let entitiesModified = false;

    const currentText = bundle.text.entries || {};
    const currentEntities = bundle.entities.entities || [];
    const entityMap = new Map(currentEntities.map((e) => [e.id, e]));

    for (const n of wire.nodes || []) {
      if (!entityMap.has(n.id)) {
        currentEntities.push({
          id: n.id,
          name: n.label || n.id,
          kind: n.type || "Component",
          origin: "authored",
          status: "present",
          codeRef: n.metadata?.codeRef || "",
          members: [],
        });
        entitiesModified = true;
      }
      if (n.label && currentText[n.id]?.name !== n.label) {
        currentText[n.id] = { ...currentText[n.id], name: n.label, doc: n.metadata?.description || currentText[n.id]?.doc || "" };
        textModified = true;
      }
    }

    for (const z of wire.zones || []) {
      const zoneName = z.name;
      if (zoneName && currentText[z.id]?.name !== zoneName) {
        currentText[z.id] = { ...currentText[z.id], name: zoneName, doc: z.metadata?.description || currentText[z.id]?.doc || "" };
        textModified = true;
      }
    }

    if (entitiesModified) {
      await fetch(`/api/save?file=${encodeURIComponent(dir + "entities.json")}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ entities: currentEntities }, null, 2),
      }).catch((err) => console.warn("Не удалось синхронизировать entities.json:", err));
    }

    const registries = bundle.textRegistries ?? (textModified ? { ru: { entries: currentText } } : null);
    if (registries) {
      for (const [lang, catalog] of Object.entries(registries)) {
        if (!catalog?.entries || Object.keys(catalog.entries).length === 0) continue;
        const textFile = dir + `text.${lang}.json`;
        // Keys written since this view was opened — a new view's name, an
        // agent's description — are on disk but not in the bundle; keep them.
        const onDisk = await fetch(new URL(textFile, new URL(this.baseUrl, location.href)))
          .then((r) => (r.ok ? r.json() : null))
          .then((raw) => (raw ? parseTextCatalog(raw, lang) : null))
          .catch(() => null);
        const loaded = bundle.textFiles?.[lang] ?? null;
        const entries = { ...onDisk?.entries, ...catalog.entries };
        const provenance = { ...onDisk?.provenance, ...loaded?.provenance };
        const base = onDisk || loaded
          ? { contractVersion: 3, language: lang, entries: { ...onDisk?.entries, ...loaded?.entries }, provenance }
          : null;
        // Values the user changed are re-stamped as authored; the rest keep the
        // provenance they were loaded with, so an untouched save is a no-op diff.
        const file = serializeTextCatalog(lang, entries, base);
        await fetch(`/api/save?file=${encodeURIComponent(textFile)}`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(file, null, 2),
        }).catch((err) => console.warn(`Не удалось синхронизировать text.${lang}.json:`, err));
      }
    }
  }
}
