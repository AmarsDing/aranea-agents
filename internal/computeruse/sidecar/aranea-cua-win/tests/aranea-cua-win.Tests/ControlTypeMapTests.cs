using Aranea.Cua.Win;
using FlaUI.Core.Definitions;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>UIA ControlType → CDP type 映射表测试</summary>
public class ControlTypeMapTests
{
    [Theory]
    [InlineData(ControlType.Button, "button")]
    [InlineData(ControlType.Edit, "edit")]
    [InlineData(ControlType.MenuItem, "menuitem")]
    [InlineData(ControlType.Text, "text")]
    [InlineData(ControlType.ComboBox, "combobox")]
    [InlineData(ControlType.CheckBox, "checkbox")]
    [InlineData(ControlType.ListItem, "listitem")]
    [InlineData(ControlType.TabItem, "tabitem")]
    [InlineData(ControlType.Window, "window")]
    public void Map_KnownTypes(ControlType ct, string expected)
    {
        Assert.Equal(expected, ControlTypeMap.Map(ct));
    }

    [Theory]
    [InlineData(ControlType.Image)]
    [InlineData(ControlType.Hyperlink)]
    [InlineData(ControlType.DataGrid)]
    [InlineData(ControlType.Pane)]
    [InlineData(ControlType.Slider)]
    public void Map_UnlistedTypes_FallBackToOther(ControlType ct)
    {
        Assert.Equal("other", ControlTypeMap.Map(ct));
    }
}
