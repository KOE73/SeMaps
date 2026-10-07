# SeMaps IDE protocol

Protocol name: `semaps.ide`; version: `1`. IDE plugins live outside the editor.
JSON message semantics are independent of the WebView technology.

This first prototype implements **only a transport probe** (`bridge.initialize`).
It advertises an empty capability list. Selection, symbol lookup, navigation and placement
are planned in [the implementation plan](../../docs/plans/PLAN_20261007_editor_ide-bridge-and-visual-studio.md).
They must not be claimed as working by an adapter until implemented.

`schemas/message.schema.json` defines the common envelope. Requests use `id`;
responses use `requestId` and exactly one of `result` or `error`. The receiver checks
protocol, version, session and request correlation. Each new attachment uses a new
session; an answer from an earlier session is ignored. `origin` and `causationId`
are reserved for correlating selection events without feedback loops.

The sender negotiates capabilities before sending editor commands. A transport probe
must not inspect the editor's DOM, intercept internal events or change the model.
`probe.js` is temporarily embedded by the VSIX, so an installed SeMaps can be tried
without changing or replacing its executable. The production editor bridge will use
the same envelope but live in the editor's optional integration adapter.

Run the transport contract checks:

```powershell
npm ci --prefix plugins/protocol
npm test --prefix plugins/protocol
```
