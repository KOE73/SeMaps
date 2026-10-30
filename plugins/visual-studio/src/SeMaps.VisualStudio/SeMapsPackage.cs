using System;
using System.ComponentModel.Design;
using System.Net.Http;
using System.Runtime.InteropServices;
using System.Threading;
using System.Threading.Tasks;
using System.Linq;
using System.IO;
using Microsoft.VisualStudio;
using Microsoft.VisualStudio.Shell;
using Microsoft.VisualStudio.Shell.Interop;
using Microsoft.VisualStudio.ComponentModelHost;
using Microsoft.VisualStudio.LanguageServices;
using Newtonsoft.Json.Linq;
using Task = System.Threading.Tasks.Task;

namespace SeMaps.VisualStudio;

[PackageRegistration(UseManagedResourcesOnly = true, AllowsBackgroundLoading = true)]
[ProvideMenuResource("Menus.ctmenu", 1)]
[ProvideToolWindow(typeof(SeMapsToolWindow), Style = VsDockStyle.MDI, Width = 1100, Height = 750)]
[Guid(PackageGuid)]
public sealed class SeMapsPackage : AsyncPackage, IVsSolutionEvents
{
    public const string PackageGuid = "d4690355-5ac0-41bb-ae4a-79c6cd7a4f3a";
    private static readonly Guid CommandSet = new Guid("30a2c5b4-0609-4f39-b7e3-40724427c2bc");
    private readonly HostConnection host = new HostConnection(new HttpClient());
    private CancellationTokenSource pending = new CancellationTokenSource();
    private IVsSolution? solution;
    private uint solutionCookie;
    private SeMapsControl? control;
    private SeMapsProject? currentProject;
    private VisualStudioWorkspace? roslyn;
    private bool dropBusy;
    private string status = "Open a solution containing a .semaps project file.";

