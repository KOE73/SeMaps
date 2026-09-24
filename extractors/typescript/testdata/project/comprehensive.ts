// Comprehensive test cases for member relations

export class Item {}
export class Container {}
export class Widget {}
export class Box<T> {}

// 1. readonly T[] and ReadonlyArray<T>
export class Collections {
  mutableArray: Item[] = [];
  readonlyArrayType: readonly Item[] = [];
  readonlyArrayBuiltin: ReadonlyArray<Item> = [];
}

// 2. Set<T> and ReadonlySet<T>
export class Sets {
  items: Set<Item> = new Set();
  readonlyItems: ReadonlySet<Item> = new Set();
}

// 3. Map<K,V> and ReadonlyMap<K,V>
export class Maps {
  byId: Map<string, Item> = new Map();
  byWidget: Map<Widget, Container> = new Map();
  readonlyMap: ReadonlyMap<string, Item> = new Map();
}

// 4. Record
export class Records {
  itemsByKey: Record<string, Item> = {};
  readonlyRecord: Readonly<Record<string, Container>> = {};
}

// 5. Index signature
export class Indexed {
  [key: string]: Item | undefined;
  readonly [key: symbol]: Container | undefined;
}

// 6. Optional (x?: T) and union with undefined
export class Optional {
  optionalField?: Item;
  nullableField: Container | undefined;
  nullableField2: Widget | null;
}

// 7. Promise and Awaited
export class Async {
  pending: Promise<Item>;
  awaited: Awaited<Promise<Container>>;
}

// 8. Union types
export class Union {
  either: Item | Container;
  multiple: Item | Container | Widget;
}

// 9. Tuple types
export class Tuples {
  pair: [Item, Container];
  triple: [Item, Container, Widget];
  namedTuple: [first: Item, second: Container];
}

// 10. Type aliases
export type ItemAlias = Item;
export type ItemArray = Item[];
export type ContainerBox = Box<Container>;

// 11. Constructor parameters
export class Constructor {
  constructor(item: Item) {}
  constructor(item: Item, container: Container) {}
}

// 12. Parameter property (constructor private readonly)
export class ParameterProperty {
  constructor(
    private readonly item: Item,
    public container: Container,
    protected widget: Widget,
  ) {}
}

// 13. Interface properties
export interface IReadable {
  item: Item;
  readonly container: Container;
  items: Item[];
  itemsByKey: Record<string, Item>;
}

// 14. Generics with output symbols
export class Generic {
  boxed: Box<Widget>;
  boxedMultiple: Box<Widget | Item>;
}

// 15. Generic parameter properties
export function process<T extends Item>(item: T): Box<T> {
  return new Box<T>();
}

// 16. Method return types and parameters
export class Methods {
  getItem(): Item {
    return new Item();
  }

  processItems(items: Item[]): Container {
    return new Container();
  }

  getBox(widget: Widget): Box<Widget> {
    return new Box<Widget>();
  }
}
