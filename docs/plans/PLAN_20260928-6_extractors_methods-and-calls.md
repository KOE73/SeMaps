# PLAN_20260928-6_extractors — методы и вызовы

Статус: **в работе** (ядро, сверка, хост и извлекатель C# сделаны; замер на реальном проекте
(`C:\GitKOE\NeuroModFlowNet.ONNX`) выполнен — см. раздел «Замер на NeuroModFlowNet.ONNX
(2026-09-28)»). Реализует
[`ADR_20260928-3`](../adr/ADR_20260928-3_host_calls-live-in-the-graph.md) и
[`ADR_20260928-4`](../adr/ADR_20260928-4_extractors_methods-and-calls.md).

## Зачем

Живой граф хоста (`ADR_20260928-3`) должен показывать методы и вызовы между ними, не трогая
реестр: они меняются на каждой правке кода и не годятся ни в `entities.json`, ни в
`relations.json`, ни в git. `ADR_20260928-4` определяет форму — вид символа `method`, рёбра
`calls`/`constructs`/`overrides`, поле `lines` — и главное правило: сверка не видит из этого
ничего, до всякой обработки, по виду.

## Что сделано в этом прогоне

### 1. Схема, `core/facts.go`, `docs/EXTRACTOR.md` — сделано

- `schemas/extractor-facts.schema.json`: `kind` получил `method`; `edges[].kind` — `calls`,
  `constructs`, `overrides`; новое поле ребра `lines`; `edgeKinds` принимает `calls`.
- `core/facts.go`: те же словари (`SymbolKinds`, `EdgeKinds`); `Edge.Lines []int`; проверки —
  `nativeKind` метода из закрытого списка (`method`, `constructor`, `property`, `indexer`,
  `operator`, `accessor`); `calls` — метод → метод; `constructs` — метод → тип; `overrides` —
  метод → метод; `implements` — либо два типа, либо два метода, никогда не вперемешку;
  `contains` на метод должен начинаться с типа; метод обязан быть `contains` ровно одним типом
  этого вывода; `lines` — только у `calls`, по возрастанию, без повторов; `via` по-прежнему не
  для органических рёбер, включая три новых.
- `docs/EXTRACTOR.md`: §1 (`--edges calls`), §2.1 (идентификатор метода), §2.2 (три новых вида
  ребра, `lines`), §2.2a (предложение «вызовы — не факты извлекателя» заменено ссылкой на оба
  ADR), §2.3 (строка `method` в словаре), §3 (обязанности извлекателя для методов: `partial`,
  перегрузки, явная реализация интерфейса, доступ свойства как один символ), §5 (что именно
  сверка отбрасывает и почему).
- Коммит `483ed10` — «Add methods and calls to the facts schema; sync drops them by kind».

### 2. Сверка пропускает всё — `core/sync.go` — сделано

`dropMethodsAndCalls` в `core/sync.go` вызывается первым делом в `Sync`, до блокировки модели и
до любой другой обработки: убирает символы `kind: method`; рёбра `calls`, `constructs`,
`overrides`; рёбра `contains`/`implements` с методом на любом конце; `calls` из `edgeKinds`.
Один по виду, а не по списку извлекателей — комментарий у функции указывает на ADR.

`core/sync_methods_test.go`:

- (a) факты с методами/вызовами и без — реестр байт в байт одинаков, на пустом реестре и на
  реестре, где уже есть типы;
- (b) отчёт сверки (`SyncReport.Print`) не упоминает ни `method`, ни имя метода, ни `calls`;
- (c) в `relation-types.json` нет типа `calls`/`constructs`/`overrides`;
- (d) второй прогон с убранным из фактов методом ничего не помечает пропавшим.

`go build ./core/...`, `go vet ./core/...`, `go test -count=1 ./core/...` — зелёные (см. ниже).
`gofmt -l core host` называет почти все файлы обеих папок: рабочая копия в CRLF (Windows
чекаут), это не связано с этой правкой — проверено `gofmt -d` на `core/facts.go` до и после:
разница только в переводах строк.

### 3. Хост: `edges` принимает `calls` — сделано частично

