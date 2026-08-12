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

    /// <summary>主屏缩放因子（DPI/96）</summary>
    public static double PrimaryScaleFactor() => GetDpiForSystem() / 96.0;

    /// <summary>device.info：平台 + 主屏 + 全部显示器（物理像素）</summary>
    public DeviceInfoResultDto GetDeviceInfo()
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

        return new DeviceInfoResultDto
        {
            Platform = "windows",
            Screen = new ScreenDto
            {
                Width = GetSystemMetrics(0),  // SM_CXSCREEN
                Height = GetSystemMetrics(1), // SM_CYSCREEN
                ScaleFactor = PrimaryScaleFactor(),
            },
            Displays = displays,
        };
    }

    /// <summary>截图（默认主屏全屏；region 指定时裁剪；zoom≠1 时缩放位图），返回 PNG base64</summary>
    public ScreenshotResultDto Screenshot(int? x, int? y, int? w, int? h, double zoom)
    {
        var sx = x ?? 0;
        var sy = y ?? 0;
        var sw = w ?? GetSystemMetrics(0);
        var sh = h ?? GetSystemMetrics(1);
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
                ScaleFactor = PrimaryScaleFactor(),
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
