# PLAN_20260928-8_host — Опыт по профилям MCP: наборы инструментов и описания

Статус: **в работе** — не начат; отложен решением человека 2026-09-28 (на опыты ушло много
токенов). План написан так, чтобы его мог выполнить другой агент, не знающий разговора.
Зависит от [`PLAN_20260928-7`](PLAN_20260928-7_host_mcp-settings-and-profiles.md).

## Что проверяется

Две независимые настройки MCP: набор инструментов (`one` | `narrow`) и подробность описаний
(`brief` | `standard` | `full`). Шесть сочетаний. Вопросы: какое сочетание даёт верные ответы
дешевле всего; хватает ли агенту описания инструмента без всяких пояснений; что годится модели
уровня 27B.

## Главное отличие от прежних опытов

В опытах [`PLAN_20260928-5`](PLAN_20260928-5_host_graph-answers-for-agents.md) агент читал
описание графа из задания, написанное человеку-постановщику. Здесь агент получает **только то,
что отдаёт сам MCP**: список инструментов с описаниями. В задании — вопросы и запрет на поиск
по тексту, ни слова о параметрах.

## Условия

| № | Набор | Описание | Зачем |
|---|---|---|---|
| 1 | `one` | `brief` | крайнее: мало текста, общий инструмент |
| 2 | `one` | `standard` | среднее, умолчание |
| 3 | `one` | `full` | крайнее: весь текст |
| 4 | `narrow` | `brief` | для слабой модели |
| 5 | `narrow` | `standard` | то же, подробнее |
| 6 | без графа | — | поиск по тексту, для сравнения |

Сочетание `narrow` + `full` не проверяется: узким инструментам нечего описывать подробно.
По два прогона на условие. Модель — та же, что в прежних опытах (sonnet); условия 4 и 5 —
ещё и на самой слабой доступной модели. Модель уровня 27B агенту-исполнителю недоступна: её
проверяет человек на своей машине по этому же плану.

## Проект и вопросы

Проект `C:\GitKOE\NeuroModFlowNet.ONNX` — **только на чтение**: ничего не создавать и не
менять, git — только `git status --short` до и после, сверка реестра — никогда. Файл проекта
для замеров и собранные хост и извлекатель — как в PLAN_20260928-6, «Замер».

Извлечение — с `edges: [holds, injects, calls]`.

| № | Вопрос |
|---|---|
| 1 | Какие классы реализуют `ICoordinateBackTransform`? Файл и строки каждого. |
| 2 | Меняю конструктор `OpBase`. Кто наследует от него напрямую и кто — от них (второй уровень)? |
| 3 | Какие классы держат `OnnxExecutionContext` и через какой член (имя и строка объявления)? |
| 4 | В каком пространстве имён и какой сборке лежит `YoloObbResourceDefinition`, сколько типов в этом пространстве имён? |
| 5 | `PaddleOCRDetFP16_ExtractorBase`: от чего наследует и кто наследует от него? |
| 6 | Кто вызывает методы `YoloObbFactory`? Вызывающий метод, вызываемый, файл и строка каждого вызова. |
| 7 | Какие типы создаёт метод `YoloObbBatchedResource.Create` и какие методы других типов вызывает? |
| 8 | Где создаются объекты `ImageRunner`? Каждый метод, файл и строки. |

## Верные ответы

Посчитаны по полному графу 2026-09-28; перед опытом пересчитать — код проекта меняется.

1. Шесть: `Crop…`, `PadResize…`, `Perspective…`, `Resize…`, `Rotate90…`,
   `UndistortCoordinateBackTransform`.
2. Прямых 18: `BranchIfInstruction`, `Copy_OrtTensor_To_ModelDevice`,
   `Copy_OrtValue_To_MatImage`, `Model_Inference`, `Model_YoloCls`, `Model_YoloPose`,
   `Model_YoloSeg`, `Model_YoloSingleDetection`, `Op_DetectionsFromYoloBox`,
   `Op_DetectionsFromYoloObb`, `Op_Map_Coordinates`,
   `Op_Onnx_ExtractObbToLetterboxBatch_FP32_NCHW`, `Op_Onnx_ExtractObbToPaddleRec_FP32_NCHW`,
   `Op_Onnx_TensorTransformBase`, `Op_Track`, `Wrap_UnmanagedMem_To_OrtTensorBase`,
   `OpDelegate`, `OrderedSyncInstruction`. Второго уровня 17: от `Model_Inference` —
   `OnnxBatchedInference`, `OnnxRunner`, `ReloadableOnnxRunner`; от
   `Wrap_UnmanagedMem_To_OrtTensorBase` — `Wrap_MatImage_To_OrtTensor`; от
   `Op_Onnx_TensorTransformBase` — 13: `Op_Onnx_BgrU8Hwc_To_RgbNchw_Div255Base` и по два на
   `Crop`, `PadResize`, `Perspective`, `Resize`, `Rotate90`, `Undistort` (`…_NCHW_Base` и
   `…_U8_NHWC`).
