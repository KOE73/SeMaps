using System;
using System.ComponentModel.Design;
using System.Net.Http;
using System.Runtime.InteropServices;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.VisualStudio;
using Microsoft.VisualStudio.Shell;
using Microsoft.VisualStudio.Shell.Interop;
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
    private string status = "Open a solution containing a .semaps project file.";

    protected override async Task InitializeAsync(CancellationToken cancellationToken, IProgress<ServiceProgressData> progress)
    {
        await JoinableTaskFactory.SwitchToMainThreadAsync(cancellationToken);
        solution = await GetServiceAsync(typeof(SVsSolution)) as IVsSolution
            ?? throw new InvalidOperationException("Visual Studio solution service is unavailable.");
        ErrorHandler.ThrowOnFailure(solution.AdviseSolutionEvents(this, out solutionCookie));
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
        control?.Disconnect();
        SetStatus("Open SeMaps or click Reconnect to load this solution's map.");
        return VSConstants.S_OK;
    }
    public int OnAfterCloseSolution(object reserved)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        pending.Cancel();
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
}
