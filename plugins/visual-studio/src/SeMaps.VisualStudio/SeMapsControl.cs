using System;
using System.IO;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using Microsoft.VisualStudio.Shell;
using Microsoft.VisualStudio.Shell.Interop;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;
using Newtonsoft.Json.Linq;

namespace SeMaps.VisualStudio;

internal sealed class SeMapsControl : UserControl
{
    private readonly WebView2CompositionControl browser = new WebView2CompositionControl();
    private readonly TextBlock connection = new TextBlock { Text = "●", Margin = new Thickness(5, 0, 4, 0), VerticalAlignment = VerticalAlignment.Center };
    private readonly MenuItem follow = new MenuItem { Header = "Выделять файл по выбору на карте", IsCheckable = true, IsChecked = true };
    private string status = "Откройте карту или переподключитесь.";
    private string bridgeStatus = "Мост: ожидание страницы";
    private string operationStatus = "Перетащите C# файл прямо на холст, затем выберите типы.";
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
    public bool FollowSelection => follow.IsChecked;
    public Task<JObject> RequestAsync(string method, JObject parameters, CancellationToken token) => bridge.RequestAsync(method, parameters, token);
    public void DropStatus(string text)
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        operationStatus = text;
        UpdateDetails();
        (Package.GetGlobalService(typeof(SVsStatusbar)) as IVsStatusbar)?.SetText("SeMaps: " + text.Replace('\n', ' '));
    }

    public SeMapsControl()
    {
        bridge = new IdeBridgeClient(message => browser.CoreWebView2.PostWebMessageAsJson(message.ToString(Newtonsoft.Json.Formatting.None)));
        var root = new DockPanel();
        var header = new StackPanel { Orientation = Orientation.Horizontal, Height = 26 };
        header.SetResourceReference(Panel.BackgroundProperty, VsBrushes.ToolWindowBackgroundKey);
        var retry = CommandButton("↻", "Переподключить карту (Reconnect)");
        retry.Click += (_, __) =>
        {
            ThreadHelper.ThrowIfNotOnUIThread();
            if (Reconnect != null) Reconnect();
            else (Package.GetGlobalService(typeof(EnvDTE.DTE)) as EnvDTE.DTE)?.ExecuteCommand("View.SeMaps");
        };
        var locate = CommandButton("◎", "Выбрать тип из редактора на карте");
        locate.Click += (_, __) => LocateSource?.Invoke();
        var more = CommandButton("⋯", "Настройки и сведения о подключении SeMaps");
        var menu = new ContextMenu();
        var openBrowser = new MenuItem { Header = "Открыть в браузере", IsEnabled = false };
        openBrowser.Click += (_, __) => OpenInBrowser();
        menu.Opened += (_, __) => openBrowser.IsEnabled = endpoint != null;
        var details = new MenuItem { Header = "Сведения о подключении и последнее сообщение…" };
        details.Click += (_, __) => MessageBox.Show(Details(), "SeMaps", MessageBoxButton.OK, MessageBoxImage.Information);
        menu.Items.Add(openBrowser); menu.Items.Add(follow); menu.Items.Add(new Separator()); menu.Items.Add(details);
        more.ContextMenu = menu;
        more.Click += (_, __) => { menu.PlacementTarget = more; menu.Placement = System.Windows.Controls.Primitives.PlacementMode.Bottom; menu.IsOpen = true; };
        header.Children.Add(retry); header.Children.Add(locate); header.Children.Add(more); header.Children.Add(connection);
        DockPanel.SetDock(header, Dock.Top); root.Children.Add(header);
        root.Children.Add(browser); Content = root;
        SetBridgeStatus(bridgeStatus);
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
            catch (Exception ex) { DropStatus("Could not read the drag payload: " + ex.Message); }
            e.Handled = true;
        };
    }

    private static Button CommandButton(string glyph, string label)
    {
        var button = new Button { Content = glyph, ToolTip = label, Width = 26, Height = 22, Margin = new Thickness(1, 2, 1, 2), Padding = new Thickness(0),
            Background = System.Windows.Media.Brushes.Transparent, BorderThickness = new Thickness(0), FontSize = 16 };
        button.SetResourceReference(Control.ForegroundProperty, VsBrushes.WindowTextKey);
        System.Windows.Automation.AutomationProperties.SetName(button, label);
        return button;
    }

    private string Details() => status + "\n" + bridgeStatus + "\n\n" + operationStatus;
    private void OpenInBrowser()
    {
        ThreadHelper.ThrowIfNotOnUIThread();
        if (endpoint == null) return;
        var address = endpoint;
        if (initialized && Uri.TryCreate(browser.CoreWebView2.Source, UriKind.Absolute, out var current)
            && SameOrigin(current, endpoint)) address = current;
        try { System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo(address.AbsoluteUri) { UseShellExecute = true }); }
        catch (Exception ex) { DropStatus("Не удалось открыть карту в браузере: " + ex.Message); }
    }
    private void UpdateDetails() => connection.ToolTip = Details();
    private void SetBridgeStatus(string text, bool connected = false)
    {
        bridgeStatus = text;
        connection.Foreground = connected ? System.Windows.Media.Brushes.SeaGreen : System.Windows.Media.Brushes.DarkGray;
        System.Windows.Automation.AutomationProperties.SetName(connection, text);
        UpdateDetails();
    }
    public void SetStatus(string text) { status = text; UpdateDetails(); }
    public void Disconnect()
    {
        endpoint = null;
        page.Cancel(); page.Dispose(); page = new CancellationTokenSource();
        bridge.Reset();
        SetBridgeStatus("Мост: отключён");
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
                var connected = bridge.Capabilities.Contains("context.get");
                SetBridgeStatus(connected ? "Bridge v1: editor connected" : "Bridge: this editor has no IDE commands. Update SeMaps.", connected);
                ActivityLog.LogInformation("SeMaps", bridgeStatus);
                return;
            }
            catch (OperationCanceledException) when (!token.IsCancellationRequested) { }
            await Task.Delay(250, token);
        }
        SetBridgeStatus("Bridge unavailable. Update SeMaps to a build with IDE integration.");
    }

    private static bool SameOrigin(Uri a, Uri b) => a.Scheme == b.Scheme && a.Host == b.Host && a.Port == b.Port;
}
