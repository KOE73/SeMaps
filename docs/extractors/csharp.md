# Членские связи C#: соответствие типов признакам

Нормативный документ о том, как типы C# отображаются на признаки членских связей из кода
(§2 [`ADR_20260924-4`](../adr/ADR_20260924-4_contract_member-relations-from-code.md)),
и какие языковые возможности не покрыты.

## Кратность и изменяемость коллекций

| Тип | Кратность | Изменяемость | Примечание |
|---|---|---|---|
| `T[]` | `many` | `mutable` | массив |
| `List<T>` | `many` | `mutable` | список |
| `HashSet<T>`, `SortedSet<T>` | `many` | `mutable` | набор без гарантий порядка |
| `ICollection<T>`, `IList<T>` | `many` | `mutable` | интерфейсы коллекции (без гарантии: интерфейс может быть readonly) |
| `IReadOnlyList<T>`, `IReadOnlyCollection<T>` | `many` | `readonly` | read-only интерфейс коллекции |
| `IEnumerable<T>` | `many` | `readonly` | перечисляемое, не добавляется ни удаляется через интерфейс |
| `ImmutableArray<T>`, `ImmutableList<T>` | `many` | `readonly` | неизменяемые структуры |
| `Span<T>`, `ReadOnlySpan<T>` | `many` | `readonly` | диапазон в памяти |
| `Dictionary<K, V>` | `keyed` | `mutable` | словарь, держит **ключ и значение** (два ребра) |
| `SortedDictionary<K, V>`, `SortedList<K, V>` | `keyed` | `mutable` | упорядоченный словарь |
| `IDictionary<K, V>` | `keyed` | `mutable` | интерфейс словаря |
| `ConcurrentDictionary<K, V>` | `keyed` | `mutable` | потокобезопасный словарь |
| `IReadOnlyDictionary<K, V>` | `keyed` | `readonly` | read-only словарь |
| `ImmutableDictionary<K, V>` | `keyed` | `readonly` | неизменяемый словарь |

## Опциональность

| Тип | Кратность | Примечание |
|---|---|---|
| `T?` (где `T` — struct) | `optional` | Nullable<T>, обёртка для null-safety |
| `Nullable<T>` | `optional` | явно написанный Nullable |
| `T` (где `T` — reference type) | `one` | ссылочный тип — в C# может быть null, но язык не отличает статически |

**Знание:** Если поле ссылочного типа помечено как `nullable` в коде (атрибут `#nullable enable` и `T?`), это не
извлекается как признак, остаётся только в подписи `via.text`. Членская связь держит ссылку на тип, опциональность не влияет на кратность связи.

## Отложенные типы

| Тип | Сигнатура пути | Примечание |
|---|---|---|
| `Task<T>` | `result` | асинхронный результат |
| `ValueTask<T>` | `result` | асинхронный результат, структура |
| `Task` | `result` | задача без результата |
| `Lazy<T>` | `result` | ленивое вычисление |
| `Func<T>` | `result` | делегат без параметров, возвращающий T |
| `Func<…, TResult>` | `result` | делегат, возвращающий TResult; параметры — `arg:n` |
| `IObservable<T>` | `result` | поток событий |

**Форма:** Признак `deferred: true` + слот в пути, чтобы обозначить, где держится целевой тип.

## Слоты пути для словарей и коллекций

