# PLAN_20260924_host — Экстракторы через `semaps.exe` и страница `/setup`

Статус: **в работе** — не начат.

Решение: [`ADR_20260924-3`](../adr/ADR_20260924-3_host_tools-beside-the-binary-and-setup.md).
Продолжает шаги 4 и 6 [`PLAN_20260923_core_sync-with-code`](PLAN_20260923_core_sync-with-code.md).

## Шаги

1. **`.semaps` — YAML.** `gopkg.in/yaml.v3`; ключ `extractors` (список: `id`, `language`,
   `project`, `root`, `include`, `exclude`, `command`). Старые файлы читаются как были.
   Запись — через `yaml.Node`, с комментариями. Тест: прочитать → поменять `include` →
   записать → комментарии и порядок на месте. Обновить `ADOPTING.md`, `README.md`.
2. **Поиск экстракторов и `semaps doctor`.** Порядок: `command` → `extractors/<lang>/`
   рядом с бинарником → PATH. Проверка runtime (`dotnet --list-sdks`, `node --version`).
   Вывод: что найдено, где, версия; чего нет и как поставить. Код выхода 1, если
   экстрактору из `.semaps` нечем запуститься.
3. **`semaps extract [id]` и `semaps sync [id] [--dry-run]` без `--facts`.** Прогон — папка
   во временном каталоге (факты, лог, статистика) с id; `sync` без `--facts` берёт
   последний прогон этого `id` или извлекает заново. Запуск — `exec` без оболочки, за
   интерфейсом `runner`. `--facts` остаётся.
4. **Поставка.** `install.cmd` и CI: `dotnet publish` экстрактора C# и бандл экстрактора TS
   в `extractors/<lang>/`; `semaps install` копирует папку целиком. Убрать `.nupkg` из
   README; `extractors/csharp/README.md` — «запуск отдельно» остаётся.
5. **HTTP API.** `GET /api/tools`, `GET`/`PUT /api/setup` (без `command`), `POST /api/runs`,
   лог прогона потоком (SSE), `POST /api/runs/{run}/sync` (`dryRun`). Все проверки
   (`sameOrigin`, `inside`) — в одном middleware. Правка `API.md` в том же коммите.
6. **`/setup` в редакторе.** Инструменты, экстракторы, прогон с логом и статистикой, отчёт
   сверки и «Записать»; после записи — перечитать проект. Кнопка «Сверить» в редакторе.
7. **Агенту.** `ADOPTING.md`: `semaps doctor` → `semaps extract` → `semaps sync --dry-run` →
   `semaps sync`.

## Не входит

- Серверный режим, аутентификация — только не замуровывать (ADR §6).
- Автономные экстракторы без runtime.