`host/projectfile.go`: `validateExtractors` разрешает `calls` в списке `edges` записи
`.semaps` (было — только `holds`, `uses`, `injects`); сообщение об ошибке и комментарий у
поля обновлены. `host/tools.go` (`extractorArgs`) уже был общим — просто склеивает список
через запятую — правки не потребовалось. `docs/API.md`: строка `extractors` в таблице ключей
`*.semaps` называет `calls` и что он означает.
Коммит `149088b` — «Accept calls in the .semaps extractor edges list».

`go vet ./host/` в этом ворктри не проходит по причине, названной в задании заранее:
`host/server.go:35: pattern all:app: no matching files found` — `host/app` (собранный
редактор) не в git, это не следствие правки.

### 4. Извлекатель C# — сделано (без замера)

Второй прогон реализовал пункт 3 целиком, тремя коммитами, каждый с зелёным `dotnet test`:

- **A** (`cad3a86`) — символы `method` (метод, конструктор — включая статический, свойство,
  индексатор, оператор), приватные тоже, с `visibility`/`file`/`line`/`endLine`; `contains`
  тип → метод; `id` по ADR §2 (`SymbolIds.MethodId`/`IndexerId`/`PropertyId`); `partial`-метод —
  один символ (часть-реализация), `spans` у обеих деклараций. Без `--edges calls` вывод не
  меняется (все 9 прежних тестов, включая два золотых файла, зелёные без изменений). Новый
  золотой файл `testdata/Sample/expected.calls.json`.
- **B** (`84b4a08`) — `overrides` (метод → непосредственный override, не корень цепочки) и
  `implements` для методов/свойств (`FindImplementationForInterfaceMember`, явная и неявная
  реализация), только когда реализация объявлена на самом посещаемом типе. По ходу: Roslyn не
  относит явную реализацию интерфейса к `MethodKind.Ordinary` — отдельный `MethodKind.ExplicitInterfaceImplementation`,
  добавлен в список допустимых; `core/facts.go`'s «contains на метод должен начинаться с типа»
  расширено до «с типа или интерфейса» (интерфейс содержит свои методы). Второй, отдельный
  тестовый проект `testdata/CallsSample` — специально, чтобы золотые файлы `testdata/Sample` не
  двигались ради сценариев calls/overrides/implements (per задание: «если фикстура становится
  неудобной — вторая маленькая фикстура»).
- **C** (`7edfa05`) — `calls`/`constructs` через `SemanticModel.GetSymbolInfo` (никогда по
  тексту): вызовы, чтение/запись свойства и индексатора, `new T(...)`/`new()`, инициализаторы
  конструктора (`base`/`this`), пользовательские операторы и преобразования, группа методов →
  делегат. Цель — исходное определение (`OriginalDefinition`, разворачивает обобщённые метод и
  тип разом); вызов через интерфейс/виртуальный метод резолвится в метод как написан на месте
  вызова — оба правила получаются от `GetSymbolInfo` бесплатно, без дополнительного кода. Вызов
  из лямбды/локальной функции достаётся объемлющему методу естественно (обход тела не
  останавливается на их границе). Одно ребро на `(from, to, kind)`, `lines` по возрастанию.
  `core/facts.go`: ребро на себя теперь разрешено для `calls` (рекурсия) — раньше запрет был
  общим для всех видов. `Program.cs` печатает в stderr счётчик мест вызова, чьи цели не попали
  в вывод. `testdata/CallsSample` расширен: перегрузки, обобщённый метод, рекурсия, взаимная
  рекурсия, лямбда, локальная функция, чтение/запись свойства, цепочка конструкторов. Оба
  `expected.calls.json` (Sample и CallsSample) перегенерированы под B и C.
- **D** (этот коммит) — `docs/extractors/csharp.md` получил раздел «Методы и вызовы»: что
  печатается, разрешение вызовов, самоссылка, и явный список того, что не сделано.

`dotnet test` в `extractors/csharp`: **15 / 15 зелёных** после каждого коммита A–D (включая
`Output_MatchesGoldenFile`/`Output_WithEdges_...`/`Output_WithCalls_...` для обеих фикстур,
проверку по схеме и детерминизм). `go test -count=1 ./core/...`: зелёный.