| Слот | Коллекция | Примечание |
|---|---|---|
| `item` | `T[]`, `List<T>`, `IEnumerable<T>` | элемент коллекции |
| `key` | `Dictionary<K, V>` | ключ (извлекается отдельно, если K — символ из вывода) |
| `value` | `Dictionary<K, V>` | значение (извлекается отдельно, если V — символ из вывода) |
| `result` | `Task<T>`, `Lazy<T>`, `Func<T>`, `IObservable<T>` | результат отложенного вычисления |
| `element:<n>` | кортежи (в будущем) | элемент кортежа по индексу (не реализовано для C#) |

**Множество путей:** `ConcurrentDictionary<long, Task<T>>` даёт:
- `path: ["value", "result"]` для связи на T (через значение, которое отложено)

## Переименование членов в параметрах конструктора

| Контекст | memberKind | Примечание |
|---|---|---|
| Поле класса | `field` | обычное поле |
| Свойство (property) | `property` | get/set, auto property, init-only |
| Событие | `event` | обработчик событий |
| Индексатор | `indexer` | `[]` синтаксис |
| Параметр конструктора | `parameter` | параметр, который может быть назначен полю через `[FromQuery]` или связь в контексте DI |
| Параметр метода | `parameter` | (не печатается для членских связей, только для сигнатур) |
| Возвращаемое значение | `return` | (не печатается для членских связей, только для сигнатур) |
| Псевдоним типа | `self` | (в C# не встречается; для TS type alias) |

## Из кода не выводится (извлекатель молчит)

| Что | Почему | Решение |
|---|---|---|
| Композиция (владение) | на уровне типа неразличима | авторская связь с `holds`, `kind: composition` |
| Агрегация (ссылка на общий объект) | на уровне типа неразличима | авторская связь с `holds`, `kind: aggregation` |
| Видимость интерфейсных параметров | интерфейсы ограничены языком, не явлены | `via` содержит фактический тип, подпись на вид |
| Обобщения с ограничениями `where T : IFoo` | не печатаются как связи | авторская связь, если нужна явность |

## Маппинг kind и nativeKind

Символ вида `type` с соответствующим `nativeKind`:

| kind | nativeKind | Примеры |
|---|---|---|
| `type` | `class` | класс |
| `type` | `struct` | структура |
| `type` | `record` | record |
| `type` | `enum` | перечисление |
| `interface` | `interface` | интерфейс |
| `function` | `delegate` | делегат (как функциональный тип) |
| `module` | `namespace` | пространство имён |
| `module` | `assembly` | проект (сборка) |

**Замечание:** Вложенный тип — это `type` с собственными членами, `namespace` пусто, файл — файл первого объявления.

## Модификаторы в nativeKind

Когда тип имеет специальный модификатор, он становится частью `nativeKind`:

| Модификатор | nativeKind | Пример |
|---|---|---|
| `abstract` | `abstract-class`, `abstract-record` | абстрактный класс |
| `static` | `static-class` | статический класс (для функций) |
| `sealed` | `sealed-class`, `sealed-record` | запечатанный класс |
| `readonly` | `readonly-struct` | структура, поля которой readonly |
| `ref` (struct) | `ref-struct` | структура на стеке (C# 7.2+) |

## Видимость (visibility)

Извлекатель печатает видимость для каждого символа:

| Видимость | Примеры | Примечание |
|---|---|---|
| `public` | открытый класс, интерфейс, поле | видна извне сборки |
| `internal` | внутренняя по умолчанию | видна только внутри сборки |
| `private` | приватное поле, методы | видно только внутри типа |
| `protected` | защищённое поле, метод | видно в наследниках |
| `protected internal` | защищённое или внутреннее | видно в наследниках и сборке |
| `private protected` | приватное или защищённое (C# 7.2) | видно в наследниках этого типа в этой сборке |
| (отсутствует) | вложенный тип без модификатора | считать публичным в контексте содержащего типа |

## Примеры рёбер и путей

### Простая коллекция

```csharp
public List<Message> Messages { get; set; }
```

Извлекатель печатает ребро:
```json
{
  "from": "App.Chat",
  "to": "App.Message",
  "kind": "holds",
  "via": {
    "member": "Messages",
    "memberKind": "property",
    "modifiers": ["public"],
    "text": "List<Message>",
    "path": ["item"],
    "cardinality": "many",
    "mutability": "mutable"
  }
}
```

### Словарь с отложенным значением

```csharp
private ConcurrentDictionary<long, Task<VmRunOutcome>> activeRuns;
```

Извлекатель печатает два ребра: одно на ключ (если `long` — символ), одно на значение:
```json
{
  "from": "VM.VmController",
  "to": "VM.VmRunOutcome",
  "kind": "holds",
  "via": {
    "member": "activeRuns",
    "memberKind": "field",
    "modifiers": ["private", "readonly"],
    "text": "ConcurrentDictionary<long, Task<VmRunOutcome>>",
    "path": ["value", "result"],
    "cardinality": "keyed",
    "mutability": "mutable",
    "deferred": true
  }
}
```

### Опциональное значение

```csharp
public ILogger? Logger { get; set; }
```

Ребро (опциональность остаётся в подписи):
```json
{
  "from": "App.Service",
  "to": "App.ILogger",
  "kind": "holds",
  "via": {
    "member": "Logger",
    "memberKind": "property",
    "modifiers": ["public"],
    "text": "ILogger?",
    "cardinality": "one"
  }
}
```

## История изменений

Документ входит в контракт языка C# по членским связям; изменяется вместе с извлекателем.
