const PUNCT: Record<string, string> = {
  Minus: "-",
  Equal: "=",
  Quote: "'",
  Comma: ",",
  Period: ".",
  Slash: "/",
  Semicolon: ";",
  BracketLeft: "[",
  BracketRight: "]",
  Backslash: "\\",
  Backquote: "`",
};

/**
 * The key as it sits on a US keyboard, lower case: `z` for the key marked Z/Я
 * whatever layout is active. Shortcuts are written in Latin ("Ctrl+Z"), and
 * matching on `e.key` breaks them under any other layout — Ctrl+Я did nothing.
 * Keys with no letter or digit on them (F1, Delete, Escape, arrows) come
 * from `e.key`, which is layout-independent for them anyway.
 */
export function layoutFreeKey(e: KeyboardEvent): string {
  const code = e.code ?? "";
  const letter = /^Key([A-Z])$/.exec(code);
  if (letter) return letter[1]!.toLowerCase();
  const digit = /^(?:Digit|Numpad)(\d)$/.exec(code);
  if (digit) return digit[1]!;
  const punct = PUNCT[code];
  if (punct !== undefined) return punct;
  return e.key.toLowerCase();
}
