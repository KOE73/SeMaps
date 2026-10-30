/** Only transport adapters know about the embedding technology. */
export interface IdeTransport {
  send(message: unknown): void;
  subscribe(receive: (message: unknown) => void): () => void;
}

interface WebViewPort {
  postMessage(message: unknown): void;
  addEventListener(type: "message", listener: (event: MessageEvent) => void): void;
  removeEventListener(type: "message", listener: (event: MessageEvent) => void): void;
}

export function webViewTransport(port: WebViewPort): IdeTransport {
  return {
    send: (message) => port.postMessage(message),
    subscribe(receive) {
      const listener = (event: MessageEvent) => receive(event.data);
      port.addEventListener("message", listener);
      return () => port.removeEventListener("message", listener);
    },
  };
}

/** The embedding application supplies a known peer and an exact trusted origin. */
export function frameTransport(peer: Window, origin: string, receiver: Window = window): IdeTransport {
  if (!origin || origin === "*" || new URL(origin).origin !== origin) throw new Error("An exact frame origin is required");
  return {
    send: (message) => peer.postMessage(message, origin),
    subscribe(receive) {
      const listener = (event: MessageEvent) => {
        if (event.source === peer && event.origin === origin) receive(event.data);
      };
      receiver.addEventListener("message", listener);
      return () => receiver.removeEventListener("message", listener);
    },
  };
}

/** A normal browser has no implicit bridge or window-message listener. */
export function embeddedTransport(): IdeTransport | undefined {
  const port = (window as Window & { chrome?: { webview?: WebViewPort } }).chrome?.webview;
  return port ? webViewTransport(port) : undefined;
}
