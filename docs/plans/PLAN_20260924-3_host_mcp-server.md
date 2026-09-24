# PLAN_20260924-3_host — MCP-сервер `semaps mcp`

Статус: **в работе** — шаги 1–5 готовы; шаг 6 открыт (проверка на живом проекте); после
[`PLAN_20260924-2_contract_member-relations`](PLAN_20260924-2_contract_member-relations.md).

Решение: [`ADR_20260924-5`](../adr/ADR_20260924-5_host_mcp-server.md).

## Шаги

1. **Точка правил в `core/`.** Всё, что сервер обещает (выдача `id`, проверка записи, тексты с
   `origin`/`at`, запрет удаления, запись в `views/` только по флагу) — функции `core/`,
   которыми потом пользуется и HTTP API. Сначала выделить их из `sync.go`/`check.go`.
2. **Транспорт.** `semaps mcp` на stdio; SDK — официальный Go SDK MCP; корни — как у `semaps`
   без аргументов. `--project <id>` для workspace с несколькими проектами.
3. **Инструменты чтения:** `list_projects`, `list_views`, `get_entity`, `find_entities`
   (имя/символ/модуль/вид), `get_relations` (сущность, тип, видимость, направление),
   `get_text`, `sync_preview` (= `--dry-run`), `doctor`.
4. **Инструменты записи:** `set_text`, `add_relation` (авторская), `add_relation_type`,
   `set_relation_visible` (вид, `except`), `confirm_rename` (сущность → `symbol`, связь →
   `via.member`), `extract`, `sync`, `place_entities` (только с `requested_by_human: true`,
   ADR_20260924_contract).
5. **`API.md` §6** — состав инструментов и их обещания; `ADOPTING.md` — путь «через MCP» как
   основной, `.mcp.json` в проекте потребителя.
6. **Проверка на живом проекте**: агент делает переход §8 ADR-4 только инструментами.

## Не входит

- HTTP-транспорт MCP, аутентификация (путь отступления ADR-3 §6).
- Генерация раскладки видов сверх `place_entities`.
