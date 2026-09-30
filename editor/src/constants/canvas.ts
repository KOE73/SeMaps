/**
 * The canvas numbers — grid, default and minimum sizes, container header and
 * padding, gaps. One source: the host's `/canvas.json` (workspace copy wins over
 * the tool default). Code needs them synchronously, so the app loads the file
 * before the editor is built (`loadCanvas`) and everything else calls `canvas()`.
 */

export interface CanvasNumbers {
  readonly grid: number;
  readonly node: { readonly width: number; readonly height: number; readonly minWidth: number; readonly minHeight: number; readonly radius: number };
  readonly container: { readonly minWidth: number; readonly minHeight: number; readonly headerHeight: number; readonly padding: number; readonly radius: number };
  readonly gap: { readonly node: number; readonly container: number };
}

/**
 * No-host fallback, for the library build only, where no host serves
 * `/canvas.json`. The same values as the host's `defaults/canvas.json`.
 */
const NO_HOST_FALLBACK: CanvasNumbers = {
  grid: 10,
  node: { width: 180, height: 60, minWidth: 100, minHeight: 40, radius: 8 },
  container: { minWidth: 160, minHeight: 100, headerHeight: 28, padding: 16, radius: 10 },
  gap: { node: 40, container: 40 },
};

let current: CanvasNumbers = NO_HOST_FALLBACK;

/** The numbers in force. */
export function canvas(): CanvasNumbers {
  return current;
}

/** Read `canvas.json` from the host; without one (or with a broken one) the fallback stays. */
export async function loadCanvas(base: string): Promise<void> {
  try {
    const response = await fetch(`${base}canvas.json`);
    if (!response.ok) return;
    const got = (await response.json()) as { grid?: number; node?: Partial<CanvasNumbers["node"]>; container?: Partial<CanvasNumbers["container"]>; gap?: { node?: number; container?: number } };
    current = {
      grid: got.grid ?? current.grid,
      node: { ...current.node, ...got.node },
      container: { ...current.container, ...got.container },
      gap: { node: got.gap?.node ?? current.gap.node, container: got.gap?.container ?? current.gap.container },
    };
  } catch {
    /* no host: keep the fallback */
  }
}
