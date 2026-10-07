using System.Collections.Generic;
using System.Linq;
using System.Windows;
using System.Windows.Controls;

namespace SeMaps.VisualStudio;

internal static class SymbolPicker
{
    public static IReadOnlyList<SourceSymbol>? Choose(IReadOnlyList<SourceSymbol> candidates)
    {
        var list = new ListBox { SelectionMode = SelectionMode.Extended, ItemsSource = candidates, Margin = new Thickness(12) };
        var accept = new Button { Content = "Найти на карте / добавить выбранные", Margin = new Thickness(12), IsDefault = true, IsEnabled = false };
        var hint = new TextBlock { Text = "Найдены типы. Выберите нужные (Ctrl/Shift — несколько). Извлечение и сверка автоматически не выполняются.", TextWrapping = TextWrapping.Wrap, Margin = new Thickness(12) };
        var panel = new DockPanel();
        DockPanel.SetDock(hint, Dock.Top); panel.Children.Add(hint);
        var cancel = new Button { Content = "Отмена", IsCancel = true, Margin = new Thickness(12), MinWidth = 100 };
        var footer = new DockPanel(); DockPanel.SetDock(cancel, Dock.Right); footer.Children.Add(cancel); footer.Children.Add(accept);
        DockPanel.SetDock(footer, Dock.Bottom); panel.Children.Add(footer); panel.Children.Add(list);
        var dialog = new Window { Title = "SeMaps — что добавить из исходников?", Content = panel, Width = 900, Height = 380,
            WindowStartupLocation = WindowStartupLocation.CenterOwner, Owner = Application.Current.MainWindow };
        list.SelectionChanged += (_, __) => accept.IsEnabled = list.SelectedItems.Count > 0;
        accept.Click += (_, __) => dialog.DialogResult = true;
        return dialog.ShowDialog() == true ? list.SelectedItems.Cast<SourceSymbol>().ToArray() : null;
    }
}
