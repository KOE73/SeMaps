# PLAN_20260928-3_extractors — Место в коде: диапазоны строк в фактах

Статус: **шаги 1–7 сделаны** (форма, схема+сверка, C#, TypeScript, граф, документы, проверка на
NeuroModFlowNet.ONNX). Шаг 5: узел графа несёт `endLine`, `spans`, `memberLines`, ребро — `line`
и `file`; всё это уходит без `fields=position`. Шаг 7 — см. «Результат проверки» ниже.
Решение: [`ADR_20260928`](../adr/ADR_20260928_host_live-code-graph.md) §5.

## Зачем

Агент, узнав из графа «X держит Y через поле `items`», должен прочитать ровно это место, а не
файл целиком. Для этого граф несёт файл и строки. Столбец не нужен: он нужен протоколу LSP, а
мы им не пользуемся.

## Что есть в коде (сверено 2026-09-28)

| Утверждение | Где |
|---|---|
| У символа есть `file` и необязательная `line` — строка объявления, с 1 | `docs/EXTRACTOR.md` §2.1, `schemas/extractor-facts.schema.json` |
| Строки конца нет; у членов и рёбер строк нет | там же: `member`, `via` |
| C#: строка — начало первого объявления; у `partial` остаётся первое по пути и строке | `extractors/csharp/.../FactsExtractor.cs:89`, `157-162` |
| TypeScript: строка — начало узла объявления | `extractors/typescript/src/collect.ts:32-33` |
| Схема закрыта: `additionalProperties: false` у символа, члена, ребра и `via` | `schemas/extractor-facts.schema.json` |
| Сверка незнакомые поля фактов пропускает | `docs/EXTRACTOR.md` §5, «Проверка фактов» |
| `members` сверка копирует в сущность **как есть**, сырым JSON | `core/facts.go:34-36`, `core/sync.go` |
| `line` символа и `visibility` в реестр не пишутся | `core/sync.go:276-291`, `816-862` |

Следствие из предпоследней строки: **строку нельзя просто добавить в запись члена** — сверка
унесёт её в `entities.json`, и реестр начнёт меняться от каждой правки кода выше по файлу.

## Шаги

### 1. Форма

Решить и записать в `docs/EXTRACTOR.md` §2 и в схему. Предложение:

- у символа — `endLine`, строка конца объявления;
- у символа из нескольких объявлений (`partial`) — `spans: [{file, line, endLine}]`, а `file` и
  `line` остаются первым объявлением, как сейчас;
- у ребра — `line`: строка члена или базового списка, откуда ребро; файл — файл символа `from`,
  либо `file`, если объявление в другом файле;
- строки членов — **вне** `members`: либо отдельным списком у символа
  (`memberLines: {<имя>: строка}`), либо сверка вырезает поле при копировании. Выбрать одно;
  первое не трогает сверку.

Все новые поля необязательны: извлекатель, который их не знает, остаётся верным.

### 2. Схема и сверка

- `schemas/extractor-facts.schema.json`, `core/facts.go`: новые поля.
- Тест сверки: факты с новыми полями дают **тот же** реестр, байт в байт, что и без них.

### 3. Извлекатель C#

`FactsExtractor.cs`: конец объявления из `GetLineSpan().EndLinePosition`; все объявления
`partial`; строки членов из `MembersBuilder.cs`; строки рёбер из `ReferenceEdgeCollector.cs`.
Детерминизм (`EXTRACTOR.md` §3) сохраняется: порядок `spans` — по пути и строке.

### 4. Извлекатель TypeScript

`collect.ts`: конец узла через `getEnd()`; строки членов и рёбер. Слияние объявлений
(интерфейсы, пространства имён) — как `partial`.

### 5. Граф

Поля узла и ребра в `core/graph.go`; `fields=position` в запросе
([`PLAN_20260928`](PLAN_20260928_host_graph-provider.md) шаг 4).

### 6. Документы

