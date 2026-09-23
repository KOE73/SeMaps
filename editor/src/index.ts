/**
 * Public API of @semaps/editor.
 *
 * Everything else under src/ is internal. Reaching past this entry point into
 * renderers or the interaction controller is not supported — those are exactly
 * the parts meant to change.
 */

import "./styles/dockview.css";
import "./styles/canvas.css";
import "./styles/ribbon.css";
import "./styles/canvas-filters.css";
import "./styles/doc-editor.css";
import "./styles/code-viewer.css";

export { DiagramCanvas, defaultRegistry } from "./canvas/DiagramCanvas.js";
export type {
  CanvasEvents,
  DiagramCanvasOptions,
  EdgeFamily,
  Selection,
  SelectionKind,
} from "./canvas/DiagramCanvas.js";

export { DiagramEditor } from "./editor/DiagramEditor.js";
export type { CatalogEntry, DiagramEditorOptions } from "./editor/DiagramEditor.js";

export { DiagramDocument } from "./model/document.js";
export { parseDocument, serializeDocument } from "./model/wire.js";
export type {
  DiagramEdge,
  DiagramElement,
  DiagramMetadata,
  DiagramView,
  ElementKind,
} from "./model/types.js";
export type {
  WireDocument,
  WireEdge,
  WireNode,
  WireView,
  WireZone,
} from "./model/wire-types.js";

// Looks. An element no longer carries colours of its own, so anything that
// wants to change how a diagram appears goes through the library rather than
// through the model — see `model/style-types.ts` for why.
export { StyleLibrary } from "./model/StyleLibrary.js";
export type {
  Endpoint,
  Paint,
  ResolvedBlockStyle,
  ResolvedEdgeStyle,
  Stroke,
  TextStyle,
} from "./model/StyleLibrary.js";
export { builtinStyleSheet } from "./model/style-defaults.js";
export type {
  EndShape,
  StyleTarget,
  WireStyle,
  WireStyleSheet,
} from "./model/style-types.js";
export { PaintRegistry } from "./canvas/render/PaintRegistry.js";

// Extension points. Registering a renderer or swapping an algorithm is the
// supported way to change how a diagram looks, without forking the canvas.
export { TypeRegistry } from "./canvas/render/TypeRegistry.js";
export type { ElementRenderer, RenderContext } from "./canvas/render/ElementRenderer.js";
export { BoxRenderer } from "./canvas/render/BoxRenderer.js";
export { ContainerRenderer } from "./canvas/render/ContainerRenderer.js";

export {
  CenterPortAssigner,
  DiscretePortAssigner,
  UniformPortAssigner,
} from "./canvas/ports/assigners.js";
export type { PortAssigner, PortRequest } from "./canvas/ports/PortAssigner.js";

export { BezierRouter } from "./canvas/routing/EdgeRouter.js";
export type { EdgeRouter, Route, RouteRequest } from "./canvas/routing/EdgeRouter.js";

export { exportDrawio } from "./editor/io/drawio.js";
export { HttpModelStore, HttpStyleStore } from "./editor/io/transfer.js";
export type { ModelStore, SaveTarget, StyleStore } from "./editor/io/transfer.js";

export type { BoundarySlot, Point, Rect, Side } from "./geometry/types.js";

// Workbench Architecture components
export { Workbench } from "./workbench/Workbench.js";
export { CommandRegistry } from "./workbench/commands/CommandRegistry.js";
export { ShortcutManager } from "./workbench/commands/ShortcutManager.js";
export { DockviewHost } from "./workbench/dockview/DockviewHost.js";
export { PanelService } from "./workbench/dockview/PanelService.js";
export { WorkspaceLayoutService } from "./workbench/dockview/WorkspaceLayoutService.js";
export { Ribbon } from "./workbench/ribbon/Ribbon.js";
export * from "./workbench/commands/types.js";
export * from "./workbench/ribbon/types.js";

// Global Layout & Geometry Configuration
export { DIAGRAM_CONFIG, type DiagramConfig } from "./constants/diagram-constants.js";

// Block content templates (ADR_20260903 §2.3). Compiler only — no renderer
// yet; the tree it produces is not wired into DiagramCanvas.
export { compileTemplate } from "./content/parser.js";
export { DirectiveRegistry, createDefaultDirectiveRegistry } from "./content/directive-registry.js";
export type { DirectiveSignature, PositionalShape } from "./content/directive-registry.js";
export type {
  CompileResult,
  DirectiveArgs,
  DirectiveNode,
  GeometryArgs,
  SizeValue,
  TemplateCell,
  TemplateError,
  TemplateRow,
  TemplateTree,
} from "./content/template-types.js";
