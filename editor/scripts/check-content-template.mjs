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
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const ROOT = new URL("..", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

async function loadModule() {
  const result = await build({
    stdin: {
      contents: 'export * from "./src/content/index.ts"; export { renderContent } from "./src/content/ContentRenderer.ts";',
      resolveDir: ROOT,
      loader: "ts",
    },
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

const { compileTemplate, createDefaultDirectiveRegistry, renderContent } = await loadModule();

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

// Rendering uses a deterministic proportional-font metric here; browser QA
// separately verifies real font measurement and applying the template in UI.
globalThis.document = {
  createElement: () => ({ getContext: () => ({
    measureText: (value) => ({ width: Array.from(value).reduce((sum, char) => sum + (char === "W" ? 15 : 5), 0) }),
  }) }),
  createElementNS: (_, tag) => ({
    tag, attrs: {}, children: [], textContent: "",
    setAttribute(name, value) { this.attrs[name] = value; },
    removeAttribute(name) { delete this.attrs[name]; },
    appendChild(child) { this.children.push(child); },
  }),
};
const look = { family: "sans-serif", size: 10, weight: 400, italic: false, color: "#123", opacity: 1, show: true, align: "middle" };
const style = { title: look, subtitle: look };
const render = (source, description, width = 100, customStyle = style) => {
  const { tree, errors } = compileTemplate(source);
  assert(errors.length === 0, `ошибка шаблона: ${JSON.stringify(errors)}`);
  return renderContent(tree, { x: 0, y: 0, width, height: 80 }, customStyle, { name: "Stage", description }, { x: 0, top: 0 });
};

testCase("описание переносится целыми словами; высота учитывает все строки", () => {
  const drawn = render("@Description(wrap)", "decode video to CUDA NV12", 50);
  assert(drawn.nodes.map((n) => n.textContent).join("|") === "decode|video to|CUDA NV12", "перенос по словам");
  assert(drawn.usedHeight === 3 * 13.5 + 3, "высота трёх строк с отступом");
});

testCase("лимит описания обозначает скрытый текст многоточием", () => {
  const drawn = render("@Description(wrap, maxLines=2)", "decode video to CUDA NV12", 50);
  assert(drawn.nodes.length === 2 && drawn.nodes[1].textContent === "video to…", "две строки, остаток обозначен");
});

testCase("явные переносы и длинные Unicode-идентификаторы сохраняются", () => {
  const drawn = render("@Description(wrap, maxLines=10)", "CUDA\n😀😀😀😀😀😀\nNV12", 20);
  assert(drawn.nodes.map((n) => n.textContent).join("|") === "CUDA|😀😀😀😀|😀😀|NV12", "без потери символов или разрыва surrogate pair");
});

testCase("широкие символы учитываются по метрике шрифта", () => {
  const drawn = render("@Description(wrap)", "WWW", 30);
  assert(drawn.nodes.map((n) => n.textContent).join("|") === "WW|W", "ширина W больше средней");
});

testCase("пустое описание не занимает места, старый @Description остаётся однострочным", () => {
  assert(render("@Name\n@Description(wrap)", " \n ").usedHeight === render("@Name", "").usedHeight, "пустая секция скрыта");
  assert(render("@Description", "decode video to CUDA NV12", 50).nodes.length === 1, "старый шаблон не меняет переносы");
});

testCase("шаблон явно показывает описание при скрытом типе, и наследует выравнивание имени", () => {
  const templates = JSON.parse(readFileSync(join(ROOT, "../host/defaults/templates.json"), "utf8")).templates;
  const template = templates.find((t) => t.id === "title-description");
  assert(template !== undefined, "новый шаблон в реестре");
  const drawn = render(template.lines.join("\n"), "CUDA NV12", 100, { title: look, subtitle: { ...look, show: false } });
  assert(drawn.nodes.length === 2, "видны заголовок и описание");
  assert(drawn.nodes[0].attrs.x === "50" && drawn.nodes[0].attrs["text-anchor"] === "middle", "имя по центру, как в стиле");
  assert(drawn.nodes[1].attrs.x === "0" && drawn.nodes[1].attrs["text-anchor"] === "start", "описание слева");
});

testCase("невидимое описание и неверный лимит обрабатываются без ошибки", () => {
  assert(render("@Description(wrap, show=false)", "CUDA").nodes.length === 0, "явно скрыто");
  for (const limit of ["0", "-1", "oops", "1.5"]) {
    const drawn = render(`@Description(wrap, maxLines=${limit})`, "one two three four five", 20);
    assert(drawn.nodes.length === 3 && drawn.nodes[2].textContent.endsWith("…"), `fallback 3 для ${limit}`);
  }
});

testCase("описание не выходит за уменьшенный блок; измерение сохраняет естественную высоту", () => {
  const { tree } = compileTemplate("@Name\n@Description(wrap)");
  const box = { x: 0, y: 0, width: 50, height: 48 };
  const data = { name: "Stage", description: "decode video to CUDA NV12" };
  const drawn = renderContent(tree, box, style, data, { x: 0, top: 0, fitHeight: true });
  assert(drawn.nodes.length === 3 && drawn.nodes[2].textContent.endsWith("…"), "заголовок и две строки с многоточием");
  assert(drawn.usedHeight <= box.height, "высота в границах блока");
  const measured = renderContent(tree, box, style, data, { x: 0, top: 0 });
  assert(measured.nodes.length === 4 && measured.usedHeight > box.height, "измерение для увеличения блока учитывает три строки");
});

console.log(`\n${passed} пройдено, ${failures} провалено`);
process.exit(failures > 0 ? 1 : 0);
