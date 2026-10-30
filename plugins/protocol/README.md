# SeMaps IDE protocol

Protocol name: `semaps.ide`; version: `1`. IDE plugins live outside the editor.
JSON message semantics are independent of the WebView technology.

The optional production adapter implements context, symbol lookup, selection, reveal
and placement in diagram views, plus source opening through the IDE. The normative
command contract is [IDE_BRIDGE.md](../../docs/IDE_BRIDGE.md).

`schemas/message.schema.json` defines the common envelope. Requests use `id`;
responses use `requestId` and exactly one of `result` or `error`. The receiver checks
protocol, version, session and request correlation. Each new attachment uses a new
session; an answer from an earlier session is ignored. `origin` and `causationId`
correlate selection events without feedback loops.

The sender negotiates capabilities before sending editor commands. A transport probe
must not inspect the editor's DOM, intercept internal events or change the model.
`probe.js` and its empty-capability handshake examples remain isolated transport
fixtures. The VSIX no longer injects this probe into the editor. Production integration
lives in `editor/src/integration/`; it requires an editor build containing that adapter.

Run the transport contract checks:

```powershell
npm ci --prefix plugins/protocol
npm test --prefix plugins/protocol
npm run check:ide-bridge --prefix editor
```
