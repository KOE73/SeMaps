# Членские связи TypeScript: соответствие типов признакам

Нормативный документ о том, как типы TypeScript отображаются на признаки членских связей из кода
(§2 [`ADR_20260924-4`](../adr/ADR_20260924-4_contract_member-relations-from-code.md)),
и какие языковые возможности не покрыты.

## Кратность и изменяемость коллекций

| Тип | Кратность | Изменяемость | Примечание |
|---|---|---|---|
| `T[]` | `many` | `mutable` | массив (встроенный тип) |
| `Array<T>` | `many` | `mutable` | явный тип массива |
| `Set<T>` | `many` | `mutable` | множество |
| `readonly T[]` | `many` | `readonly` | массив с readonly модификатором |
| `ReadonlyArray<T>` | `many` | `readonly` | тип read-only массива |
| `ReadonlySet<T>` | `many` | `readonly` | множество с readonly модификатором |
| `Map<K, V>` | `keyed` | `mutable` | словарь, держит **ключ и значение** (два ребра) |
| `Record<K, V>` | `keyed` | — | объект как словарь; мутабельность определяется в зависимости от контекста (см. ниже) |
| `{ [key: string]: V }` | `keyed` | — | сигнатура индекса (мутабельность неизвестна) |
| `ReadonlyMap<K, V>` | `keyed` | `readonly` | словарь с readonly модификатором |
| `Readonly<Record<K, V>>` | `keyed` | `readonly` | неизменяемый объект-словарь |
| `{ readonly [key: string]: V }` | `keyed` | `readonly` | сигнатура индекса с readonly |

**Замечание:** `Record<K, V>` и индексные сигнатуры могут быть мутабельны или readonly в зависимости от контекста;
извлекатель не выводит мутабельность из типа и не печатает признак.

## Опциональность

| Тип | Кратность | Примечание |
|---|---|---|
| `T \| undefined` | `optional` | может быть undefined |
| `T \| null` | `optional` | может быть null (не типично в TypeScript) |
| `T?` (свойство) | `optional` | опциональное свойство класса |
| `T` | `one` | обязательное значение |

**Форма:** Опциональность не печатается как признак `cardinality: optional` в членской связи на само значение.
Вместо этого извлекатель рассматривает `T | undefined` как `one`, т.к. целевой тип — это T. Информация об опциональности остаётся в подписи `via.text`.

## Отложенные типы

| Тип | Сигнатура пути | Примечание |
|---|---|---|
| `Promise<T>` | `result` | асинхронный результат |
| `Awaited<T>` | `result` | распаковка Promise (TS 4.5+) |

**Форма:** Признак `deferred: true` + слот в пути.

## Объединения (union types)

| Тип | Рёбра | Примечание |
|---|---|---|
| `T1 \| T2 \| T3` | несколько рёбер | каждый тип в объединении — отдельное ребро с `cardinality: one` |

**Замечание:** Если в объединении несколько символов из вывода, извлекатель печатает ребро на каждый.
Пример: `IUser | IAdmin` даёт два ребра на одного члена, оба с одинаковыми `(from, to, member)`, но с разными `to`.

## Кортежи

| Тип | Слоты пути | Примечание |
|---|---|---|
| `[T1, T2, T3]` | `element:0`, `element:1`, `element:2` | элементы по индексу |
| `[x: T1, y: T2]` | `element:x`, `element:y` | элементы по имени (TS 4.4+) |

**Форма:** На каждый символ в кортеже — отдельное ребро с соответствующим слотом в пути.

## Псевдонимы типов

| Тип | memberKind | Примечание |
|---|---|---|
| `type Alias = T` | `self` | псевдоним типа (правая часть = целевой тип) |

**Замечание:** `type Alias<T> = T[]` печатается как связь на T через псевдоним, с `memberKind: self`.

## Членские виды (memberKind)

| memberKind | Контекст | Примечание |
|---|---|---|
| `field` | свойство класса | обычное свойство |
| `property` | свойство с getter/setter | (может быть не отличимо от field в TS) |
| `parameter` | параметр конструктора | параметр для инициализации |
| `constructor` | конструктор | блок объявления членов через конструктор |
| `return` | тип возврата функции | (не печатается для членских связей класса, только для function/value) |
| `self` | псевдоним типа | указывает, что это псевдоним (`type Alias = T`) |

**Замечание:** TypeScript рано вычисляет типы; `getter`/`setter` могут быть не отличимы от простого свойства без явного анализа синтаксиса.

