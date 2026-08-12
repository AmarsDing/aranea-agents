using System.Drawing;
using Aranea.Cua.Win;
using Xunit;

namespace Aranea.Cua.Win.Tests;

/// <summary>截图缩放逻辑测试（纯位图操作，无桌面依赖）——75 review F1：zoom 参数不得静默忽略</summary>
public class CaptureServiceTests
{
    [Fact]
    public void ScaleBitmap_Zoom2_UpscalesDimensions()
    {
        using var src = new Bitmap(10, 6);
        using var dst = CaptureService.ScaleBitmap(src, 2.0);
        Assert.Equal(20, dst.Width);
        Assert.Equal(12, dst.Height);
    }

    [Fact]
    public void ScaleBitmap_ZoomHalf_DownscalesDimensions()
    {
        using var src = new Bitmap(10, 6);
        using var dst = CaptureService.ScaleBitmap(src, 0.5);
        Assert.Equal(5, dst.Width);
        Assert.Equal(3, dst.Height);
    }

    [Fact]
    public void ScaleBitmap_Zoom1_KeepsDimensions()
    {
        using var src = new Bitmap(10, 6);
        using var dst = CaptureService.ScaleBitmap(src, 1.0);
        Assert.Equal(10, dst.Width);
        Assert.Equal(6, dst.Height);
    }

    [Fact]
    public void ScaleBitmap_TinySource_ClampToAtLeast1px()
    {
        using var src = new Bitmap(1, 1);
        using var dst = CaptureService.ScaleBitmap(src, 0.1);
        Assert.True(dst.Width >= 1);
        Assert.True(dst.Height >= 1);
    }

    [Fact]
    public void ScaleBitmap_Zoom2_PreservesPixelContent()
    {
        using var src = new Bitmap(2, 1);
        src.SetPixel(0, 0, Color.Red);
        src.SetPixel(1, 0, Color.Blue);
        using var dst = CaptureService.ScaleBitmap(src, 2.0);
        var left = dst.GetPixel(0, 0);
        var right = dst.GetPixel(3, 0);
        Assert.True(left.R > 200 && left.B < 100, $"left={left}");
        Assert.True(right.B > 200 && right.R < 100, $"right={right}");
    }
}
