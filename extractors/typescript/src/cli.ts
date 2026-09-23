#!/usr/bin/env node
import { extract, ExtractError } from "./extract.js";

interface ParsedArgs {
  root: string;
  include: string[];
  exclude: string[];
}

function parseArgs(argv: string[]): ParsedArgs | undefined {
  let root = ".";
  const include: string[] = [];
  const exclude: string[] = [];

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--root") {
      const value = argv[++i];
      if (value === undefined) return undefined;
      root = value;
    } else if (arg === "--include") {
      const value = argv[++i];
      if (value === undefined) return undefined;
      include.push(value);
    } else if (arg === "--exclude") {
      const value = argv[++i];
      if (value === undefined) return undefined;
      exclude.push(value);
    } else {
      return undefined;
    }
  }

  return { root, include, exclude };
}

function main(): void {
  const parsed = parseArgs(process.argv.slice(2));
  if (!parsed) {
    process.stderr.write("usage: semaps-extract-typescript [--root <dir>] [--include <path>]... [--exclude <glob>]...\n");
    process.exitCode = 2;
    return;
  }

  try {
    const facts = extract(parsed);
    process.stdout.write(JSON.stringify(facts, null, 2) + "\n");
    process.exitCode = 0;
  } catch (err) {
    if (err instanceof ExtractError) {
      process.stderr.write(`${err.message}\n`);
    } else {
      process.stderr.write(`${err instanceof Error ? (err.stack ?? err.message) : String(err)}\n`);
    }
    process.exitCode = 1;
  }
}

main();
