/**
 * Interaction roles.
 *
 * A renderer marks the parts of its output that mean something to the user:
 * this rectangle is the drag handle, that little box is the resize grip. The
 * interaction controller listens once on the canvas root and looks the role up
 * with `closest()`.
 *
 * That indirection is what makes renderers independent. A renderer registered
 * from outside this library gets dragging, resizing and collapsing by marking
 * its parts — the controller never learns that the renderer exists.
 */

export const ROLE_ATTR = "data-role";
export const ELEMENT_ATTR = "data-element";
export const EDGE_ATTR = "data-edge";
/** Which side or corner a resize grip drags: "n", "se", … */
export const HANDLE_ATTR = "data-handle";

export const Role = {
  /** Selects the element when clicked; does not start a drag. */
  Body: "body",
  /** Pressing here starts moving the element. */
  DragHandle: "drag-handle",
  /** Pressing here starts resizing; double-click fits width. */
  ResizeHandle: "resize-handle",
  /** Releasing here toggles collapse. */
  CollapseToggle: "collapse-toggle",
  /** Releasing here toggles ghost links. */
  GhostToggle: "ghost-toggle",
  /** Releasing here opens the doc editor or shows doc tooltip. */
  DocEdit: "doc-edit",
  /** Releasing here opens the code viewer or shows code preview tooltip. */
  CodeView: "code-view",
} as const;

export type RoleName = (typeof Role)[keyof typeof Role];

export interface RoleHit {
  readonly role: RoleName;
  readonly elementId: string | null;
  readonly edgeId: string | null;
  /** Set on a resize grip: which side or corner it drags. */
  readonly handle: string | null;
}

/** Resolve a DOM event target to the role and element it belongs to. */
export function hitTest(target: EventTarget | null): RoleHit | null {
  if (!(target instanceof Element)) return null;

  const roleEl = target.closest(`[${ROLE_ATTR}]`);
  const ownerEl = target.closest(`[${ELEMENT_ATTR}]`);
  const edgeEl = target.closest(`[${EDGE_ATTR}]`);

  if (roleEl === null && edgeEl === null) return null;

  const role = (roleEl?.getAttribute(ROLE_ATTR) ?? Role.Body) as RoleName;
  return {
    role,
    elementId: ownerEl?.getAttribute(ELEMENT_ATTR) ?? null,
    edgeId: edgeEl?.getAttribute(EDGE_ATTR) ?? null,
    handle: target.closest(`[${HANDLE_ATTR}]`)?.getAttribute(HANDLE_ATTR) ?? null,
  };
}