Счётчики на `testdata/CallsSample` (`--edges calls`): 56 символов, 97 рёбер, из них методов —
20, `overrides` — 2, `implements` — 4 (два неявных + одна явная реализация на методах, плюс
`Robot`→`IShape` на типах), `calls`/`constructs` — 16 (перечислены в отчёте предыдущего
сообщения этого прогона). `--edges calls` на `testdata/Sample` даёт 2 непойманных вызова
(`Task.FromResult`, `new Lazy<...>(...)` — оба в библиотечном коде вне вывода).

## Что не сделано в этом прогоне

- **Инициализаторы полей/свойств не приписаны конструктору** (ни явному, ни неявному) — известный,
  документированный пробел (`docs/extractors/csharp.md`), не нарушает THE ONE RULE (методы и
  вызовы всё равно не доходят до сверки), но снижает полноту извлечения.
- **Статический неявный конструктор (`.cctor`) не синтезируется** — статические инициализаторы
  не дают вызовов в этом прогоне.
- **Первичные конструкторы и позиционные члены `record`** — реализованы «самым буквальным
  прочтением» (как обычный конструктор/свойства), но не проверены отдельным тестом.
- **`nameof(Метод)`** может дать лишнее ребро `calls` — не отфильтровано.
- **Замер на `C:\GitKOE\NeuroModFlowNet.ONNX`** (пункт 5 задания): выполнен, см. раздел
  «Замер на NeuroModFlowNet.ONNX (2026-09-28)» ниже.

## Противоречия и неоднозначности ADR, замеченные по пути

- «Конструктор — символ вывода» (§3 таблица ADR) не говорит, что делать, когда конструктор
  объявлен, но не входит в `--include` (внешняя сборка). Принято читать как продолжение общего
  правила EXTRACTOR.md §2.2 — «ребро на символ вне вывода не печатается»: `calls` на такой
  конструктор просто не появляется, `constructs` на тип остаётся.
- Идентификатор свойства/индексатора (ADR §2: «свойство — без скобок») не говорит явно про
  индексатор; решено, что индексатор **со** скобками и списком типов параметров (индексаторы
  перегружаются, без скобок разные перегрузки дали бы один `id`) — исправлено в EXTRACTOR.md в
  коммите A.
- «`contains` на метод должен начинаться с типа» (первоначальная формулировка коммита A) не
  учитывала, что интерфейс тоже содержит свои методы; исправлено на «с типа или интерфейса» в
  коммите B, после того как первый прогон на `testdata/CallsSample` упал на этой проверке.
- «Ребро на себя запрещено» (существовавшее правило `core/facts.go`, не из этого ADR) вступало в
  прямое противоречие с прямо запрошенной в задании рекурсией. Решено точечно: самоссылка
  разрешена только для `calls`, остальные виды остаются под старым запретом — задокументировано
  в коммите C, ни один ADR не редактировался (правило было в коде, не в ADR).
- Остальных противоречий между двумя ADR или с существующей схемой не найдено.

## Замер на NeuroModFlowNet.ONNX (2026-09-28)

Статус: замер выполнен. Реестр (проверено на копии) не получает ничего из `calls`-части.

Перед этим прогоном на проекте были найдены и исправлены два дефекта: идентификаторы методов
содержали пробелы из-за `Dictionary<int, T[]>` (пробел после запятой в аргументах générика
просачивался в id); поле `lines` на ребре `constructs` отклонялось валидацией (validation
считала `lines` допустимым только для `calls`).

### A. Извлечение — время и размер

| Прогон | 1, с | 2, с | 3, с | мин | медиана | макс | байт | gzip байт | 3 прогона побайтово одинаковы |
|---|---|---|---|---|---|---|---|---|---|
| `holds,injects` (plain) | 6.27 | 6.34 | 6.37 | 6.27 | 6.34 | 6.37 | 671 167 | 40 744 | да |
| `holds,injects,calls` | 8.75 | 8.87 | 9.18 | 8.75 | 8.87 | 9.18 | 2 587 954 | 129 672 | да |