    protected override async Task InitializeAsync(CancellationToken cancellationToken, IProgress<ServiceProgressData> progress)
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync(cancellationToken);
        solution = await GetServiceAsync(typeof(SVsSolution)) as IVsSolution
            ?? throw new InvalidOperationException("Visual Studio solution service is unavailable.");
        ErrorHandler.ThrowOnFailure(solution.AdviseSolutionEvents(this, out solutionCookie));
        var components = await GetServiceAsync(typeof(SComponentModel)) as IComponentModel;
        roslyn = components?.GetService<VisualStudioWorkspace>();
        var menu = await GetServiceAsync(typeof(IMenuCommandService)) as OleMenuCommandService
            ?? throw new InvalidOperationException("Visual Studio command service is unavailable.");
        menu.AddCommand(new MenuCommand((sender, args) => Run(async () =>
        {
            await ShowWindowAsync();
            await ConnectSolutionAsync();
        }), new CommandID(CommandSet, 0x0100)));
    }

    private void Run(Func<Task> action) => JoinableTaskFactory.RunAsync(async () =>
    {
        try { await action(); }
        catch (OperationCanceledException) { }
        catch (Exception ex)
        {
            await JoinableTaskFactory.SwitchToMainThreadAsync();
            SetStatus("SeMaps: " + ex.Message);
            ActivityLog.LogError("SeMaps", ex.ToString());
        }
    }).FileAndForget("SeMaps/Integration");

    private async Task ConnectSolutionAsync()
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        pending.Cancel();
        pending.Dispose();
        pending = new CancellationTokenSource();
        var token = pending.Token;
        control?.Disconnect();
        currentProject = null;
        if (solution == null) return;
        ErrorHandler.ThrowOnFailure(solution.GetSolutionInfo(out var directory, out var solutionFile, out var optionsFile));
        if (string.IsNullOrWhiteSpace(directory)) return;
        // Disk parsing and HTTP/process work must not block the IDE's UI thread.
        var files = await Task.Run(() => ProjectDiscovery.Find(directory), token);
        await JoinableTaskFactory.SwitchToMainThreadAsync(token);
        if (files.Count == 0)
        {
            SetStatus("No *.semaps found beside the solution or in its parent folders.");
            control?.Disconnect();
            return;
        }
        var file = files.Count == 1 ? files[0] : ProjectPicker.Choose(files);
        if (file == null) { SetStatus("SeMaps project selection cancelled."); return; }
        SetStatus("Starting or connecting to SeMaps…");
        var project = await Task.Run(() => new SeMapsProject(file), token);
        var endpoint = await Task.Run(() => host.EnsureAsync(project, token), token);
        await JoinableTaskFactory.SwitchToMainThreadAsync(token);
        currentProject = project;
        SetStatus(System.IO.Path.GetFileName(file) + " · " + endpoint);
        // Called only by the explicit Open or Reconnect actions.
        if (control != null) await control.ConnectAsync(endpoint, token);
        ActivityLog.LogInformation("SeMaps", "Connected " + file + " to " + endpoint);
    }

    private async Task ShowWindowAsync()
    {
        var pane = (SeMapsToolWindow)await ShowToolWindowAsync(typeof(SeMapsToolWindow), 0, true, DisposalToken);
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        control = (SeMapsControl)pane.Content;
        control.SetStatus(status);
        control.Reconnect = () => Run(ConnectSolutionAsync);
        control.RequestReceived = OpenSourceAsync;
        control.EventReceived = (method, parameters, origin) =>
        {
            if (method == "selection.changed" && origin != "ide") Run(() => FollowSelectionAsync(parameters));
        };
        control.FilesDropped = (files, at) => Run(() => DropFilesAsync(files, at));
        control.LocateSource = () => Run(LocateSourceAsync);
    }

    private void SetStatus(string value)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        status = value;
        control?.SetStatus(value);
    }

    public int OnAfterOpenSolution(object reserved, int newSolution)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        pending.Cancel();
        currentProject = null;
        control?.Disconnect();
        SetStatus("Open SeMaps or click Reconnect to load this solution's map.");
        return VSConstants.S_OK;
    }
    public int OnAfterCloseSolution(object reserved)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        pending.Cancel();
        currentProject = null;
        control?.Disconnect(); SetStatus("Solution closed. The shared SeMaps server remains running.");
        return VSConstants.S_OK;
    }
    protected override void Dispose(bool disposing)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        if (disposing)
        {
            pending.Cancel(); pending.Dispose();
            if (solution != null && solutionCookie != 0) solution.UnadviseSolutionEvents(solutionCookie);
        }
        base.Dispose(disposing);
    }
    public int OnAfterOpenProject(IVsHierarchy hierarchy, int added) => VSConstants.S_OK;
    public int OnQueryCloseProject(IVsHierarchy hierarchy, int removing, ref int cancel) => VSConstants.S_OK;
    public int OnBeforeCloseProject(IVsHierarchy hierarchy, int removed) => VSConstants.S_OK;
    public int OnAfterLoadProject(IVsHierarchy stub, IVsHierarchy real) => VSConstants.S_OK;
    public int OnQueryUnloadProject(IVsHierarchy real, ref int cancel) => VSConstants.S_OK;
    public int OnBeforeUnloadProject(IVsHierarchy real, IVsHierarchy stub) => VSConstants.S_OK;
    public int OnQueryCloseSolution(object reserved, ref int cancel) => VSConstants.S_OK;
    public int OnBeforeCloseSolution(object reserved) => VSConstants.S_OK;

    private async Task<JObject> OpenSourceAsync(string method, JObject parameters)
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        if (currentProject == null) return new JObject { ["opened"] = false };
        var relative = parameters["file"]?.Type == JTokenType.String ? (string)parameters["file"]! : "";
        var file = SourcePaths.Resolve(currentProject.SourceRoot, relative);
        if (!File.Exists(file)) return new JObject { ["opened"] = false };
        VsShellUtilities.OpenDocument(this, file, VSConstants.LOGVIEWID_Code, out var hierarchy, out var item, out var frame, out var view);
        var line = parameters["line"]?.Type == JTokenType.Integer ? (int)parameters["line"]! : 1;
        var column = parameters["column"]?.Type == JTokenType.Integer ? (int)parameters["column"]! : 1;
        if (line > 0 && column > 0 && view != null) view.SetCaretPos(line - 1, column - 1);
        frame.Show();
        SelectFile(file);
        return new JObject { ["opened"] = true };
    }

    private async Task FollowSelectionAsync(JObject parameters)
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        if (control?.FollowSelection != true || currentProject == null || !(parameters["objects"] is JArray objects)) return;
        var primary = (string?)parameters["primaryId"];
        var selected = objects.OfType<JObject>().FirstOrDefault(o => (string?)o["entityId"] == primary);
        var files = (selected?["sources"] as JArray)?.OfType<JObject>().Select(s => (string?)s["file"])
            .Where(f => !string.IsNullOrWhiteSpace(f)).Distinct(StringComparer.OrdinalIgnoreCase).ToArray();
        if (files?.Length == 1) SelectFile(SourcePaths.Resolve(currentProject.SourceRoot, files[0]!));
    }

    private void SelectFile(string file)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        if (solution == null || roslyn == null) return;
        foreach (var documentId in roslyn.CurrentSolution.GetDocumentIdsWithFilePath(file))
        {
            var project = roslyn.CurrentSolution.GetProject(documentId.ProjectId);
            var hierarchy = project == null ? null : roslyn.GetHierarchy(project.Id);
            if (hierarchy == null || ErrorHandler.Failed(hierarchy.ParseCanonicalName(file, out var itemId))) continue;
            var explorer = VsShellUtilities.GetUIHierarchyWindow(this, new Guid(ToolWindowGuids80.SolutionExplorer));
            if (hierarchy is IVsUIHierarchy uiHierarchy) explorer?.ExpandItem(uiHierarchy, itemId, EXPANDFLAGS.EXPF_SelectItem);
            return;
        }
    }

    private async Task LocateSourceAsync()
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        if (control == null || currentProject == null || roslyn == null) return;
        if (!control.Capabilities.Contains("symbols.resolve")) { control.DropStatus("Обновите SeMaps: IDE bridge недоступен."); return; }
        var manager = await GetServiceAsync(typeof(Microsoft.VisualStudio.TextManager.Interop.SVsTextManager)) as Microsoft.VisualStudio.TextManager.Interop.IVsTextManager;
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        if (manager == null || ErrorHandler.Failed(manager.GetActiveView(0, null, out var view)) || view == null)
        { control.DropStatus("Откройте C# исходник и поставьте курсор внутри нужного типа."); return; }
        ErrorHandler.ThrowOnFailure(view.GetCaretPos(out var line, out var column));
        ErrorHandler.ThrowOnFailure(view.GetBuffer(out var buffer));
        if (!(buffer is IPersistFileFormat persisted)) return;
        ErrorHandler.ThrowOnFailure(persisted.GetCurFile(out var file, out var format));
        var token = pending.Token;
        var context = (JObject)(await control.RequestAsync("context.get", new JObject(), token))["context"]!;
        var symbols = await SymbolResolver.AtAsync(roslyn.CurrentSolution, currentProject.SourceRoot, file, line, column, token);
        await JoinableTaskFactory.SwitchToMainThreadAsync(token);
        if (symbols.Count == 0) { control.DropStatus("В позиции курсора нет объявления C# типа."); return; }
        var chosen = symbols.Count == 1 ? symbols : SymbolPicker.Choose(symbols);
        if (chosen == null || chosen.Count == 0) return;
        var resolved = await control.RequestAsync("symbols.resolve", new JObject { ["context"] = context,
            ["symbols"] = new JArray(chosen.Select(c => new JObject { ["language"] = "csharp", ["symbolId"] = c.SymbolId, ["file"] = c.File, ["assembly"] = c.Assembly })) }, token);
        var results = ((JArray)resolved["results"]!).OfType<JObject>().ToArray();
        var ids = results.Where(r => (string?)r["status"] == "alreadyOnView")
            .SelectMany(r => ((JArray)r["candidates"]!).OfType<JObject>().Select(c => (string)c["entityId"]!)).Distinct().ToArray();
        if (ids.Length > 0)
        {
            await control.RequestAsync("selection.set", new JObject { ["context"] = context, ["entityIds"] = new JArray(ids) }, token);
            await control.RequestAsync("entities.reveal", new JObject { ["context"] = context, ["entityIds"] = new JArray(ids) }, token);
        }
        await JoinableTaskFactory.SwitchToMainThreadAsync(token);
        control.DropStatus("Выбрано на карте: " + ids.Length + ". " + string.Join("; ", results.Select(r => (string?)r["symbol"]?["symbolId"] + ": " + (string?)r["status"])));
    }

    private async Task DropFilesAsync(string[] files, JObject? at)
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync();
        if (dropBusy || currentProject == null || control == null) return;
        if (!control.Capabilities.Contains("symbols.resolve")) { control.DropStatus("Обновите SeMaps: редактор ещё не поддерживает IDE bridge."); return; }
        if (roslyn == null) { control.DropStatus("Roslyn workspace недоступен."); return; }
        dropBusy = true;
        var token = pending.Token;
        try
        {
            var context = (await control.RequestAsync("context.get", new JObject(), token))["context"] as JObject
                ?? throw new InvalidOperationException("Editor returned no context.");
            var snapshot = roslyn.CurrentSolution;
            var sourceRoot = currentProject.SourceRoot;
            var candidates = await Task.Run(() => SymbolResolver.TypesAsync(snapshot, sourceRoot, files, token), token);
            await JoinableTaskFactory.SwitchToMainThreadAsync(token);
            if (candidates.Count == 0) { control.DropStatus("В файлах не найдены объявления типов."); return; }
            var chosen = SymbolPicker.Choose(candidates);
            if (chosen == null || chosen.Count == 0) { control.DropStatus("Добавление отменено."); return; }
            var resolved = await control.RequestAsync("symbols.resolve", new JObject { ["context"] = context,
                ["symbols"] = new JArray(chosen.Select(c => new JObject { ["language"] = "csharp", ["symbolId"] = c.SymbolId, ["file"] = c.File, ["assembly"] = c.Assembly })) }, token);
            await JoinableTaskFactory.SwitchToMainThreadAsync(token);
            var results = (resolved["results"] as JArray)?.OfType<JObject>().ToArray() ?? Array.Empty<JObject>();
            var accepted = results.Where(r => (string?)r["status"] == "resolved" || (string?)r["status"] == "alreadyOnView").ToArray();
            var ids = accepted.SelectMany(r => ((JArray)r["candidates"]!).OfType<JObject>().Select(c => (string)c["entityId"]!)).Distinct().ToArray();
            var problems = results.Except(accepted).Select(r => (string?)r["symbol"]?["symbolId"] + ": " + (string?)r["status"]).ToArray();
            if (ids.Length > 0)
            {
                var parameters = new JObject { ["context"] = context, ["entityIds"] = new JArray(ids) };
                if (at != null) parameters["at"] = at;
                await control.RequestAsync("entities.place", parameters, token);
            }
            control.DropStatus("Выбрано: " + chosen.Count + "; найдено в модели: " + ids.Length
                + (chosen.Any(c => c.Unsaved) ? ". Есть несохранённые буферы; граф отражает последнее извлечение." : "")
                + (problems.Length > 0 ? "\n" + string.Join("\n", problems) + "\nnotFound: проверьте извлечение; notInModel: требуется сверка. Автоматически не выполняются." : ""));
        }
        catch (Exception ex) { control.DropStatus("Не удалось добавить: " + ex.Message); ActivityLog.LogError("SeMaps", ex.ToString()); }
        finally { dropBusy = false; }
    }
}
