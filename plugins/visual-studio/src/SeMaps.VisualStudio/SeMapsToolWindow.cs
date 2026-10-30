using System;
using System.Runtime.InteropServices;
using Microsoft.VisualStudio.Shell;

namespace SeMaps.VisualStudio;

[Guid("f3fa32c7-8d49-4f5c-a65b-73f97fced688")]
public sealed class SeMapsToolWindow : ToolWindowPane
{
    public SeMapsToolWindow() : base(null)
    {
        Caption = "SeMaps";
        Content = new SeMapsControl();
    }
}
