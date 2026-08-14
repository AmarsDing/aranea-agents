using System.Collections.Generic;
using System.ComponentModel;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;

namespace Aranea.Cua.Win;

/// <summary>屏幕信息与截图服务（PerMonitorV2 下全部为物理像素）</summary>
public sealed class CaptureService
{
    [DllImport("user32.dll")]
    private static extern int GetSystemMetrics(int nIndex);

    [DllImport("user32.dll")]
    private static extern uint GetDpiForSystem();

    private delegate bool MonitorEnumProc(IntPtr hMonitor, IntPtr hdcMonitor, ref RECT lprcMonitor, IntPtr dwData);

    [DllImport("user32.dll")]
    private static extern bool EnumDisplayMonitors(IntPtr hdc, IntPtr lprcClip, MonitorEnumProc lpfnEnum, IntPtr dwData);

    [DllImport("shcore.dll")]
    private static extern int GetDpiForMonitor(IntPtr hMonitor, int dpiType, out uint dpiX, out uint dpiY);

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct MONITORINFOEX
    {
        public int cbSize;
        public RECT rcMonitor;
        public RECT rcWork;
        public int dwFlags;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)]
        public string szDevice;
    }

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern bool GetMonitorInfo(IntPtr hMonitor, ref MONITORINFOEX lpmi);

    private const int MONITORINFOF_PRIMARY = 0x00000001;

    /// <summary>截图区域（物理像素）</summary>
    public readonly struct ScreenshotBounds
    {
        public int X { get; }
        public int Y { get; }
        public int W { get; }
        public int H { get; }

        public ScreenshotBounds(int x, int y, int w, int h)
        {
            X = x; Y = y; W = w; H = h;
        }
    }

    private const int SM_CXSCREEN = 0;
    private const int SM_CYSCREEN = 1;
    private const int SM_XVIRTUALSCREEN = 76;
    private const int SM_YVIRTUALSCREEN = 77;
    private const int SM_CXVIRTUALSCREEN = 78;
    private const int SM_CYVIRTUALSCREEN = 79;

    /// <summary>
    /// 解析截图区域：未指定 region 时使用虚拟桌面（含所有显示器）；指定时按物理像素裁剪。
    /// virt* 由调用方注入（测试可固定），生产取 SM_*VIRTUALSCREEN。
    /// </summary>
    public static ScreenshotBounds ResolveScreenshotBounds(int? x, int? y, int? w, int? h, int virtX, int virtY, int virtW, int virtH)
    {
        if (!x.HasValue && !y.HasValue && !w.HasValue && !h.HasValue)
        {
            return new ScreenshotBounds(virtX, virtY, virtW, virtH);
        }
        return new ScreenshotBounds(x ?? virtX, y ?? virtY, w ?? virtW, h ?? virtH);
    }

    /// <summary>
    /// 元素 bbox 并集；无有效框时返回 null（调用方回退虚拟桌面）。
    /// </summary>
    public static ScreenshotBounds? UnionElementBounds(IEnumerable<BBoxDto>? boxes)
    {
        if (boxes == null)
        {
            return null;
        }
        var minX = int.MaxValue;
        var minY = int.MaxValue;
        var maxX = int.MinValue;
        var maxY = int.MinValue;
        var any = false;
        foreach (var b in boxes)
        {
            if (b == null || b.W <= 0 || b.H <= 0)
            {
                continue;
            }
            any = true;
            if (b.X < minX) minX = b.X;
            if (b.Y < minY) minY = b.Y;
            var right = b.X + b.W;
            var bottom = b.Y + b.H;
            if (right > maxX) maxX = right;
            if (bottom > maxY) maxY = bottom;
        }
        if (!any)
        {
            return null;
        }
        return new ScreenshotBounds(minX, minY, maxX - minX, maxY - minY);
    }

    /// <summary>主屏缩放因子（DPI/96）</summary>
    public static double PrimaryScaleFactor() => GetDpiForSystem() / 96.0;

    /// <summary>
    /// 75 review C1：选取截图区域中心点所在显示器的缩放因子；未命中/无显示器信息时回退主屏缩放。纯函数，可单测
    /// </summary>
    public static double PickScaleFactorForRegion(int x, int y, int w, int h, IReadOnlyList<DisplayDto> displays, double fallbackScale)
    {
        var cx = x + w / 2;
        var cy = y + h / 2;
        foreach (var d in displays)
        {
            if (cx >= d.X && cx < d.X + d.W && cy >= d.Y && cy < d.Y + d.H)
            {
                return d.ScaleFactor;
            }
        }
        return fallbackScale;
    }

    /// <summary>枚举全部显示器（物理像素 + 各屏缩放因子）</summary>
    private static List<DisplayDto> EnumDisplays()
    {
        var displays = new List<DisplayDto>();
        EnumDisplayMonitors(IntPtr.Zero, IntPtr.Zero, (IntPtr hMon, IntPtr hdc, ref RECT rc, IntPtr data) =>
        {
            var info = new MONITORINFOEX { cbSize = Marshal.SizeOf<MONITORINFOEX>(), szDevice = "" };
            if (GetMonitorInfo(hMon, ref info))
            {
                double scale = 1.0;
                try
                {
                    if (GetDpiForMonitor(hMon, 0, out var dpiX, out _) == 0 && dpiX > 0)
                    {
                        scale = dpiX / 96.0;
                    }
                }
                catch
                {
                    // shcore 不可用时按 1.0
                }
                displays.Add(new DisplayDto
                {
                    X = info.rcMonitor.Left,
                    Y = info.rcMonitor.Top,
                    W = info.rcMonitor.Right - info.rcMonitor.Left,
                    H = info.rcMonitor.Bottom - info.rcMonitor.Top,
                    ScaleFactor = scale,
                    IsPrimary = (info.dwFlags & MONITORINFOF_PRIMARY) != 0,
                });
            }
            return true;
        }, IntPtr.Zero);
        return displays;
    }

    /// <summary>device.info：平台 + 主屏 + 全部显示器（物理像素）</summary>
    public DeviceInfoResultDto GetDeviceInfo()
    {
        return new DeviceInfoResultDto
        {
            Platform = "windows",
            Screen = new ScreenDto
            {
                Width = GetSystemMetrics(SM_CXSCREEN),
                Height = GetSystemMetrics(SM_CYSCREEN),
                ScaleFactor = PrimaryScaleFactor(),
            },
            VirtualScreen = new ScreenDto
            {
                X = GetSystemMetrics(SM_XVIRTUALSCREEN),
                Y = GetSystemMetrics(SM_YVIRTUALSCREEN),
                Width = GetSystemMetrics(SM_CXVIRTUALSCREEN),
                Height = GetSystemMetrics(SM_CYVIRTUALSCREEN),
                ScaleFactor = PrimaryScaleFactor(),
            },
            Displays = EnumDisplays(),
        };
    }

    /// <summary>截图（默认虚拟桌面全屏；region 指定时裁剪；zoom≠1 时缩放位图），返回 PNG base64</summary>
    public ScreenshotResultDto Screenshot(int? x, int? y, int? w, int? h, double zoom)
    {
        var bounds = ResolveScreenshotBounds(
            x, y, w, h,
            GetSystemMetrics(SM_XVIRTUALSCREEN),
            GetSystemMetrics(SM_YVIRTUALSCREEN),
            GetSystemMetrics(SM_CXVIRTUALSCREEN),
            GetSystemMetrics(SM_CYVIRTUALSCREEN));
        var sx = bounds.X;
        var sy = bounds.Y;
        var sw = bounds.W;
        var sh = bounds.H;
        if (sw <= 0 || sh <= 0)
        {
            throw new CuaException(JsonRpc.InternalError, $"非法截图区域: {sw}x{sh}");
        }
        if (zoom <= 0)
        {
            zoom = 1.0;
        }
        try
        {
            using var bmp = new Bitmap(sw, sh, PixelFormat.Format32bppArgb);
            using (var g = Graphics.FromImage(bmp))
            {
                g.CopyFromScreen(sx, sy, 0, 0, new Size(sw, sh), CopyPixelOperation.SourceCopy);
            }
            // F1：zoom≠1 时缩放（dense UI 局部放大提升 VLM 精度）；返回尺寸为缩放后实际像素
            using var scaled = ScaleBitmap(bmp, zoom);
            using var ms = new MemoryStream();
            scaled.Save(ms, ImageFormat.Png);
            return new ScreenshotResultDto
            {
                PngBase64 = Convert.ToBase64String(ms.ToArray()),
                Width = scaled.Width,
                Height = scaled.Height,
                // C1 修复：按截图区域中心点匹配显示器缩放（多屏混合 DPI 时不再恒定主屏）
                ScaleFactor = PickScaleFactorForRegion(sx, sy, sw, sh, EnumDisplays(), PrimaryScaleFactor()),
            };
        }
        catch (Win32Exception wex)
        {
            // 安全桌面/锁屏等场景 CopyFromScreen 会失败
            throw new CuaException(JsonRpc.InjectionDenied, "截图被 OS 拒绝: " + wex.Message);
        }
    }

    /// <summary>zoom 上限：防止超大 zoom 触发巨额位图分配（OOM/DoS，75 review B3）；局部放大场景 10x 足够</summary>
    private const double MaxZoom = 10.0;

    /// <summary>按倍率缩放位图（zoom=1 原样复制；结果至少 1x1；zoom 超过 MaxZoom 截断）。纯位图操作，可单测</summary>
    public static Bitmap ScaleBitmap(Bitmap src, double zoom)
    {
        if (zoom <= 0)
        {
            zoom = 1.0;
        }
        if (zoom > MaxZoom)
        {
            zoom = MaxZoom;
        }
        var zw = Math.Max(1, (int)Math.Round(src.Width * zoom));
        var zh = Math.Max(1, (int)Math.Round(src.Height * zoom));
        var dst = new Bitmap(zw, zh, PixelFormat.Format32bppArgb);
        using var g = Graphics.FromImage(dst);
        g.InterpolationMode = System.Drawing.Drawing2D.InterpolationMode.HighQualityBicubic;
        g.DrawImage(src, 0, 0, zw, zh);
        return dst;
    }
}
