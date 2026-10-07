using System;
using System.IO;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using Microsoft.VisualStudio.Shell;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;
using Newtonsoft.Json.Linq;

namespace SeMaps.VisualStudio;

internal sealed class SeMapsControl : UserControl
{
    private readonly WebView2CompositionControl browser = new WebView2CompositionControl();
    private readonly TextBlock status = new TextBlock { Margin = new Thickness(6), TextWrapping = TextWrapping.Wrap };
    private readonly TextBlock probeStatus = new TextBlock { Margin = new Thickness(6), Text = "Bridge: waiting for page" };
    private readonly CheckBox follow = new CheckBox { Content = "Выделять файл по выбору на карте", IsChecked = true, Margin = new Thickness(6) };
    private readonly Border dropTarget = new Border
    {
        AllowDrop = true, Padding = new Thickness(8), Background = System.Windows.Media.Brushes.AliceBlue,
        Child = new TextBlock { Text = "Перетащите C# файл на карту или сюда, затем выберите типы для добавления.", TextWrapping = TextWrapping.Wrap },
    };
    private readonly IdeBridgeClient bridge;
    private CancellationTokenSource page = new CancellationTokenSource();
    private Uri? endpoint;
    private bool initialized;
    public Action? Reconnect { get; set; }
    public Action? LocateSource { get; set; }
    public Action<string[], JObject?>? FilesDropped { get; set; }
    public Func<string, JObject, Task<JObject>>? RequestReceived { set => bridge.RequestReceived = value; }
    public Action<string, JObject, string?>? EventReceived { set => bridge.EventReceived = value; }
    public ISet<string> Capabilities => bridge.Capabilities;
    public bool FollowSelection => follow.IsChecked == true;
    public Task<JObject> RequestAsync(string method, JObject parameters, CancellationToken token) => bridge.RequestAsync(method, parameters, token);
    public void DropStatus(string text) => ((TextBlock)dropTarget.Child).Text = text;

    public SeMapsControl()
    {
        bridge = new IdeBridgeClient(message => browser.CoreWebView2.PostWebMessageAsJson(message.ToString(Newtonsoft.Json.Formatting.None)));
        status.SetResourceReference(TextBlock.ForegroundProperty, VsBrushes.WindowTextKey);
        probeStatus.SetResourceReference(TextBlock.ForegroundProperty, VsBrushes.WindowTextKey);
        follow.SetResourceReference(CheckBox.ForegroundProperty, VsBrushes.WindowTextKey);
        var root = new DockPanel();
        var header = new StackPanel();
        var retry = new Button { Content = "Reconnect", HorizontalAlignment = HorizontalAlignment.Left, Margin = new Thickness(6) };
        retry.Click += (_, __) =>
        {
            ThreadHelper.ThrowIfNotOnUIThread();
            if (Reconnect != null) Reconnect();
            else (Package.GetGlobalService(typeof(EnvDTE.DTE)) as EnvDTE.DTE)?.ExecuteCommand("View.SeMaps");
        };
        var locate = new Button { Content = "Выбрать тип из редактора на карте", HorizontalAlignment = HorizontalAlignment.Left, Margin = new Thickness(6) };
        locate.Click += (_, __) => LocateSource?.Invoke();
        var actions = new StackPanel { Orientation = Orientation.Horizontal };
        actions.Children.Add(retry); actions.Children.Add(locate);
        header.Children.Add(status); header.Children.Add(actions); header.Children.Add(follow); header.Children.Add(probeStatus);
        DockPanel.SetDock(header, Dock.Top); root.Children.Add(header);
        DockPanel.SetDock(dropTarget, Dock.Bottom); root.Children.Add(dropTarget);
        root.Children.Add(browser); Content = root;
        AllowDrop = true;
        browser.AllowDrop = true;
        PreviewDragOver += (_, e) =>
        {
            e.Effects = e.Data.GetDataPresent(DataFormats.FileDrop) || e.Data.GetDataPresent("CF_VSSTGPROJECTITEMS")
                || e.Data.GetDataPresent("CF_VSREFPROJECTITEMS") ? DragDropEffects.Copy : DragDropEffects.None;
            e.Handled = true;
        };
        PreviewDrop += (_, e) =>
        {
            try
            {
                var files = e.Data.GetData(DataFormats.FileDrop) as string[];
                if (files == null || files.Length == 0) { DropStatus("No source files in this drag payload."); return; }
                var point = e.GetPosition(browser);
                var at = point.X >= 0 && point.Y >= 0 && point.X <= browser.ActualWidth && point.Y <= browser.ActualHeight
                    ? new JObject { ["space"] = "client", ["x"] = point.X, ["y"] = point.Y } : null;
                DropStatus("Reading source files…");
                FilesDropped?.Invoke(files, at);
            }
            catch (Exception ex) { ((TextBlock)dropTarget.Child).Text = "Could not read the drag payload: " + ex.Message; }
            e.Handled = true;
        };
    }

