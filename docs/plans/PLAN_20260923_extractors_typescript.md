# PLAN_20260923_extractors_typescript — Извлекатель для TypeScript: образец «на самом себе»

Статус: **в работе**.

Зачем: извлекатель C# нужен практически, этот — методологически. Второй язык показывает,
что в [`EXTRACTOR.md`](../EXTRACTOR.md) общее, а что случайно «по-сишарповски»; и это
единственный извлекатель, который можно прогнать на самом SeMaps (`editor/src`) в CI.
Он же — образец для тех, кто захочет написать извлекатель для своего языка.

Нормативные документы: [`EXTRACTOR.md`](../EXTRACTOR.md) (контракт вывода),
[`ADR_20260923-5`](../adr/ADR_20260923-5_extractors_symbol-ids.md) (форма `id`),
[`ADR_20260923-6`](../adr/ADR_20260923-6_extractors_all-declarations-with-visibility.md)
(все объявления, поле `visibility`),
[`schemas/extractor-facts.schema.json`](../../schemas/extractor-facts.schema.json) (схема).
Расхождение кода с ними — баг кода, а не повод править документ молча.

## Техническое задание

### Расположение и запуск

- Каталог `extractors/typescript/`, свой `package.json` (`"type": "module"`, `private`),
  Node 24, TypeScript ^5.9. Зависимость только `typescript` (компилятор как библиотека);
  никаких `ts-morph`, `@babel/*`. Валидатор схемы в тестах — `ajv` как dev-зависимость.
- Команда `semaps-extract-typescript` через `bin` в `package.json` → `dist/cli.js`.
  Локальный запуск без установки: `node extractors/typescript/dist/cli.js`.
- Аргументы ровно по §1: `--root <dir>` (по умолчанию cwd), `--include <path>`… ,
  `--exclude <glob>`… . Коды выхода 0/1/2 по §1. stdout — только JSON; всё остальное в stderr.
- Программу строит `ts.createProgram` по файлам из `--include` (или всего корня), с
  `tsconfig.json` ближайшего к корню, если он есть; иначе дефолтные опции. Нужны
  **разрешённые символы** (`TypeChecker`), не синтаксис: `extends Foo` должен разрешаться в
  объявление, а не в текст `Foo`.

### Исключения по умолчанию

`node_modules/`, `dist/`, `build/`, `*.d.ts`, `*.test.ts`, `*.spec.ts`, `vite-env.d.ts`,
всё вне `.ts`/`.tsx`/`.mts`. `--exclude` добавляет поверх.

### Символы

| Объявление TS | `kind` | `nativeKind` |
|---|---|---|
| файл `.ts` | `module` | `file` |
| `class`, `abstract class` | `type` | `class` / `abstract-class` |
| `interface` | `interface` | `interface` |
| `type X = …` | `type` | `type-alias` |
| `enum`, `const enum` | `type` | `enum` |
| `function`, `export default function` | `function` | `function` |
| `export const x = …` (не функция) | `value` | `const` |
| `export const x = () => …` / `function`-значение | `function` | `arrow-function` |
| `namespace N` | `module` | `namespace` |

- **Все** объявления верхнего уровня файла и внутри `namespace` (ADR-6): экспортированные —
  `visibility: "exported"`, остальные — `visibility: "file"`. `export { a, b }` в конце файла
  делает `a` и `b` экспортированными. Ре-экспорт `export * from`/`export { x } from` символов
  не порождает. Символ файла — без `visibility`.
- `id`, `name`, `file`, `line` по ADR-5 и §2.1. `namespace` — пусто для верхнего уровня файла,
  имя `namespace` для вложенных.
- Перегрузки функции — один символ. Объявление в `.ts` и `.tsx` с одним именем в одном
  каталоге — это разные модули, разные `id`.
- `members`: для `enum` — все члены (`kind: "value"`, `type` — значение, если литерал); для
  `interface` — все члены с сигнатурой в `type`; для `class` — **все** поля, свойства и методы
  (`kind`: `field` / `property` / `method`; `visibility`: `public` / `protected` / `private`,
  `#`-члены — `private`). `type` — как печатает `checker.typeToString`.

### Рёбра

- `contains`: файл → каждое его объявление; `namespace` → вложенные.
- `extends`: `class A extends B`, `interface A extends B, C`.
- `implements`: `class A implements I`.
- `references`: тип члена (поле, свойство, параметр/возврат метода), тип `type`-alias,
  **собственная сигнатура `function`** (параметры, возврат, включая arrow-функции) и
  **аннотация типа `value`** упоминают символ **из этого же вывода** (EXTRACTOR §2.2). Generic-аргументы разворачиваются (`Map<K, Foo>` → ребро на
  `Foo`). Все `A → B` схлопываются в одно ребро. Импорты сами по себе рёбер **не** дают.
- Ребро на символ вне вывода (библиотека, `lib.dom.d.ts`, исключённый файл) не печатается.

### Детерминизм

