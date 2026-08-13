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
    public void ScaleBitmap_HugeZoom_ClampedToMax()
    {
        // B3：zoom 无上界时攻击者可传入极大值触发巨额位图分配（OOM/DoS）
        using var src = new Bitmap(10, 6);
        using var dst = CaptureService.ScaleBitmap(src, 1000.0);
        Assert.Equal(100, dst.Width);  // MaxZoom=10 → 10x
        Assert.Equal(60, dst.Height);
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

    // 75 review C1：截图 ScaleFactor 必须反映被截显示器的 DPI，而非恒定主屏 DPI

    [Fact]
    public void PickScaleFactor_PrimaryRegion_ReturnsPrimaryScale()
    {
        var displays = new List<DisplayDto>
        {
            new() { X = 0, Y = 0, W = 1920, H = 1080, ScaleFactor = 1.0, IsPrimary = true },
            new() { X = 1920, Y = 0, W = 2560, H = 1440, ScaleFactor = 1.5, IsPrimary = false },
        };
        Assert.Equal(1.0, CaptureService.PickScaleFactorForRegion(0, 0, 1920, 1080, displays, 1.0));
    }

    [Fact]
    public void PickScaleFactor_SecondaryRegion_ReturnsSecondaryScale()
    {
        // 核心 bug 场景：截副屏（DPI 150%）但 ScaleFactor 曾恒为主屏 1.0
        var displays = new List<DisplayDto>
        {
            new() { X = 0, Y = 0, W = 1920, H = 1080, ScaleFactor = 1.0, IsPrimary = true },
            new() { X = 1920, Y = 0, W = 2560, H = 1440, ScaleFactor = 1.5, IsPrimary = false },
        };
        Assert.Equal(1.5, CaptureService.PickScaleFactorForRegion(1920, 0, 2560, 1440, displays, 1.0));
    }

    [Fact]
    public void PickScaleFactor_NoHit_FallsBackPrimary()
    {
        var displays = new List<DisplayDto>
        {
            new() { X = 0, Y = 0, W = 1920, H = 1080, ScaleFactor = 1.0, IsPrimary = true },
        };
        // 区域中心在虚拟桌面外（如负坐标盲区）→ 回退主屏缩放
        Assert.Equal(1.25, CaptureService.PickScaleFactorForRegion(-5000, -5000, 100, 100, displays, 1.25));
    }

    [Fact]
    public void PickScaleFactor_EmptyDisplays_FallsBack()
    {
        Assert.Equal(1.25, CaptureService.PickScaleFactorForRegion(0, 0, 100, 100, new List<DisplayDto>(), 1.25));
    }
}
