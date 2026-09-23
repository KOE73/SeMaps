export interface Greeter {
  greet(name: string): string;
}

export class Base {
  public label: string = "base";
}

class Hidden {
  secret = 1;
  #password = "shh";
}

export class Widget extends Base implements Greeter {
  public id: string = "w1";
  private token: string = "secret";

  greet(name: string): string {
    return `hi ${name}`;
  }
}

export enum Color {
  Red,
  Green = "green",
}

export interface Box {
  value: number;
}

export type Pair<T> = { first: T; second: Box };

export function makeWidget(): Widget {
  return new Widget();
}

export const VERSION = 1;

export const build = (): Widget => makeWidget();

export namespace Registry {
  export interface Entry {
    box: Box;
  }

  export class Store {
    public items: Entry[] = [];
  }
}

function helper(x: Box): Box {
  return x;
}

export { helper };
