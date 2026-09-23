/**
 * Symbol id construction per ADR_20260923-5 ("TypeScript" section):
 *
 *   <module path relative to root, forward slashes, no extension> "#" <dot-joined
 *   path of exported declaration names, innermost last>
 *
 * The module (file) symbol itself has no "#" suffix — its id is just the module path.
 */

const TS_EXTENSIONS = [".tsx", ".mts", ".ts"];

/** Strip a recognised TypeScript extension from a forward-slash relative path. */
export function stripExtension(relPath: string): string {
  for (const ext of TS_EXTENSIONS) {
    if (relPath.endsWith(ext)) {
      return relPath.slice(0, -ext.length);
    }
  }
  return relPath;
}

/** id of the file-as-module symbol. */
export function moduleId(relPathNoExt: string): string {
  return relPathNoExt;
}

/** id of an exported declaration, possibly nested inside one or more `namespace` blocks. */
export function declId(relPathNoExt: string, namespacePath: readonly string[], name: string): string {
  return `${relPathNoExt}#${[...namespacePath, name].join(".")}`;
}
