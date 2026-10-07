using System.Collections.Generic;
using System.Windows;
using System.Windows.Controls;

namespace SeMaps.VisualStudio;

internal static class ProjectPicker
{
    public static string? Choose(IReadOnlyList<string> files)
    {
        var list = new ListBox { ItemsSource = files, Margin = new Thickness(12) };
        var accept = new Button { Content = "Open", Margin = new Thickness(12), IsDefault = true, IsEnabled = false };
        var panel = new DockPanel();
        DockPanel.SetDock(accept, Dock.Bottom);
        panel.Children.Add(accept); panel.Children.Add(list);
        var dialog = new Window
        {
            Title = "Select a SeMaps project", Content = panel, Width = 620, Height = 280,
            WindowStartupLocation = WindowStartupLocation.CenterOwner,
            Owner = Application.Current.MainWindow,
        };
        list.SelectionChanged += (_, __) => accept.IsEnabled = list.SelectedItem != null;
        accept.Click += (_, __) => dialog.DialogResult = true;
        return dialog.ShowDialog() == true ? list.SelectedItem as string : null;
    }
}