stderr прогона с `calls`: `calls: 7773 call/construct site(s) outside the output` (вызовы/конструирования
за пределами `--include src`, ожидаемо не печатаются как рёбра). stderr прогона `holds,injects` пуст.

### B. Разбор facts-calls.json

Символы (2344 всего): kind — `method` 1939, `type` 346, `interface` 40, `module` 18, `function` 1.
nativeKind: `method` 917, `property` 717, `constructor` 301, `class` 167, `static-class` 45,
`interface` 40, `struct` 36, `abstract-class` 34, `record-struct` 27, `record` 19, `enum` 18,
`namespace` 11, `assembly` 7, `operator` 3, `delegate` 1, `indexer` 1.

Рёбра (6584 всего) по kind: `calls` 2752, `contains` 2709, `implements` 252, `constructs` 225,
`holds` 183, `uses` 177, `overrides` 162, `extends` 118, `depends` 6.

| Методов на тип | мин | медиана | p90 | макс |
|---|---|---|---|---|
| | 1 | 3 | 11 | 31 |

Топ-5 типов по числу методов: `TrtConfig` (31), `OcrRegionPostprocessor` (23),
`OnnxExecutionContext` (23), `Op_Onnx_TensorTransformBase` (22), `OnnxModel` (21).

Рёбер `calls`/`constructs` с более чем одной строкой в `lines`: 470. Самый длинный список —
9 строк, ребро `Op_Onnx_ExtractObbToLetterboxBatch_FP32_NCHW.RunWithFreshBinding(...)` →
`OnnxExecutionContext.IoBinding`.

10 методов с наибольшим числом РАЗНЫХ вызывающих: `OnnxExecutionContext.Model` (62),
`ImageRunner\`4..ctor(OnnxExecutionContext)` (58), `OpDescriptor.Create(...)` (34),
`OnnxModel.Session` (33), `VarRequirement.Read\`1(string,bool)` (32),
`VarRequirement.Write\`1(string,bool)` (32), `VmRunContext.Set\`1(string,T,bool)` (28),
`ResultExtractorBase\`1.Model` (27), `IModelMetadataProvider.PrimaryOutputName` (23),
`OnnxGraphBuilderHelpers.CreateModel(string,GraphProto,long)` (20).

10 методов, вызывающих больше всего РАЗНЫХ целей: `OcrRegionPostprocessorJsonOptions.ToRuntimeOptions(float?)` (38),
`IouTracker.Process(...)` (34), `TrtConfig.FromDefaultOptions(string)` (27),
`YoloPoseFP32UniversalExtractor.GetOutput(IOnnxModelOutputs)` (27), `TrtConfig.ToDictionary()` (26),
`IouTracker+TrackState.Create(int,TrackDetection)` (21), `TextRegionProcessingStageFactory.CreateStage(...)` (20),
`VmController.WarmupAsync(...)` (20), `VmController.ExecuteRunAsync(...)` (19),
`VmProgram.ExecuteLoopInternalAsync(...)` (19).

Методов без единого входящего/исходящего `calls`-ребра: 424 (из 1939). Самоссылок (рекурсия) — 8.
`overrides` — 162. Методических (метод→метод) `implements` — 145 (из 252 `implements` всего).
Рёбер `calls`, нацеленных на метод интерфейса: 121 (интерфейсных методов всего 77).

### C. Неизменность «обычной» части

Из `facts-calls.json` удалены все символы `kind:method` и все рёбра `calls`, `constructs`,
`overrides`, а также `contains`/`implements` с методом на одном из концов, и `calls` вычеркнут
из `edgeKinds`. Результат посимвольно (как разобранный JSON, тот же порядок) совпал с
`facts-plain.json`: символов 405 = 405, рёбер 1361 = 1361, `edgeKinds` тем же набором. Различий нет.

### D. Точечная проверка 12 рёбер по коду