`docs/EXTRACTOR.md` §2.1, §2.2; `docs/extractors/csharp.md`, `docs/extractors/typescript.md`;
черновики остальных языков — строка о новых полях.

### 7. Проверка

На NeuroModFlowNet.ONNX: размер фактов до и после; реестр после сверки не изменился; выборочно
десять символов — диапазон совпадает с кодом.

## Не входит

- Столбцы.
- Строки в реестре и в `evidence` связей (контракт допускает `evidence.line`, сверка его не
  пишет — так и остаётся).

## Результат

Сделано (сессия 2026-09-28):

- **Форма** (шаг 1): у символа — `endLine`, `spans: [{file, line, endLine}]` (только при
  нескольких объявлениях), `memberLines: {<имя>: строка}` рядом с `members`; у ребра —
  `line`, `file` (только если отличается от файла символа `from`), не для `contains`/`depends`.
  Все поля необязательны.
- **Схема и сверка** (шаг 2): `schemas/extractor-facts.schema.json`, `core/facts.go` (структуры
  `Symbol.EndLine/Spans/MemberLines`, `Edge.Line/File`, `Span`) плюс проверки в `problems()`.
  `core/facts_positions_test.go`: факты с новыми полями и без них дают байт-в-байт одинаковый
  реестр после сверки.
- **C#** (шаг 3): `FactsExtractor.cs` собирает все объявления `partial`-типа (дедуп при линковке
  файла в две сборки), печатает `endLine`, `spans`; `MembersBuilder.cs` печатает `memberLines`;
  `ReferenceEdgeCollector.cs` и `AddStructuralEdges` печатают `line`/`file` рёбер. Золотые файлы
  `testdata/Sample/expected*.json` обновлены.
- **TypeScript** (шаг 4): `collect.ts` копит объявления символа (`spansById`) — слияние
  интерфейсов/`namespace` даёт `spans`; печатает `endLine`, `memberLines`, `line`/`file` рёбер
  (`file` для TS практически никогда не печатается: член всегда в файле своего символа).
  `types.ts` — новые поля. Золотые файлы `testdata/expected*.json` обновлены.
- **Документы** (шаг 6): `docs/EXTRACTOR.md` §2.1/§2.2/§3, `docs/extractors/csharp.md`,
  `docs/extractors/typescript.md` — разделы «Место в коде»; `docs/extractors/go.md`,
  `python.md`, `java-kotlin.md`, `rust.md` — по одной строке, что поля необязательны.

Не сделано в предыдущей сессии:

- **Шаг 5 (граф).** `core/graph.go`, `core/containers.go` и их тесты — не мои файлы в этой
  сессии (владеет другой агент, см. `PLAN_20260928_host_graph-provider.md` шаг 4).

### Результат проверки (шаг 7, 2026-09-28)

Экстрактор — свежий `dotnet publish` `SeMaps.Extract.CSharp` из этого коммита. Проект —
`C:\GitKOE\NeuroModFlowNet.ONNX` (рабочая копия с незакоммиченными правками, только на чтение).

**Размер фактов до/после.** Не «до/после сверки» в буквальном смысле (сверка — код другого
агента, `core/graph.go`), а до/после двух независимых прогонов извлечения: оба `facts.json` —
671 167 байт, побайтово идентичны (`md5 c9ab09b0…`), 405 символов, 1361 рёбер. Реестр модели
(`entities.json`/`relations.json` проекта) не трогался ни разу за всю проверку — файлы кода
только читались, `git status --short` до и после отличий не показал (см. общий отчёт по обоим
планам в `PLAN_20260928_host_graph-provider.md`).

**F. Выборка десяти символов.** Правило: символы с `file` и `endLine`, взятые с равным шагом
`N = floor(387/10) = 38` по списку из `facts.json` (387 символов из 405 несут `endLine`; 18 без
него — все `module`, см. «H» в `PLAN_20260928_host_graph-provider.md`). Для каждого — первая
строка (`line`) должна быть объявлением, последняя (`endLine`) — закрывающей скобкой или концом
однострочного объявления:

