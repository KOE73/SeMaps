/* Transport probe for IDE integration v1. Does not claim any editor capabilities. */
(() => {
  if (globalThis.__semapsIdeProbe || !globalThis.chrome?.webview) return;
  globalThis.__semapsIdeProbe = true;
  globalThis.chrome.webview.addEventListener("message", (event) => {
    const request = event.data;
    if (!request || request.protocol !== "semaps.ide" || request.version !== 1
      || request.kind !== "request" || request.method !== "bridge.initialize"
      || typeof request.id !== "string" || !request.id
      || typeof request.sessionId !== "string" || !request.sessionId) return;
    globalThis.chrome.webview.postMessage({
      protocol: "semaps.ide", version: 1, sessionId: request.sessionId,
      kind: "response", requestId: request.id,
      result: { capabilities: [], transport: "webview2", probe: true },
    });
  });
})();
