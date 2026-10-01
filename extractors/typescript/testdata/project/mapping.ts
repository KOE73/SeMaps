// Rows of "Сопоставление рёбер" (docs/extractors/typescript.md) not covered elsewhere.
export interface Readable {
  read(): string;
}

// interface extends interface: extends, native "interface".
export interface Stream extends Readable {
  close(): void;
}

// Annotated value: uses, native "annotation".
export const defaultStream: Stream = { read: () => "", close: () => {} };
