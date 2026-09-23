export type Listener<T> = (payload: T) => void;
export type Unsubscribe = () => void;

/**
 * A minimal typed event bus.
 *
 * The canvas reports what happened and never reaches out to the editor, which
 * is what lets `DiagramCanvas` be used on its own.
 */
export class Emitter<Events> {
  private readonly listeners = new Map<keyof Events, Set<Listener<never>>>();

  on<K extends keyof Events>(event: K, listener: Listener<Events[K]>): Unsubscribe {
    let set = this.listeners.get(event);
    if (set === undefined) {
      set = new Set();
      this.listeners.set(event, set);
    }
    set.add(listener as Listener<never>);
    return () => {
      set?.delete(listener as Listener<never>);
    };
  }

  emit<K extends keyof Events>(event: K, payload: Events[K]): void {
    const set = this.listeners.get(event);
    if (set === undefined) return;
    for (const listener of [...set]) {
      (listener as Listener<Events[K]>)(payload);
    }
  }

  clear(): void {
    this.listeners.clear();
  }
}
