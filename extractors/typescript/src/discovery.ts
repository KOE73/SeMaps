import * as fs from "node:fs";
import * as path from "node:path";

const SOURCE_EXTENSIONS = [".ts", ".tsx", ".mts"];

const DEFAULT_EXCLUDE_DIRS = new Set(["node_modules", "dist", "build"]);

function isDefaultExcludedName(baseName: string): boolean {
  if (baseName.endsWith(".d.ts")) return true;
  if (baseName.endsWith(".test.ts") || baseName.endsWith(".test.tsx")) return true;
  if (baseName.endsWith(".spec.ts") || baseName.endsWith(".spec.tsx")) return true;
  if (baseName === "vite-env.d.ts") return true;
  return false;
}

/** Convert a shell-style glob (`*`, `**`, `?`) into a RegExp matched against a forward-slash relative path. */
function globToRegExp(glob: string): RegExp {
  let out = "";
  const normalized = glob.replace(/\\/g, "/");
  for (let i = 0; i < normalized.length; i++) {
    const c = normalized[i];
    if (c === "*") {
      if (normalized[i + 1] === "*") {
        out += ".*";
        i++;
        if (normalized[i + 1] === "/") i++;
      } else {
        out += "[^/]*";
      }
    } else if (c === "?") {
      out += "[^/]";
    } else if (".+^${}()|[]\\".includes(c)) {
      out += "\\" + c;
    } else {
      out += c;
    }
  }
  return new RegExp(`^${out}$`);
}

export interface DiscoveryOptions {
  root: string;
  include: string[];
  exclude: string[];
}

/** Returns absolute file paths of TypeScript sources to extract, sorted. */
export function discoverFiles(opts: DiscoveryOptions): string[] {
  const excludeRegexes = opts.exclude.map(globToRegExp);

  const isExcludedByGlob = (relPath: string): boolean =>
    excludeRegexes.some((re) => re.test(relPath));

  const results: string[] = [];

  const visitFile = (absPath: string): void => {
    const relPath = path.relative(opts.root, absPath).split(path.sep).join("/");
    const baseName = path.basename(absPath);
    if (!SOURCE_EXTENSIONS.some((ext) => absPath.endsWith(ext))) return;
    if (isDefaultExcludedName(baseName)) return;
    if (isExcludedByGlob(relPath)) return;
    results.push(absPath);
  };

  const visitDir = (absDir: string): void => {
    const baseName = path.basename(absDir);
    if (DEFAULT_EXCLUDE_DIRS.has(baseName)) return;
    const relPath = path.relative(opts.root, absDir).split(path.sep).join("/");
    if (relPath && isExcludedByGlob(relPath)) return;
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(absDir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const childAbs = path.join(absDir, entry.name);
      if (entry.isDirectory()) {
        visitDir(childAbs);
      } else if (entry.isFile()) {
        visitFile(childAbs);
      }
    }
  };

  const roots = opts.include.length > 0 ? opts.include : ["."];
  for (const rel of roots) {
    const abs = path.resolve(opts.root, rel);
    let stat: fs.Stats;
    try {
      stat = fs.statSync(abs);
    } catch {
      continue;
    }
    if (stat.isDirectory()) {
      visitDir(abs);
    } else if (stat.isFile()) {
      visitFile(abs);
    }
  }

  return Array.from(new Set(results)).sort();
}
