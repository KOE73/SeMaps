import type {
  CommandDefinition,
  CommandState,
  CommandContext,
  Disposable,
} from "./types.js";

export class CommandRegistry {
  private readonly commands = new Map<string, CommandDefinition>();
  private readonly listeners = new Set<() => void>();
  private contextProvider: (() => CommandContext) | null = null;

  setContextProvider(provider: () => CommandContext): void {
    this.contextProvider = provider;
  }

  getContext(): CommandContext {
    if (this.contextProvider === null) {
      throw new Error("CommandRegistry contextProvider has not been set");
    }
    return this.contextProvider();
  }

  register(def: CommandDefinition): Disposable {
    if (this.commands.has(def.id)) {
      console.warn(`Command "${def.id}" is already registered. Overwriting.`);
    }
    this.commands.set(def.id, def);
    this.notifyStateChanged();
    return () => this.unregister(def.id);
  }

  registerAll(defs: readonly CommandDefinition[]): Disposable {
    for (const def of defs) {
      this.commands.set(def.id, def);
    }
    this.notifyStateChanged();
    return () => {
      for (const def of defs) {
        this.unregister(def.id);
      }
    };
  }

  unregister(id: string): void {
    if (this.commands.delete(id)) {
      this.notifyStateChanged();
    }
  }

  get(id: string): CommandDefinition | undefined {
    return this.commands.get(id);
  }

  has(id: string): boolean {
    return this.commands.has(id);
  }

  list(): readonly CommandDefinition[] {
    return Array.from(this.commands.values());
  }

  execute(id: string, args?: unknown): Promise<void> | void {
    const cmd = this.commands.get(id);
    if (cmd === undefined) {
      console.error(`Cannot execute unknown command "${id}"`);
      return;
    }
    const context = this.getContext();
    if (cmd.isEnabled && !cmd.isEnabled(context)) {
      console.warn(`Command "${id}" is disabled in the current context`);
      return;
    }
    const res = cmd.execute(context, args);
    this.notifyStateChanged();
    return res;
  }

  getState(id: string, context?: CommandContext): CommandState {
    const cmd = this.commands.get(id);
    const ctx = context ?? this.getContext();
    if (cmd === undefined) {
      return {
        enabled: false,
        visible: false,
        title: id,
      };
    }
    const enabled = cmd.isEnabled ? cmd.isEnabled(ctx) : true;
    const visible = cmd.isVisible ? cmd.isVisible(ctx) : true;
    const checked = cmd.isChecked ? cmd.isChecked(ctx) : undefined;

    return {
      enabled,
      visible,
      checked,
      title: cmd.title,
      description: cmd.description,
      icon: cmd.icon,
      shortcut: cmd.shortcut,
      keyTip: cmd.keyTip,
      category: cmd.category,
    };
  }

  notifyStateChanged(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }

  onStateChanged(listener: () => void): Disposable {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }
}
