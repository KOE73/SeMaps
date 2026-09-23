# PLAN_20260923_extractors_csharp — Первый извлекатель: C#

Статус: **не начат**.

Цель: `semaps-extract-csharp`, печатающий факты по [`EXTRACTOR.md`](../EXTRACTOR.md).
Первый язык — C#, потому что первые проекты-потребители на нём. Без извлекателя
сверка ([`PLAN_20260923_core_sync-with-code`](PLAN_20260923_core_sync-with-code.md))
проверяется только на выдуманных фактах.

## Шаги

1. Проект `extractors/csharp/` — .NET tool, Roslyn через `MSBuildWorkspace`
   (§3: разрешённые символы, не текст). Аргументы §1, коды выхода §1.
2. Символы: типы, интерфейсы, делегаты, пространства имён как `module`;
   `partial` в одну запись; `enum` со значениями в `members`; публичные члены
   интерфейсов; свойства и поля с типом.
3. Рёбра: `extends`, `implements`, `references` (схлопнуты в одно на пару),
   `contains`. Всё, что указывает за пределы вывода, не печатается.
4. Исключения по умолчанию: `bin/`, `obj/`, `*.g.cs`, `*.Designer.cs`,
   `AssemblyInfo`, `GlobalUsings`.
5. Детерминизм: сортировка `symbols` по `id`, `edges` по `(from, to, kind)`;
   тест «два запуска — один байт в байт вывод».
6. JSON-схема вывода в `schemas/extractor-facts.schema.json`; извлекатель
   валидирует свой вывод по ней в тестах.
7. Прогон на самом SeMaps не получится (тут нет C#) — тестовый набор в
   `extractors/csharp/testdata/` из небольшого решения с `partial`, `enum`,
   вложенными типами и generic-ссылками.

## Открытые вопросы

- Как ставить: `dotnet tool install -g` или exe в том же Release, что `semaps`?
  Второе означает self-contained сборку в 60+ МБ на платформу.
- Generic-типы: `id` с арностью (`List\`1`) или с параметрами? Нужен ADR в зоне
  `extractors_csharp` перед шагом 2.