Проверены 8 `calls`, 2 `constructs`, 1 `overrides`, 1 методический `implements` (фиксированный шаг
по отсортированному списку). Во всех 12 случаях место в файле по заданной строке (или строкам)
лежит внутри диапазона `span..endLine` метода `from`, и в исходнике на этой строке действительно
стоит указанный вызов/конструирование/переопределение/реализация — включая однострочные
методы-выражения (`=>`), например `YoloObbFactory.List_SymCvdnn_FP32` (строка 28, тело в одну
строку). Несовпадений не найдено.

### E. YoloObbFactory — мотивирующий случай

У 8 методов `YoloObbFactory` — 17 исходящих рёбер `calls`/`constructs` (в основном `new(context)`
через `ImageRunner\`4..ctor`, плюс обращения к `context.Model.Session`/`PrimaryInputName`/
`PrimaryOutputName` и внутренний вызов `GetExtractorType`) и 2 входящих `calls` — оба из
`YoloObbBatchedResource.Create(YoloObbResourceDefinition)`, вызывающего `List_PosCvdnn_FP32` и
`List_SymCvdnn_FP32`.

При чтении `YoloObbFactory.cs` в глаза бросается `CreateRunner<TOut>` (строки 36-59): там есть
`typeof(ImageRunner<,,,>).MakeGenericType(...)` и `Activator.CreateInstance(closedType, context)`
(строка 57) — динамическое конструирование через reflection. Этого конструирования в рёбрах нет,
и это ожидаемо: тип и конструктор разрешаются только в рантайме (`MakeGenericType`/
`Activator.CreateInstance` не являются статическим `new`-выражением), извлекатель работает по
семантической модели компиляции и не может знать заранее, какой закрытый generic-тип будет
создан. Других пропущенных механизмов (делегаты, `new T()` по generic-параметру) в этом файле нет.

### F. Идентификаторы

Все 2344 id символов уникальны. Пробелов в id нет (0 штук) — оба ранее найденных дефекта
(в т.ч. пробелы от `Dictionary<int, T[]>`) уже исправлены. Самый длинный id — 257 символов:
`NeuroModFlowNet.Pipeline.ONNX.OrtValueBatchedInferenceEndpoint\`1..ctor(string,string,InferenceBackend,OnnxBatchedResourceOptions,IOrtValueBatchInputAssembler,IOrtValueOutputShapeResolver,IOrtValueBatchOutputDecoder<TOutput>,Action<ExecutionProviderConfig>?)`.
Id методов длиннее 200 символов — 3. Перегрузки: тип `PosCvdnnFP16`, метод `Fill` — id различаются
по списку параметров (`Fill(List<Mat>,Span<Float16>,int,int,int)` vs `Fill(Mat,Span<Float16>,int)`).

### G. Реестр не должен измениться — проверено на копии

**Главный результат: реестр не меняется.** Копии `docs/diagrams` в
`C:\GitKOE\SeMaps\.claude\nmfn-calls-check\copy-a` (синхронизирована из `facts-plain.json`,
проект `a.semaps`) и `copy-b` (синхронизирована из `facts-calls.json`, проект `b.semaps`)
побайтово идентичны (`diff -rq` — без различий). Оба отчёта `semaps sync` показывают одно и то же:
«Сверка проекта nmfn с фактами csharp: 405 символов, 1361 рёбер» — хотя `facts-calls.json` содержит
2344 символа и 6584 ребра, сверка сама отбрасывает всё, что относится к методам и вызовам, ещё до
записи в реестр. Список «не хватает (80)» в обоих отчётах идентичен. Поиск по `copy-b` не нашёл
ни `"calls"`, ни `"constructs"`, ни `"overrides"`, ни `..ctor(` ни в одном файле; `relation-types.json`
в `copy-b` содержит только `references`, `implements`, `extends`, `contains`, `depends`, `holds.*`,
`injects` — без единого типа отношения, связанного с методами или вызовами.

### Проект NeuroModFlowNet.ONNX

`git status --short` до и после прогона — оба раза пустой вывод, дерево не тронуто.

## Следующий шаг

Замер выполнен; извлекатель методов и вызовов на реальном проекте подтверждён рабочим и
безопасным для реестра. Следующий шаг — вне рамок этого плана (интеграция вызовов в UI/граф хоста).