`symbols` отсортированы по `id`, `edges` по `(from, to, kind)`, ключи объектов в порядке
схемы, отступ два пробела, `\n` в конце. Тест: два прогона — один байт в байт вывод.

### Тесты

- `extractors/typescript/testdata/` — маленький проект: два файла с `class`/`interface`/
  `enum`/`type`/`function`, `extends`+`implements`, `namespace`, не экспортированный класс
  (не должен попасть), `references` через generic, ре-экспорт. Рядом `expected.json`.
- Тесты на `node --test` (без jest/vitest): вывод равен `expected.json`; вывод валиден по
  `schemas/extractor-facts.schema.json`; детерминизм; коды выхода (`--root` на несуществующий
  каталог → 1 и пустой stdout; неизвестный флаг → 2).
- Скрипты: `npm run build`, `npm test`, `npm run self` = прогон на `../../editor/src` с
  `--root ../..` и `--include editor/src`, вывод в stdout. `self` должен проходить без
  ошибок; его размер и число `references` записать в README.

### Не делать

- Не писать файлы, не трогать `examples/`, не знать о реестре (§4).
- Не менять `EXTRACTOR.md`, ADR, схему. Если контракт не покрывает случай — записать в
  раздел «Открытое» этого плана и выбрать самое узкое поведение.
- Не править `.github/workflows/ci.yml` — шаг CI добавляется отдельно.
- Не коммитить.

## Шаги

1. Каркас: `package.json`, `tsconfig.json`, `src/cli.ts` (аргументы, коды выхода), `README.md`.
2. `src/extract.ts`: программа, обход файлов, символы.
3. Рёбра и `members`.
4. `testdata/` + `expected.json` + тесты.
5. `npm run self` на `editor/src`; числа в README; открытые вопросы сюда.

## Открытое

Контракт (`EXTRACTOR.md`, ADR-5, схема) не покрывал следующие случаи; выбрано самое узкое
поведение, код и тесты (`testdata/`) следуют ему:

1. **`name` файла-модуля.** Не задано явно нигде. Взято имя файла без расширения
   (`editor/src/a.ts` → `name: "a"`), как «короткое имя, как в коде» по аналогии с прочими
   символами.
2. **`namespace` всегда присутствует**, включая символ файла и символы верхнего уровня — пустой
   строкой, а не отсутствует как поле. Буквально по «namespace — пусто для верхнего уровня
   файла» (§Символы): «пусто», не «нет поля».
3. **`export default`.** Суффикс `id` — всегда `default` (и для анонимного, и для именованного
   `export default class Foo {}`), поле `name` — локальное имя, если оно есть, иначе `default`.
   ADR-5 даёт пример только для анонимной функции; для именованного случая суффикс `id` выбран
   таким же, потому что именно `default` — это то имя, под которым модуль отдаёт экспорт
   (`import X from …` не видит локальное имя).
4. **`export default <identifier>;`** (отдельным `ExportAssignment`, не частью объявления) —
   поддержан только простой случай, когда `<identifier>` — имя объявления в том же файле:
   такое объявление считается экспортированным под `default`, даже если у него самого нет
   `export`. Любое другое выражение после `export default` (объектный литерал, вызов и т.п.) и
   `export = X` не порождают символ — совпадающего `kind`/`nativeKind` в таблице нет.
5. **`export { a as b }`.** Символ строится по исходному имени `a`, alias `b` не влияет ни на
   `id`, ни на `name`. Явно не описано; описан только случай без переименования
   («`export { a, b }` в конце файла считается экспортом `a` и `b`»).
6. **Закрыто ADR-6.** `references` теперь и из собственной сигнатуры `function`/arrow-function
   и из аннотации типа `value`, не только из members и `type`-alias — реализовано.
7. **Закрыто ADR-6.** Печатаются все члены класса с `visibility: public/protected/private`
   (в т.ч. `#`-члены — `private`), не только публичные/protected — реализовано.
8. **Не обрабатываются:** index/call/construct-сигнатуры интерфейсов (нет простого `name`),
   деструктурирующие паттерны в `const`/`let` (`const {a, b} = …`). Тихо пропускаются —
   ни как символ, ни как источник `references`.
9. **`tsconfig.json` ищется только прямо в `--root`**, без подъёма по родителям и без
   отдельного tsconfig на `--include`-подкаталог.
10. **`members` enum: `type`** — печатается через `String(value)` числового или строкового
    константного значения (`checker.getConstantValue`), без кавычек у строк (т.е. `"green"`, а
    не `"\"green\"""`). Не литеральное значение (вычисляемое выражение) — поле `type` не
    печатается.
11. **Имя `#private`-члена класса** печатается с ведущим `#` (`"#password"`), как оно и написано
    в коде — private identifier это часть имени, не модификатор поверх обычного имени.
12. **`references` из сигнатуры `function`/`value`** берутся только из явных аннотаций типа в
    AST (как и у членов класса/интерфейса), не из выведенного checker'ом типа: функция без
    аннотации возврата (`function f(x: Foo) { return x; }`) даёт ребро от параметра, но не от
    невыраженного возврата.