## Модули и файлы

| Контекст | kind | nativeKind | Примечание |
|---|---|---|
| Файл `.ts` или `.tsx` | `module` | `file` | файл — единица группировки в TypeScript |
| Пакет (package.json) | `module` | `package` | пакет — единица распределения |

## Видимость (visibility)

В TypeScript видимость контролируется с помощью модификаторов и экспорта:

| Видимость | Примеры | Примечание |
|---|---|---|
| `public` | явно написано `public` или открыто в интерфейсе | видна извне модуля |
| `protected` | `protected` в классе | видна наследникам |
| `private` | `private` в классе | видна только внутри класса |
| `exported` | `export` на уровне модуля | видна импортерам |
| (отсутствует/internal) | не экспортировано | видна только в модуле |

**Замечание:** Извлекатель печатает видимость, основываясь на наличии `export` и модификаторах доступа. Внутренняя видимость — это не-экспортированная переменная или функция.

## Из кода не выводится (извлекатель молчит)

| Что | Почему | Решение |
|---|---|---|
| Композиция (владение) | на уровне типа неразличима | авторская связь с `holds`, `kind: composition` |
| Агрегация (ссылка на общий объект) | на уровне типа неразличима | авторская связь с `holds`, `kind: aggregation` |
| Отличие getter/setter от field | разные синтаксисы при анализе | оба печатаются как property, если не разобран синтаксис |
| Дополнительные параметры generics | ограничения `<T extends Foo>` | авторская связь, если нужна явность |
| Декораторы и метаданные | TypeScript не хранит их в типах | авторская связь, если нужна явность |

## Сопоставление рёбер

Нормативная таблица по [`ADR_20260927`](../adr/ADR_20260927_contract_closed-relation-vocabulary.md) §4:
как конструкция TypeScript сводится к закрытому словарю `kind` ([`EXTRACTOR.md`](../EXTRACTOR.md) §2.2) и
что пишется в `native`. Каждая строка покрыта примером в `extractors/typescript/testdata/project` и
проверяется тестом `src/test/mapping.test.ts` (`kind` и `native` по строке); расхождение таблицы и кода
роняет сборку. `holds`/`uses` печатаются только с `--edges holds,uses`.
`native` сверка не сравнивает: тип связи в реестре он не меняет
([`ADR_20260930-3`](../adr/ADR_20260930-3_contract_fact-kinds-and-derived-relation-types.md)).

| конструкция | kind | native | почему | что отвергнуто |
|---|---|---|---|---|
| `class A extends Base` | `extends` | `class` | подтип класса | без `native`: `extends` выражается и классом, и интерфейсом |
| `interface A extends B` | `extends` | `interface` | расширение контракта | `implements`: интерфейс ничего не реализует |
| `class A implements I` | `implements` | `implements` | явное объявление соответствия | без `native`: структурное соответствие без `implements` тоже соответствие; извлекатель его не выводит, но `native` отмечает, что связь объявлена явно |
| файл → объявление | `contains` | `file` | модуль-файл содержит объявление | без `native`: объявление может лежать и в `namespace` |
| `namespace N { … }` → объявление | `contains` | `namespace` | вложенность в пространство имён | `file`: пространство имён — отдельный модуль |
| поле класса (`x: T`) | `holds` | `field` | держит ссылку на тип | `property`: у класса поле и аксессор различаются |
| свойство интерфейса; `get`/`set` класса | `holds` | `property` | держит ссылку через свойство | `field`: в интерфейсе нет полей |
| параметр-свойство конструктора (`constructor(private x: T)`) | `holds` | `parameterProperty` | параметр с модификатором становится полем | `field`: теряется, что поле объявлено в конструкторе. Этот же параметр даёт и `uses`/`constructor` |
| параметр функции или метода | `uses` | `parameter` | пользуется, не держа | `holds`: параметр не хранится |
| возвращаемый тип | `uses` | `return` | пользуется, не держа | — |
| параметр конструктора | `uses` | `constructor` | зависимость при создании (`injects` в сверке) | `holds`: хранит ли класс параметр, видно только у параметра-свойства (строка выше) |
| `type A = …B…` | `uses` | `alias` | правая часть псевдонима упоминает тип | `extends`: псевдоним не подтип; `holds`: состояния нет |
| `const v: T = …` | `uses` | `annotation` | объявленный тип значения | `holds`: значение модуля — не член типа |

## Маппинг kind и nativeKind

