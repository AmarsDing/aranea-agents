using System.Runtime.InteropServices;
using System.Text.Encodings.Web;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Aranea.Cua.Win;

/// <summary>统一 JSON 序列化选项（camelCase，中文不转义，null 忽略）</summary>
public static class JsonOut
{
    public static readonly JsonSerializerOptions Options = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DictionaryKeyPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };
}

/// <summary>矩形（物理像素）</summary>
public sealed class BBoxDto
{
    public int X { get; set; }
    public int Y { get; set; }
    public int W { get; set; }
    public int H { get; set; }
}

/// <summary>统一元素模型（CDP §2.3）</summary>
public sealed class UIElementDto
{
    public string Ref { get; set; } = "";
    public string Type { get; set; } = "other";
    public string Name { get; set; } = "";
    public BBoxDto Bbox { get; set; } = new();
    public bool Interactivity { get; set; }
    public string Source { get; set; } = "uia";
    public string AppName { get; set; } = "";
    public bool Enabled { get; set; }
}

/// <summary>perception.snapshot 返回</summary>
public sealed class SnapshotResultDto
{
    public List<UIElementDto> Elements { get; set; } = new();
    public int Generation { get; set; }
}

/// <summary>perception.screenshot 返回</summary>
public sealed class ScreenshotResultDto
{
    public string PngBase64 { get; set; } = "";
    public int Width { get; set; }
    public int Height { get; set; }
    public double ScaleFactor { get; set; }
}

/// <summary>单显示器信息（物理像素）</summary>
public sealed class DisplayDto
{
    public int X { get; set; }
    public int Y { get; set; }
    public int W { get; set; }
    public int H { get; set; }
    public double ScaleFactor { get; set; }
    public bool IsPrimary { get; set; }
}

/// <summary>主屏信息</summary>
public sealed class ScreenDto
{
    public int Width { get; set; }
    public int Height { get; set; }
    public double ScaleFactor { get; set; }
}

/// <summary>device.info 返回</summary>
public sealed class DeviceInfoResultDto
{
    public string Platform { get; set; } = "windows";
    public ScreenDto Screen { get; set; } = new();
    public List<DisplayDto> Displays { get; set; } = new();
}

/// <summary>窗口列表项</summary>
public sealed class WindowDto
{
    public long Hwnd { get; set; }
    public string Title { get; set; } = "";
    public string ProcessName { get; set; } = "";
    public bool IsForeground { get; set; }
    public BBoxDto Bounds { get; set; } = new();
}

/// <summary>window.list 返回</summary>
public sealed class WindowListResultDto
{
    public List<WindowDto> Windows { get; set; } = new();
}

/// <summary>Win32 RECT（跨服务共享）</summary>
[StructLayout(LayoutKind.Sequential)]
public struct RECT
{
    public int Left;
    public int Top;
    public int Right;
    public int Bottom;
}

/// <summary>ref（g{generation}.e{index}）解析与格式化</summary>
public static class RefParser
{
    /// <summary>解析 ref 文本为 (generation, index)，非法格式返回 false</summary>
    public static bool TryParse(string? refText, out int generation, out int index)
    {
        generation = 0;
        index = 0;
        if (string.IsNullOrEmpty(refText)) return false;
        // 形如 g12.e42
        if (refText[0] != 'g') return false;
        var dot = refText.IndexOf('.');
        if (dot <= 1 || dot == refText.Length - 1) return false;
        if (refText[dot + 1] != 'e') return false;
        if (!int.TryParse(refText.AsSpan(1, dot - 1), out generation)) return false;
        if (!int.TryParse(refText.AsSpan(dot + 2), out index)) return false;
        return generation >= 0 && index >= 0;
    }

    /// <summary>生成 ref 文本</summary>
    public static string Format(int generation, int index) => $"g{generation}.e{index}";
}