3. Тринадцать классов: `ConverterBase` — `Context`:10; `OnnxExecutionContextDebugView` —
   `context`:11; `PaddleOCRRecListConverter` — `Context`:7; `PaddleOCRRecSingleConverter` —
   `Context`:7; `PaddleUVDocConverter` — `Context`:5; `Runner` — `Context`:7;
   `ConcatOrtValueBatchInputAssembler` — `concatContext`:12;
   `ConcatOrtValueRequestBatchInputAssembler` — `concatContext`:11;
   `Copy_OrtTensor_To_ModelDevice` — `onnxContext`:22;
   `Op_Onnx_ExtractObbToLetterboxBatch_FP32_NCHW` — `prepareContext`:55,
   `matrixUploadContext`:56; `Op_Onnx_ExtractObbToPaddleRec_FP32_NCHW` — `prepareContext`:44,
   `matrixUploadContext`:45; `Op_Onnx_TensorTransformBase` — `onnxContext`:26;
   `OrtValueBatchedInferenceEndpoint` — `modelContext`:16. `ImageRunner` и `StrategyRunner`
   получают через конструктор и наследуют `Context` от `Runner` — держателями не считаются.
4. Пространство имён `NeuroModFlowNet.Pipeline.ONNX`, сборка `NeuroModFlowNet.Pipeline.ONNX`,
   100 типов.
5. Наследует от `PaddleOCRDetExtractorBase`; наследников семь: `…FP16_16FC1_SafeExtractor`,
   `…16FC1_SafeListExtractor`, `…16FC1_UnsafeExtractor`, `…8UC1_Extractor`,
   `…8UC1_SafeListExtractor`, `…8UC3_RGBExtractor`, `…8UC3_RGB_SafeListExtractor`.
6. `YoloObbBatchedResource.Create` вызывает `List_PosCvdnn_FP32` (строка 31) и
   `List_SymCvdnn_FP32` (строка 32), файл `YoloObbBatchedResource.cs`. Других нет.
7. Создаёт `OnnxModel` и `OnnxExecutionContext` (28), `YoloObbBatchedResource` (36). Вызывает
   два метода фабрики (31, 32) и читает свойства `YoloObbResourceDefinition`: `ModelPath` (26),
   `RunnerKind` (29), `Batching` и `Name` (36).
8. 58 мест в семи фабриках: `PaddleOCRDetFactory` 14, `PaddleOCRRecFactory` 3,
   `YoloBoxFactory` 9, `YoloClsFactory` 10, `YoloObbFactory` 6, `YoloPoseFactory` 10,
   `YoloSegFactory` 6. Сверх того — создание через отражение (`Activator.CreateInstance`) в
   каждой фабрике; граф его целью не называет, ответ с ним и без него считается верным, а
   названное — отмечается отдельно.

## Что записывать

По каждому прогону: верных из восьми (с разбором каждой ошибки и её причины); токены и число
вызовов инструментов — из учёта запуска, а не со слов агента; объём ответов графа; сколько
файлов прочитано; размер описаний инструментов в байтах. Со слов агента: чего не хватило в
описании, что в нём лишнее, что он понял неверно.

Прежние результаты для сравнения — PLAN_20260928-5, шаг 4: `facts` 7 из 8 за 88–90 тыс.
токенов, `json` 7 из 8 за 133 тыс., без графа 6 из 8 за 126 тыс.

## Чего не делать

- Не давать агенту ничего, кроме вопросов: ни параметров, ни примеров, ни словаря.
- Не менять код по ходу опыта. Найденное — в список, правки после.
- Не делать выводов из разницы в токенах меньше 10 %.

## Результат

Сюда: таблица по условиям, разбор ошибок, предложение умолчаний для `mcp.tools` и
`mcp.description`.
