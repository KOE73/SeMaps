# SeMaps for Visual Studio — prototype

The existing SeMaps editor opens inside Visual Studio using a WPF tool window and
WebView2CompositionControl. No SeMaps editor, host, core or extractor changes are needed
for this prototype. The composition control keeps the page visible across VS docking changes.

## What works

- On solution open (including an already open solution when the package loads), find
  `*.semaps` beside the solution, then in its parent folders. The nearest matching
  directory wins; multiple files require a choice. No file means no server launch.
- Connect automatically to the host recorded in `.semaps/host.json`, verifying its
  workspace **and** source root using `/api/info`.
- If no matching host is live, start the installed `semaps.exe --here --no-browser <file>`
  without a console or browser. The executable is found in the user installation,
  then PATH. Closing Visual Studio/solution leaves the shared host running.
- Open SeMaps via **View → Other Windows → SeMaps** (`View.SeMaps`). Reconnect retries
  discovery; files created after opening the solution are picked up on reconnect.
- A correlated JSON handshake confirms WebView2 messaging. It advertises **no editor
  commands**; the generic editor bridge is the next stage.
- The bottom drop zone reports the native Visual Studio drag formats and file paths.
  It does not parse symbols or change a map. This is a diagnostic stage, not final UX.

## Build and try

Requires Visual Studio 2022/2026 with VSSDK, .NET Framework 4.7.2 targeting pack,
.NET 10 SDK for the test runner, WebView2 Runtime and an installed SeMaps.
The build pins VS SDK/BuildTools/WebView2 versions in the VSIX project.
Use full Visual Studio MSBuild; VSIX deployment is not supported by `dotnet build`.

From the repository root:

```powershell
./plugins/visual-studio/scripts/Build-Vsix.ps1 -Configuration Release
./plugins/visual-studio/scripts/Build-Vsix.ps1 -Configuration Debug -Deploy
./plugins/visual-studio/scripts/Start-Experimental.ps1
npm ci --prefix plugins/protocol
npm test --prefix plugins/protocol
dotnet run --project plugins/visual-studio/tests/SeMaps.VisualStudio.Tests
```

Deployment uses the **SeMapsDev** experimental profile only. The scripts do not install
into the normal Visual Studio profile, reset experimental profiles or kill running hosts.
Close the experimental VS before rebuilding/deploying a changed DLL.

Artifact: `src/SeMaps.VisualStudio/bin/Release/net472/SeMaps.VisualStudio.vsix`.
Solution: `SeMaps.VisualStudio.sln`. A repository containing this solution may have
its `.semaps` farther up the directory tree; there is no hardcoded SeMaps workspace.

The startup script opens an interactive test instance intentionally, with a log under
the temporary folder. Successful SDK compilation alone does not prove UI behavior.

## Verified on 2026-10-07

Visual Studio Enterprise 2026 18.10.3, VSSDK, MSBuild, WebView2 Runtime:

- Debug VSIX built and deployed to `18.0_ae73be00SeMapsDev`.
- Opening this solution automatically started the repository's installed SeMaps host.
- Another experimental launch reused the same server PID and endpoint.
- Existing editor and architecture view rendered inside the VS window.
- Release composition control rendered in the document area and survived a transition
  to a floating window; the ordinary WPF WebView2 had gone blank on docking in this test.
- ActivityLog and UI confirmed a `bridge.initialize` request/response round-trip.
- A real drag of `HostConnection.cs` from Solution Explorer yielded `FileDrop` and
  `CF_VSSTGPROJECTITEMS` (plus other native formats) and the correct full source path.
- Discovery/host unit checks and transport checks passed.

VS 2022, symbol navigation, a full symbol-choice dialog, and placement on the map have
not been tested/implemented in this prototype. See [the remaining plan](../../docs/plans/PLAN_20261007_editor_ide-bridge-and-visual-studio.md).
