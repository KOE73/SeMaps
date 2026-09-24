# PLAN_20260924-2_contract — Членские связи из кода

Статус: **в работе** — не начат.

Решение: [`ADR_20260924-4`](../adr/ADR_20260924-4_contract_member-relations-from-code.md).
Следом — [`PLAN_20260924-3_host_mcp-server`](PLAN_20260924-3_host_mcp-server.md): он ложится
на готовый `core/`.

## Шаги

1. **Папка `docs/extractors/`** в [`agents/documentation.md`](../../agents/documentation.md) —
   своим коммитом (правило этого файла). Затем `docs/extractors/csharp.md` и `typescript.md`:
   таблицы соответствия (кратность, изменяемость, слоты, `memberKind`, что язык не даёт);
   заготовки `go.md`, `python.md`, `rust.md`, `java-kotlin.md` со статусом «извлекателя нет» и
   черновой таблицей из обсуждения.
2. **Контракт.** `EXTRACTOR.md` §1 (`--edges`), §2.2 (виды рёбер, `via`, `edgeKinds`), §3
   (уникальность и сортировка рёбер по `(from, to, kind, member, path)`), §5 (вывод типов из
   признаков, ключ связи, кандидаты на переименование члена, `missing` только по покрытым
   видам). `schemas/extractor-facts.schema.json`. `CONTRACT.md` §4 (`via` у связи, `depends`),
   §5 (типы из кода и их `visibility`), §8.5 (что видимо по умолчанию).
3. **C#-извлекатель.** `ReferenceEdgeCollector` → сбор по членам: обход типа члена с
   накоплением `path` и признаков; семейства коллекций/словарей/отложенных — таблица из
   `docs/extractors/csharp.md`; `--edges`; `depends` для `ProjectReference`; `edgeKinds` в
   заголовке. `expected.json` тестовых данных, тесты.
4. **TS-извлекатель.** То же в `collect.ts`: свойства классов и интерфейсов, `T[]`,
   `readonly T[]`, `ReadonlyArray`, `Map`/`Set`/`Record`, индексные сигнатуры, `x?:`,
   `| undefined | null`, `Promise`, объединения (ребро на каждый тип), кортежи, псевдонимы
   (`memberKind: self`), параметры конструктора. Тесты.
5. **Сверка (`core/`).** Ключ членской связи; вывод типа из признаков; заведение типов со
   стилем-умолчанием и `visibility`; `via` в `relations.json` обновляется; кандидаты
   «переименование?» для связей; `missing` только по `edgeKinds`; отчёт. Миграции — **нет**.
6. **Хост.** `edges` у записи экстрактора в `.semaps` (`projectfile.go`, `PUT /api/setup/
   extractors/{id}`), передача `--edges`; чекбоксы на странице «Экстракторы»; `API.md`.
7. **Стили по умолчанию.** `host/defaults/styles.json`: все типы §3 ADR; `.internal` — тусклее;
   `depends`.
8. **Редактор.** Подпись связи из `via` и кратность на конце из `cardinality`, когда
   авторского текста нет; панель «Связи» показывает `via.text`; ничего не пишет в тексты для
   `origin: code`.
9. **`ADOPTING.md`.** Переход со старой модели (§8 ADR): порядок действий для агента;
   переименование члена; `edges` в `.semaps`; что видно по умолчанию.
10. **Прогон на NeuroModFlowNet.ONNX** с `edges: [holds, injects]`: объём фактов и
    `relations.json`, время; результат — в план, порог из `EXTRACTOR.md` §6.

## Не входит

- Свёртка поля и свойства; композиция из кода; петли `A → A`.
- Деление `relations.json` (отвергнуто в ADR).