| Символ | Файл | line…endLine | Что в этих строках |
|---|---|---|---|
| `FrameContext` | `NeuroModFlowNet.CV/Tracking/FrameContext.cs` | 6…9 | `public readonly record struct FrameContext(` … `long? SourceFrameId);` — однострочный `record` без тела, конец на `);` |
| `ConverterMatSingleNchw<TBuf,TAlgo>` | `.../Inputs/Nchw/ConverterMatSingleNchw.cs` | 12…24 | `public class ConverterMatSingleNchw<…>` … `}` |
| `ITextRegionGammaCorrectionSettings` | `.../GammaCorrection/ITextRegionGammaCorrectionSettings.cs` | 11…14 | `public interface …` … `}` |
| `PaddleOCRDetExtractorBase<TOut>` | `.../PaddleOCR/Det/Extractors/PaddleOCRDetExtractorBase.cs` | 8…43 | `public abstract class …` … `}` |
| `TextRegionGaussianBlurOptions` | `.../GaussianBlur/TextRegionGaussianBlurOptions.cs` | 7…12 | `public sealed class …` … `}` |
| `YoloClsExtractorBase<TOut>` | `.../Yolo/Cls/Extractors/YoloClsExtractorBase.cs` | 8…22 | `public abstract class …` … `}` |
| `YoloSeg_FP32` | `.../Yolo/Seg/OutData/YoloSeg_FP32.cs` | 3…25 | `public readonly struct …` … `}` |
| `OnnxBatchedRequest<TInput,TOutput>` | `.../Resources/OnnxBatchedRequest.cs` | 5…19 | `internal sealed class …` … `}` |
| `Op_Onnx_Undistort_U8_NHWC` | `.../Operators/Op_Onnx_Undistort_U8_NHWC.cs` | 11…130 | `public sealed class … : Op_Onnx_TensorTransformBase` … `}` |
| `OpDebugPoint` | `NeuroModFlowNet.Pipeline/Diagnostics/OpDebugPoint.cs` | 10…15 | `public sealed record OpDebugPoint(` … `IReadOnlyList<VarDebugInfo> VariablesBefore);` — однострочный `record`, конец на `);` |

Все десять совпали без расхождений: диапазон строк точно накрывает объявление символа.

**Пять рёбер с `line`.** Правило: рёбра с непустым `line`, шаг `N = floor(585/5) = 117` по списку
рёбер, несущих `line` (585 из 1361):

| Ребро | line | Что в строке |
|---|---|---|
| `ITrackerDebugSnapshot` --holds(`ActiveTracks`)--> `TrackedObject` | 7 | `IReadOnlyList<TrackedObject> ActiveTracks { get; }` |
| `PaddleOCRRecSingleConverter` --holds(`Model`)--> `OnnxModel` | 8 | `OnnxModel Model => Context.Model;` |
| `ArrayMapCoordinatesExecutor<…>` --implements--> `IMapCoordinatesExecutor` | 5 | `internal sealed class ArrayMapCoordinatesExecutor<…> : IMapCoordinatesExecutor` — строка списка базовых типов |
| `Op_Onnx_PadResize_NCHW_Base` --uses(`executionBackend`)--> `InferenceBackend` | 21 | `protected Op_Onnx_PadResize_NCHW_Base(` — начало конструктора, где объявлен параметр |
| `YoloObbBatchedResource` --holds(`runner`)--> `IRunner<…>` | 12 | `readonly IRunner<List<Mat>, IDetectionResult<YoloObb>> runner;` |

Все пять совпали: строка либо содержит сам член (поле/свойство), либо (для `implements`) — список
базовых типов, либо (для `uses` через конструктор) — начало конструктора, где объявлен параметр.
Расхождений не найдено.

**Не проверялось:** столбцы (вне контракта — см. «Не входит»); `evidence.line` связей реестра (не
пишется намеренно); символы без `endLine` (все 18 — `module`, диапазон им не положен по контракту,
поэтому вне выборки).
