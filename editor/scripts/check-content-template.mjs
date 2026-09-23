#!/usr/bin/env node
/**
 * Self-check for the content-template compiler (`src/content/`, stream B of
 * PLAN_20260903). No test runner in this package (see `check-model.mjs` for
 * precedent) — this is a script that bundles the module with esbuild,
 * imports it, runs a table of cases and fails loudly.
 *
 * The one thing every case is really checking: `compileTemplate` never
 * throws, and a broken template still yields a tree plus a non-empty
 * `errors[]` — that is the entire acceptance bar from the ADR.
 */

import { build } from "esbuild";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const ROOT = new URL("..", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

async function loadModule() {
  const result = await build({
    entryPoints: [join(ROOT, "src/content/index.ts")],
    bundle: true,
    write: false,
    format: "esm",
    platform: "neutral",
    target: "es2022",
  });
  const code = result.outputFiles[0].text;
  const dir = mkdtempSync(join(tmpdir(), "semaps-content-check-"));
  const file = join(dir, "content-bundle.mjs");
  writeFileSync(file, code, "utf8");
  return import(pathToFileURL(file).href);
}

const { compileTemplate, createDefaultDirectiveRegistry } = await loadModule();

let failures = 0;
let passed = 0;

/** Runs `check`, which throws on a failed assertion; reports pass/fail. */
function testCase(name, check) {
  try {
    check();
    passed++;
  } catch (err) {
    failures++;
    console.log(`FAIL  ${name}`);
    console.log(`      ${err.message}`);
  }
}

function assert(cond, message) {
  if (!cond) throw new Error(message);
}

// -------------------------------------------------------------- well-formed

testCase("простой @Name — одна строка, одна ячейка, без ошибок", () => {
  const { tree, errors } = compileTemplate("@Name");
  assert(errors.length === 0, `неожиданные ошибки: ${JSON.stringify(errors)}`);
  assert(tree.rows.length === 1, "ожидалась одна строка");
  assert(tree.rows[0].cells.length === 1, "ожидалась одна ячейка");
  assert(tree.rows[0].cells[0].name === "Name", "имя директивы");
});

testCase("многострочный шаблон из ADR — стек и без ошибок", () => {
  const text = "@Name\n@Members([name, type], where=kind:field)\n@Asset(brain, h=300, w=50%)";
  const { tree, errors } = compileTemplate(text);
  assert(errors.length === 0, `неожиданные ошибки: ${JSON.stringify(errors)}`);
  assert(tree.rows.length === 3, "три строки");
  const members = tree.rows[1].cells[0];
  assert(Array.isArray(members.args.positional), "positional должен быть списком");
  assert(members.args.positional.join(",") === "name,type", "содержимое списка");
  assert(members.args.named.where === "kind:field", "именованный аргумент where");
  const asset = tree.rows[2].cells[0];
  assert(asset.args.positional === "brain", "positional @Asset");
  assert(asset.args.geometry.h?.kind === "px" && asset.args.geometry.h.value === 300, "h=300 -> px");
  assert(asset.args.geometry.w?.kind === "percent" && asset.args.geometry.w.value === 50, "w=50% -> percent");
});

testCase("колонки через | в одной строке", () => {
  const { tree, errors } = compileTemplate("@Members(where=kind:property) | @Members(where=kind:method)");
  assert(errors.length === 0, `неожиданные ошибки: ${JSON.stringify(errors)}`);
  assert(tree.rows[0].cells.length === 2, "две колонки");
  assert(tree.rows[0].cells[0].args.named.where === "kind:property", "первая колонка");
  assert(tree.rows[0].cells[1].args.named.where === "kind:method", "вторая колонка");
});

testCase("пустая строка — spacer, не ошибка", () => {
  const { tree, errors } = compileTemplate("@Name\n\n@Description");
  assert(errors.length === 0, "пустая строка не должна порождать ошибок");
  assert(tree.rows.length === 3, "три строки, включая пустую");
  assert(tree.rows[1].spacer === true, "средняя строка — spacer");
  assert(tree.rows[1].cells.length === 0, "у spacer нет ячеек");
});

testCase("флаг без значения — @Members(collapsed)", () => {
  const { tree, errors } = compileTemplate("@Members(collapsed)");
  assert(errors.length === 0, `неожиданные ошибки: ${JSON.stringify(errors)}`);
  const node = tree.rows[0].cells[0];
  assert(node.args.positional === undefined, "collapsed не должен стать позиционным списком");
  assert(node.args.named.collapsed === true, "collapsed должен быть флагом");
});

testCase("директива без скобок вообще", () => {
  const { errors } = compileTemplate("@Description");
  assert(errors.length === 0, "@Description без аргументов — валиден");
});

// ----------------------------------------------------------------- ошибки

testCase("неизвестная директива — ошибка, но не исключение", () => {
  const { tree, errors } = compileTemplate("@Frobnicate(x)");
  assert(errors.some((e) => e.message.includes("неизвестная директива")), "должна быть ошибка о неизвестной директиве");
  assert(tree.rows.length === 1, "дерево всё равно строится");
});

testCase("незакрытая скобка не роняет компилятор", () => {
  const { tree, errors } = compileTemplate("@Members([name, type)");
  assert(errors.length > 0, "должна быть хотя бы одна ошибка");
  assert(tree.kind === "template", "дерево возвращается даже на кривом тексте");
});

testCase("незакрытая внешняя скобка директивы", () => {
  const { errors } = compileTemplate("@Asset(brain");
  assert(
    errors.some((e) => e.message.includes("скобк")),
    "ошибка должна упоминать скобку",
  );
});

testCase("два позиционных аргумента — ошибка", () => {
  const { errors } = compileTemplate("@Members([a, b], [c, d])");
  assert(
    errors.some((e) => e.message.includes("уже есть основной аргумент")),
    "второй позиционный аргумент должен быть отклонён",
  );
});

testCase("директива без позиционного аргумента получает список — ошибка", () => {
  // A bare word after @Name (shape "none") is legal — it becomes a flag, the
  // same way @Members(collapsed) does; flags are unrestricted named args.
  // What "none" actually forbids is the *positional* slot, which only a
  // bracketed list or (for shape "value"/"either") a claimed bare word fills.
  const { errors } = compileTemplate("@Name([oops])");
  assert(
    errors.some((e) => e.message.includes("не принимает основной аргумент")),
    "@Name не принимает основной аргумент",
  );
});

testCase("директива без позиционного аргумента: одинокий флаг — не ошибка", () => {
  const { tree, errors } = compileTemplate("@Name(oops)");
  assert(errors.length === 0, `голый флаг у @Name не должен быть ошибкой: ${JSON.stringify(errors)}`);
  assert(tree.rows[0].cells[0].args.named.oops === true, "oops должен стать флагом");
});

testCase("обязательный аргумент отсутствует — ошибка", () => {
  const { errors } = compileTemplate("@Asset()");
  assert(
    errors.some((e) => e.message.includes("нужен основной аргумент")),
    "@Asset без аргумента должен жаловаться",
  );
});

testCase("мусорное значение w — ошибка, но директива не пропадает", () => {
  const { tree, errors } = compileTemplate("@Asset(brain, w=банан)");
  assert(errors.some((e) => e.message.includes('"w"')), "ошибка должна называть w");
  assert(tree.rows[0].cells.length === 1, "директива всё равно попадает в дерево");
});

testCase("текст целиком мусорный — не бросает исключение", () => {
  let threw = false;
  let result;
  try {
    result = compileTemplate("]][[ @@ (((( )))) | | @Name(");
  } catch {
    threw = true;
  }
  assert(!threw, "compileTemplate не должен бросать исключение");
  assert(result.errors.length > 0, "мусорный текст должен дать хотя бы одну ошибку");
  assert(result.tree.kind === "template", "дерево возвращается даже на полном мусоре");
});

testCase("реестр директив расширяется извне", () => {
  const registry = createDefaultDirectiveRegistry();
  registry.register({ name: "Icon", description: "тест", positional: { shape: "value", required: true } });
  const { errors } = compileTemplate("@Icon(key)", registry);
  assert(errors.length === 0, "новая директива должна компилироваться без ошибок");
});

console.log(`\n${passed} пройдено, ${failures} провалено`);
process.exit(failures > 0 ? 1 : 0);
