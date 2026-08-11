using Aranea.Cua.Win;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>组合键解析测试</summary>
public class ComboKeysTests
{
    private const ushort VK_CONTROL = 0x11;
    private const ushort VK_MENU = 0x12;
    private const ushort VK_SHIFT = 0x10;
    private const ushort VK_LWIN = 0x5B;

    [Fact]
    public void Parse_CtrlS()
    {
        Assert.True(ComboKeys.TryParse("ctrl+s", out var mods, out var key, out var err));
        Assert.Null(err);
        Assert.Equal(new List<ushort> { VK_CONTROL }, mods);
        Assert.Equal((ushort)'S', key);
    }

    [Fact]
    public void Parse_CtrlShiftS()
    {
        Assert.True(ComboKeys.TryParse("ctrl+shift+s", out var mods, out var key, out _));
        Assert.Equal(new List<ushort> { VK_CONTROL, VK_SHIFT }, mods);
        Assert.Equal((ushort)'S', key);
    }

    [Fact]
    public void Parse_AltF4()
    {
        Assert.True(ComboKeys.TryParse("alt+f4", out var mods, out var key, out _));
        Assert.Equal(new List<ushort> { VK_MENU }, mods);
        Assert.Equal((ushort)0x73, key);
    }

    [Fact]
    public void Parse_Enter_NoModifiers()
    {
        Assert.True(ComboKeys.TryParse("enter", out var mods, out var key, out _));
        Assert.Empty(mods);
        Assert.Equal((ushort)0x0D, key);
    }

    [Fact]
    public void Parse_WinE_CaseInsensitive()
    {
        Assert.True(ComboKeys.TryParse("WIN+e", out var mods, out var key, out _));
        Assert.Equal(new List<ushort> { VK_LWIN }, mods);
        Assert.Equal((ushort)'E', key);
    }

    [Fact]
    public void Parse_Digit()
    {
        Assert.True(ComboKeys.TryParse("ctrl+5", out _, out var key, out _));
        Assert.Equal((ushort)'5', key);
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("ctrl+")]
    [InlineData("ctrl+unknown")]
    [InlineData("s+ctrl")] // 主键不在末尾
    public void Parse_Invalid_ReturnsFalse(string? combo)
    {
        Assert.False(ComboKeys.TryParse(combo, out _, out _, out var err));
        Assert.NotNull(err);
    }

    [Fact]
    public void Parse_OnlyModifiers_LastBecomesKey()
    {
        Assert.True(ComboKeys.TryParse("ctrl+shift", out var mods, out var key, out _));
        Assert.Equal(new List<ushort> { VK_CONTROL }, mods);
        Assert.Equal(VK_SHIFT, key);
    }
}