| kind | nativeKind | Примеры |
|---|---|---|
| `type` | `class` | класс |
| `type` | `interface` | интерфейс (обрабатывается как тип) |
| `type` | `type-alias` | `type Foo = …` |
| `type` | `enum` | перечисление |
| `function` | `function` | функция на уровне модуля |
| `function` | `method` | метод класса/интерфейса |
| `function` | `constructor` | конструктор |
| `value` | `variable` | переменная на уровне модуля |
| `value` | `const` | константа |
| `module` | `file` | файл `.ts` или `.tsx` |
| `module` | `namespace` | объявление namespace (редко) |
| `module` | `package` | пакет (если анализируется) |

## Примеры рёбер и путей

### Простой массив

```typescript
interface ChatRoom {
  messages: Message[];
}
```

Ребро:
```json
{
  "from": "ChatRoom",
  "to": "Message",
  "kind": "holds",
  "via": {
    "member": "messages",
    "memberKind": "field",
    "text": "Message[]",
    "path": ["item"],
    "cardinality": "many",
    "mutability": "mutable"
  }
}
```

### Словарь (Map) с отложенным значением

```typescript
class Executor {
  private running: Map<string, Promise<Result>>;
}
```

Два ребра: на ключ (если `string` — символ, то ребро не печатается, т.к. это примитив), на значение:
```json
{
  "from": "Executor",
  "to": "Result",
  "kind": "holds",
  "via": {
    "member": "running",
    "memberKind": "field",
    "modifiers": ["private"],
    "text": "Map<string, Promise<Result>>",
    "path": ["value", "result"],
    "cardinality": "keyed",
    "mutability": "mutable",
    "deferred": true
  }
}
```

### Объединение типов

```typescript
class Form {
  state: ValidState | InvalidState;
}
```

Два ребра:
```json
[
  {
    "from": "Form",
    "to": "ValidState",
    "kind": "holds",
    "via": {
      "member": "state",
      "memberKind": "field",
      "text": "ValidState | InvalidState",
      "cardinality": "one"
    }
  },
  {
    "from": "Form",
    "to": "InvalidState",
    "kind": "holds",
    "via": {
      "member": "state",
      "memberKind": "field",
      "text": "ValidState | InvalidState",
      "cardinality": "one"
    }
  }
]
```

### Кортеж

```typescript
interface Pair {
  data: [User, Config];
}
```

Два ребра с разными путями:
```json
[
  {
    "from": "Pair",
    "to": "User",
    "kind": "holds",
    "via": {
      "member": "data",
      "memberKind": "field",
      "text": "[User, Config]",
      "path": ["element:0"],
      "cardinality": "one"
    }
  },
  {
    "from": "Pair",
    "to": "Config",
    "kind": "holds",
    "via": {
      "member": "data",
      "memberKind": "field",
      "text": "[User, Config]",
      "path": ["element:1"],
      "cardinality": "one"
    }
  }
]
```

### Псевдоним типа

```typescript
type UserList = User[];
```

Ребро (тип псевдонима):
```json
{
  "from": "UserList",
  "to": "User",
  "kind": "holds",
  "via": {
    "member": "UserList",
    "memberKind": "self",
    "text": "User[]",
    "path": ["item"],
    "cardinality": "many",
    "mutability": "mutable"
  }
}
```

## Место в коде (`endLine`, `spans`, `memberLines`, `line`/`file` ребра)

Извлекатель печатает, где в коде живёт каждый факт (`docs/EXTRACTOR.md` §2.1–2.2):

- `endLine` — конец узла объявления (`getEnd()`).
- `spans` — слияние объявлений (интерфейсы, `namespace`) трактуется как `partial` в C#: если
  один и тот же символ TypeScript встречает несколько раз, каждое вхождение — отдельный элемент
  `spans`; первое встреченное объявление остаётся `file`/`line` символа.
- `memberLines` — строка первого не-неявного объявления члена; перегрузки методов интерфейса с
  одинаковым именем берут меньшую строку.
- `line`/`file` ребра `extends`/`implements` — строка списка наследования (`heritageClauses`);
  `holds`/`uses` — строка самого члена (поля, свойства, метода) или конструктора, чей параметр
  или возврат дал ребро. Поскольку член всегда лежит в том же файле, что и его символ, `file`
  ребра для TypeScript не печатается.

## История изменений

Документ входит в контракт языка TypeScript по членским связям; изменяется вместе с извлекателем.