    public void SetStatus(string text) => status.Text = text;
    public void Disconnect()
    {
        endpoint = null;
        page.Cancel(); page.Dispose(); page = new CancellationTokenSource();
        bridge.Reset();
        probeStatus.Text = "Bridge: disconnected";
        if (initialized) browser.CoreWebView2.Navigate("about:blank");
    }

    public async Task ConnectAsync(Uri address, CancellationToken cancellationToken)
    {
        await ThreadHelper.JoinableTaskFactory.SwitchToMainThreadAsync(cancellationToken);
        endpoint = address;
        page.Cancel(); page.Dispose(); page = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        bridge.Reset();
        if (!initialized)
        {
            var cache = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "SeMaps", "VisualStudio", "WebView2");
            var environment = await CoreWebView2Environment.CreateAsync(null, cache);
            cancellationToken.ThrowIfCancellationRequested();
            await browser.EnsureCoreWebView2Async(environment);
            cancellationToken.ThrowIfCancellationRequested();
            browser.CoreWebView2.NavigationStarting += (_, e) =>
            {
                if (e.Uri == "about:blank") return;
                if (endpoint == null || !Uri.TryCreate(e.Uri, UriKind.Absolute, out var target)
                    || !SameOrigin(target, endpoint)) e.Cancel = true;
            };
            browser.CoreWebView2.NewWindowRequested += (_, e) => e.Handled = true;
            browser.CoreWebView2.WebMessageReceived += OnWebMessageReceived;
            browser.CoreWebView2.NavigationCompleted += OnNavigationCompleted;
            initialized = true;
        }
        browser.CoreWebView2.Navigate(address.AbsoluteUri);
    }

    // WebView2 exposes a void event handler; catch all failures at the event boundary.
#pragma warning disable VSTHRD100
    private async void OnWebMessageReceived(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        if (endpoint == null || !Uri.TryCreate(e.Source, UriKind.Absolute, out var source) || !SameOrigin(source, endpoint)) return;
        try { await bridge.ReceiveAsync(JObject.Parse(e.WebMessageAsJson)); }
        catch (Exception ex) { ActivityLog.LogError("SeMaps", ex.ToString()); }
    }

    private async void OnNavigationCompleted(object? sender, CoreWebView2NavigationCompletedEventArgs e)
    {
        try { if (endpoint != null && e.IsSuccess) await InitializeBridgeAsync(page.Token); }
        catch (OperationCanceledException) { }
        catch (Exception ex) { ActivityLog.LogError("SeMaps", ex.ToString()); }
    }
#pragma warning restore VSTHRD100

    private async Task InitializeBridgeAsync(CancellationToken token)
    {
        // The application may still be loading its optional integration module.
        for (var attempt = 0; attempt < 10; attempt++)
        {
            try
            {
                var result = await bridge.RequestAsync("bridge.initialize", new JObject { ["capabilities"] = new JArray("source.open") }, token, 1000);
                await ThreadHelper.JoinableTaskFactory.SwitchToMainThreadAsync(token);
                if (result["capabilities"] is JArray capabilities)
                    foreach (var capability in capabilities) if (capability.Type == JTokenType.String) bridge.Capabilities.Add((string)capability!);
                probeStatus.Text = bridge.Capabilities.Contains("context.get") ? "Bridge v1: editor connected" : "Bridge: this editor has no IDE commands. Update SeMaps.";
                ActivityLog.LogInformation("SeMaps", probeStatus.Text);
                return;
            }
            catch (OperationCanceledException) when (!token.IsCancellationRequested) { }
            await Task.Delay(250, token);
        }
        probeStatus.Text = "Bridge unavailable. Update SeMaps to a build with IDE integration.";
    }

    private static bool SameOrigin(Uri a, Uri b) => a.Scheme == b.Scheme && a.Host == b.Host && a.Port == b.Port;
}
