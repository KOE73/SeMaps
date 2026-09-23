/**
 * Undo history over whole-model snapshots.
 *
 * Snapshots are the serialized wire form, which is what the original did and
 * what keeps restoring simple: parsing a snapshot rebuilds the containment tree
 * from scratch, so no half-updated object can survive an undo.
 */
export class History {
  private stack: string[] = [];
  private index = -1;
  /** Snapshot taken when a gesture began, held until the gesture ends. */
  private pending: string | null = null;

  constructor(private readonly limit = 50) {}

  /** Start a fresh history for a newly loaded model. */
  reset(snapshot: string): void {
    this.stack = [snapshot];
    this.index = 0;
    this.pending = null;
  }

  /** Remember the state before a gesture, so it can be committed if it changed. */
  begin(snapshot: string): void {
    this.pending = snapshot;
  }

  /**
   * Commit the pending gesture, but only if it actually changed something —
   * a drag that ends where it started leaves no step to undo (R-HIST-06).
   */
  end(current: string): boolean {
    const before = this.pending;
    this.pending = null;
    if (before === null || before === current) return false;
    this.push(current);
    return true;
  }

  push(snapshot: string): void {
    if (this.stack[this.index] === snapshot) return;
    if (this.index < this.stack.length - 1) {
      this.stack = this.stack.slice(0, this.index + 1);
    }
    this.stack.push(snapshot);
    if (this.stack.length > this.limit) this.stack.shift();
    this.index = this.stack.length - 1;
  }

  undo(): string | null {
    if (!this.canUndo) return null;
    this.index -= 1;
    return this.stack[this.index] ?? null;
  }

  redo(): string | null {
    if (!this.canRedo) return null;
    this.index += 1;
    return this.stack[this.index] ?? null;
  }

  get canUndo(): boolean {
    return this.index > 0;
  }

  get canRedo(): boolean {
    return this.index < this.stack.length - 1;
  }
}
