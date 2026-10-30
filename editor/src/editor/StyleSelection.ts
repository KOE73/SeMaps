/** Selection survives a newer stylesheet arriving before its rename event. */
export class StyleSelection {
  id: string | null = null;
  private missingId: string | null = null;

  open(id: string | null): void {
    this.id = id;
    this.missingId = null;
  }

  reconcile(has: (id: string) => boolean): void {
    if (this.id !== null && !has(this.id)) {
      this.missingId = this.id;
      this.id = null;
    } else if (this.missingId !== null && has(this.missingId)) {
      this.open(this.missingId);
    }
  }

  rename(oldId: string, newId: string): boolean {
    if (this.id !== oldId && this.missingId !== oldId) return false;
    this.open(newId);
    return true;
  }

  remove(id: string): void {
    if (this.id === id || this.missingId === id) this.open(null);
  }
}
