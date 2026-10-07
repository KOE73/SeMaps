using System;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using Microsoft.VisualStudio.Shell;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace SeMaps.VisualStudio;

internal sealed class SeMapsControl : UserControl
{
    private readonly WebView2CompositionControl browser = new WebView2CompositionControl();
    private readonly TextBlock status = new TextBlock { Margin = new Thickness(6), TextWrapping = TextWrapping.Wrap };
    private readonly TextBlock probeStatus = new TextBlock { Margin = new Thickness(6), Text = "Bridge: waiting for page" };
    private readonly Border dropTarget = new Border
    {
        AllowDrop = true, Padding = new Thickness(8), Background = System.Windows.Media.Brushes.AliceBlue,
        Child = new TextBlock { Text = "Drop a source file here to inspect the Visual Studio drag payload (prototype).", TextWrapping = TextWrapping.Wrap },
    };
    private string session = Guid.NewGuid().ToString("N");
    private string? pendingRequestId;
    private Uri? endpoint;
    private bool initialized;
    public Action? Reconnect { get; set; }

    public SeMapsControl()
    {
        status.SetResourceReference(TextBlock.ForegroundProperty, VsBrushes.WindowTextKey);
        probeStatus.SetResourceReference(TextBlock.ForegroundProperty, VsBrushes.WindowTextKey);
        var root = new DockPanel();
        var header = new StackPanel();
        var retry = new Button { Content = "Reconnect", HorizontalAlignment = HorizontalAlignment.Left, Margin = new Thickness(6) };
        retry.Click += (_, __) => Reconnect?.Invoke();
        header.Children.Add(status); header.Children.Add(retry); header.Children.Add(probeStatus);
        DockPanel.SetDock(header, Dock.Top); root.Children.Add(header);
        DockPanel.SetDock(dropTarget, Dock.Bottom); root.Children.Add(dropTarget);
        root.Children.Add(browser); Content = root;
        dropTarget.DragOver += (_, e) =>
        {
            e.Effects = e.Data.GetDataPresent(DataFormats.FileDrop) || e.Data.GetDataPresent("CF_VSSTGPROJECTITEMS")
                || e.Data.GetDataPresent("CF_VSREFPROJECTITEMS") ? DragDropEffects.Copy : DragDropEffects.None;
            e.Handled = true;
        };
        dropTarget.Drop += (_, e) =>
        {
            try
            {
                var formats = e.Data.GetFormats();
                var files = e.Data.GetData(DataFormats.FileDrop) as string[];
                var text = "Formats: " + string.Join(", ", formats);
                if (files != null) text += "\nFiles: " + string.Join("\n", files);
                // Native formats never reach the web page. This probe only establishes what VS supplies.
                ((TextBlock)dropTarget.Child).Text = text;
                ActivityLog.LogInformation("SeMaps", "Native drag received: " + text);
            }
            catch (Exception ex) { ((TextBlock)dropTarget.Child).Text = "Could not read the drag payload: " + ex.Message; }
            e.Handled = true;
        };
    }

    public void SetStatus(string text) => status.Text = text;
    public void Disconnect()
    {
        endpoint = null;
        pendingRequestId = null;
        probeStatus.Text = "Bridge: disconnected";
        if (initialized) browser.CoreWebView2.Navigate("about:blank");
    }

    public async Task ConnectAsync(Uri address, CancellationToken cancellationToken)
    {
        await ThreadHelper.JoinableTaskFactory.SwitchToMainThreadAsync(cancellationToken);
        endpoint = address;
        session = Guid.NewGuid().ToString("N");
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
            browser.CoreWebView2.WebMessageReceived += (_, e) => Receive(e);
            browser.CoreWebView2.NavigationCompleted += OnNavigationCompleted;
            initialized = true;
        }
        browser.CoreWebView2.Navigate(address.AbsoluteUri);
    }

    // WebView2 exposes a void event handler; catch all failures at the event boundary.
#pragma warning disable VSTHRD100
    private async void OnNavigationCompleted(object? sender, CoreWebView2NavigationCompletedEventArgs e)
    {
        try { if (endpoint != null && e.IsSuccess) await ProbeAsync(); }
        catch (Exception ex) { ActivityLog.LogError("SeMaps", ex.ToString()); }
    }
#pragma warning restore VSTHRD100

    private async Task ProbeAsync()
    {
        await ThreadHelper.JoinableTaskFactory.SwitchToMainThreadAsync();
        try
        {
            // A transport-only probe; no DOM inspection or model changes.
            using (var stream = typeof(SeMapsControl).Assembly.GetManifestResourceStream("SeMaps.VisualStudio.BridgeProbe.js"))
            using (var reader = new StreamReader(stream!))
                await browser.CoreWebView2.ExecuteScriptAsync(await reader.ReadToEndAsync());
            if (endpoint == null) return;
            pendingRequestId = Guid.NewGuid().ToString("N");
            browser.CoreWebView2.PostWebMessageAsJson(JsonConvert.SerializeObject(new
            {
                protocol = "semaps.ide", version = 1, sessionId = session, id = pendingRequestId,
                kind = "request", method = "bridge.initialize", @params = new { },
            }));
        }
        catch (Exception ex) { probeStatus.Text = "Bridge: " + ex.Message; }
    }

    private void Receive(CoreWebView2WebMessageReceivedEventArgs args)
    {
        if (endpoint == null || !Uri.TryCreate(args.Source, UriKind.Absolute, out var source) || !SameOrigin(source, endpoint)) return;
        try
        {
            var message = JObject.Parse(args.WebMessageAsJson);
            if ((string?)message["protocol"] != "semaps.ide" || (int?)message["version"] != 1
                || (string?)message["sessionId"] != session || (string?)message["kind"] != "response"
                || pendingRequestId == null || (string?)message["requestId"] != pendingRequestId) return;
            pendingRequestId = null;
            if (message["error"] != null || (bool?)message["result"]?["probe"] != true)
            {
                probeStatus.Text = "Bridge: transport probe rejected";
                return;
            }
            probeStatus.Text = "Bridge v1: connected (transport probe; editor commands pending)";
            ActivityLog.LogInformation("SeMaps", "Bridge v1 transport round-trip succeeded.");
        }
        catch (JsonException ex) { probeStatus.Text = "Bridge: invalid response: " + ex.Message; }
    }

    private static bool SameOrigin(Uri a, Uri b) => a.Scheme == b.Scheme && a.Host == b.Host && a.Port == b.Port;
}
