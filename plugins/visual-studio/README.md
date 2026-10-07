# SeMaps for Visual Studio

The existing editor runs in a WPF tool window with WebView2CompositionControl.
IDE parsing, source navigation and native drag handling live in the VSIX. The editor
has an optional, transport-independent [IDE bridge](../../docs/IDE_BRIDGE.md).

## Behavior

- Opening a solution never starts SeMaps or opens the map. Use **View → Other Windows
  → SeMaps** (`View.SeMaps`) or Reconnect. Only these actions discover the nearest
  `*.semaps`, prompt for ambiguous configurations, reuse a matching host or start
  `semaps.exe --here --no-browser`. Both workspace and source root are verified.
- Switching/closing the solution disconnects the map. Closing VS leaves the shared
  host running. Reconnect finds configurations created after opening the solution.
- Selecting a map block selects its single bound file in Solution Explorer without
  opening the source. Disable the checkbox to stop following selections.
- Clicking a code binding opens the native editor at the bound line. Browser code
  viewing remains the fallback if the IDE declines or the bridge is unavailable.
- Drag C# source files from Solution Explorer onto the canvas or the bottom area.
  Roslyn resolves types using the loaded solution and project references. A dialog
  lets you choose types, shows assembly/file/line and marks unsaved buffers. After
  confirmation, existing model entities are placed through normal editor operations
  with one Undo. Repeated drops select existing blocks without duplicates.
- The button **Выбрать тип из редактора на карте** resolves the containing type at
  the last active code editor's caret, then selects/reveals its existing block.
  It never creates a placement automatically.
- Missing facts/model entities and ambiguity are reported. Extraction and sync are
  separate actions; neither runs as a hidden effect of navigation or drop.

## Build and test

Requires Visual Studio 2022/2026 with VSSDK, the .NET Framework 4.7.2 targeting pack,
.NET 10 SDK for tests, WebView2 Runtime, and SeMaps built with the IDE bridge.
Versions are pinned in the projects. Roslyn runtime comes from Visual Studio;
the VSIX does not package another Workspaces runtime.

From the repository root:

```powershell
npm ci --prefix editor
npm ci --prefix plugins/protocol
npm test --prefix plugins/protocol
npm run check:ide-bridge --prefix editor
dotnet run --project plugins/visual-studio/tests/SeMaps.VisualStudio.Tests
./plugins/visual-studio/scripts/Build-Vsix.ps1 -Configuration Release
./plugins/visual-studio/scripts/Build-Vsix.ps1 -Configuration Release -Deploy
./plugins/visual-studio/scripts/Start-Experimental.ps1
```

Deployment targets **SeMapsDev** only. It does not install into normal VS, reset its
profile or stop shared hosts. Close the experimental IDE before deploying changes.
Use full Visual Studio MSBuild; `dotnet build` cannot deploy the VSIX.

Artifact: `src/SeMaps.VisualStudio/bin/Release/net472/SeMaps.VisualStudio.vsix`.
Solution: `SeMaps.VisualStudio.sln`. Paths come from the consuming project.

For a reproducible isolated trial with a local editor/host, use PowerShell 7:

```powershell
$trial = ./plugins/visual-studio/scripts/Prepare-IntegrationTrial.ps1
./plugins/visual-studio/scripts/Start-Experimental.ps1 -Solution $trial.Solution -HostExecutable $trial.HostExecutable
```

The trial script builds the editor and host and creates an ignored workspace/solution
under `plugins/visual-studio/bin/ide-trials`. It includes one placed and two unplaced
entities bound to plugin source files. `-HostExecutable <exe>` copies an existing
host instead of rebuilding Go. Start-Experimental passes this override only to its
child process through `SEMAPS_VS_HOST_EXE`; the installed SeMaps and global environment
are unchanged. Normal executable search uses user installation, then PATH. An older
installed editor can render but needs updating for bridge commands.

## Verified on 2026-10-07

Enterprise 2026 18.10.3, installed VSSDK and WebView2 Runtime:

- Release VSIX compiled and deployed to the experimental profile.
- Cold solution startup did not launch the fixture host; explicit Reconnect did.
- Production bridge handshake, map selection → Solution Explorer and code binding
  → native editor at line 12 worked in the live IDE.
- Native ProjectPicker.cs drag → Roslyn choice dialog → placement at drop point
  worked. One editor Undo removed the block.
- The caret command resolved HostConnection in its two loaded project contexts,
  asked for a project choice and selected/revealed the existing block.
- Cancelling the native picker left the map unchanged. Dropping HostConnection
  again selected its existing block without moving or duplicating it.
- Editor bridge, common envelope/probe fixtures, discovery/host, Roslyn identity,
  correlation, timeout, replay and invalid-payload checks passed.

VS 2022 and multiple-monitor DPI are not live verified. Only diagram views, own C#
type declarations and FileDrop are supported. Members and external metadata symbols
are not resolved by this adapter. Multiple model matches are reported; a second
entity-choice dialog is a remaining step. The iframe transport is tested with a fake
peer; no VS Code extension is included. See the [plan](../../docs/plans/PLAN_20261007_editor_ide-bridge-and-visual-studio.md).
