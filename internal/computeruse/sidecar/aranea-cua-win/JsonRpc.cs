using System.Text.Json;

namespace Aranea.Cua.Win;

/// <summary>带 CDP 错误码的业务异常</summary>
public sealed class CuaException : Exception
{
    public int Code { get; }

    public CuaException(int code, string message) : base(message)
    {
        Code = code;
    }
}

/// <summary>JSON-RPC 2.0 帧解析、响应包装与错误码常量</summary>
public static class JsonRpc
{
    /// <summary>元素未找到 / ref 过期</summary>
    public const int ElementNotFound = -32001;
    /// <summary>窗口失焦 / 元素不可交互</summary>
    public const int NotInteractable = -32002;
    /// <summary>OS 级注入被拒绝</summary>
    public const int InjectionDenied = -32003;
    /// <summary>sidecar 内部错误</summary>
    public const int InternalError = -32004;
    /// <summary>方法不存在</summary>
    public const int MethodNotFound = -32601;
    /// <summary>JSON 解析失败</summary>
    public const int ParseError = -32700;

    /// <summary>一个已解析的 JSON-RPC 请求（持有 JsonDocument 生命周期）</summary>
    public sealed class Request : IDisposable
    {
        public string IdRaw { get; set; } = "null";
        public string Method { get; set; } = "";
        public JsonElement? Params { get; set; }
        internal JsonDocument? Doc { get; set; }

        public void Dispose() => Doc?.Dispose();
    }

    /// <summary>解析一行请求帧；失败时 errorResponse 为可直接写出的错误帧</summary>
    public static bool TryParseRequest(string line, out Request? request, out string? errorResponse)
    {
        request = null;
        errorResponse = null;
        JsonDocument doc;
        try
        {
            doc = JsonDocument.Parse(line);
        }
        catch (JsonException ex)
        {
            errorResponse = ErrorResponse(null, ParseError, "JSON 解析失败: " + ex.Message);
            return false;
        }
        var root = doc.RootElement;
        if (root.ValueKind != JsonValueKind.Object)
        {
            doc.Dispose();
            errorResponse = ErrorResponse(null, ParseError, "帧必须是 JSON 对象");
            return false;
        }
        var idRaw = "null";
        if (root.TryGetProperty("id", out var idEl) &&
            idEl.ValueKind != JsonValueKind.Null && idEl.ValueKind != JsonValueKind.Undefined)
        {
            idRaw = idEl.GetRawText();
        }
        if (!root.TryGetProperty("method", out var mEl) || mEl.ValueKind != JsonValueKind.String)
        {
            doc.Dispose();
            errorResponse = ErrorResponse(idRaw, MethodNotFound, "缺少 method 字段");
            return false;
        }
        JsonElement? prms = root.TryGetProperty("params", out var pEl) ? pEl : null;
        request = new Request { IdRaw = idRaw, Method = mEl.GetString()!, Params = prms, Doc = doc };
        return true;
    }

    /// <summary>包装成功响应帧</summary>
    public static string ResultResponse(string idRaw, object result)
    {
        var resultJson = JsonSerializer.Serialize(result, JsonOut.Options);
        return $"{{\"jsonrpc\":\"2.0\",\"id\":{idRaw},\"result\":{resultJson}}}";
    }

    /// <summary>包装错误响应帧</summary>
    public static string ErrorResponse(string? idRaw, int code, string message)
    {
        var msgJson = JsonSerializer.Serialize(message, JsonOut.Options);
        return $"{{\"jsonrpc\":\"2.0\",\"id\":{idRaw ?? "null"},\"error\":{{\"code\":{code},\"message\":{msgJson}}}}}";
    }

    /// <summary>已注册方法清单</summary>
    private static readonly HashSet<string> KnownMethods = new()
    {
        "device.ping", "device.info",
        "perception.snapshot", "perception.screenshot",
        "action.invoke", "action.click", "action.type", "action.key", "action.wheel", "action.drag",
        "window.list", "window.focus", "app.launch",
    };

    /// <summary>判断方法名是否已注册</summary>
    public static bool IsKnownMethod(string method) => KnownMethods.Contains(method);
}

/// <summary>请求调度器：按 method 分发到各服务，统一异常→错误帧</summary>
public sealed class Dispatcher
{
    private readonly UiaService _uia;
    private readonly InputService _input;
    private readonly CaptureService _capture;

    public Dispatcher(UiaService uia, InputService input, CaptureService capture)
    {
        _uia = uia;
        _input = input;
        _capture = capture;
    }

    /// <summary>处理一行请求，返回一行响应 JSON（保证单请求异常不崩进程）</summary>
    public string HandleLine(string line)
    {
        if (!JsonRpc.TryParseRequest(line, out var req, out var err))
        {
            return err!;
        }
        using (req!)
        {
            try
            {
                var result = Dispatch(req);
                return JsonRpc.ResultResponse(req.IdRaw, result);
            }
            catch (CuaException ce)
            {
                return JsonRpc.ErrorResponse(req.IdRaw, ce.Code, ce.Message);
            }
            catch (Exception ex)
            {
                Program.Diag($"请求 {req.Method} 未捕获异常: {ex}");
                return JsonRpc.ErrorResponse(req.IdRaw, JsonRpc.InternalError, "内部错误: " + ex.Message);
            }
        }
    }

    /// <summary>按方法名分发执行</summary>
    private object Dispatch(JsonRpc.Request req)
    {
        var p = req.Params;
        switch (req.Method)
        {
            case "device.ping":
                return new { ok = true };
            case "device.info":
                return _capture.GetDeviceInfo();
            case "perception.snapshot":
                return _uia.Snapshot(GetString(p, "windowTitle"), GetInt(p, "maxElements", 500));
            case "perception.screenshot":
                return Screenshot(p);
            case "action.invoke":
                return _uia.Invoke(RequireString(p, "ref"));
            case "action.click":
                return _input.Click(RequireInt(p, "x"), RequireInt(p, "y"),
                    GetString(p, "button") ?? "left", GetInt(p, "clickCount", 1));
            case "action.type":
                return _input.TypeText(RequireString(p, "text"), GetInt(p, "intervalMs", 10));
            case "action.key":
                return _input.Key(RequireString(p, "combo"));
            case "action.wheel":
                return _input.Wheel(RequireInt(p, "x"), RequireInt(p, "y"), RequireInt(p, "delta"));
            case "action.drag":
                return Drag(p);
            case "window.list":
                return _uia.ListWindows();
            case "window.focus":
                return _uia.FocusWindow(GetString(p, "titleRegex"), GetLong(p, "hwnd"));
            case "app.launch":
                return _uia.Launch(RequireString(p, "target"), GetString(p, "args"), GetString(p, "workDir"));
            default:
                throw new CuaException(JsonRpc.MethodNotFound, $"方法不存在: {req.Method}");
        }
    }

    /// <summary>解析截图参数并执行</summary>
    private object Screenshot(JsonElement? p)
    {
        int? x = null, y = null, w = null, h = null;
        if (TryGet(p, "region", out var region) && region.ValueKind == JsonValueKind.Object)
        {
            x = GetInt(region, "x", 0);
            y = GetInt(region, "y", 0);
            w = RequireInt(region, "w");
            h = RequireInt(region, "h");
        }
        return _capture.Screenshot(x, y, w, h);
    }

    /// <summary>解析拖拽参数并执行</summary>
    private object Drag(JsonElement? p)
    {
        if (!TryGet(p, "from", out var from) || !TryGet(p, "to", out var to))
        {
            throw new CuaException(JsonRpc.InternalError, "action.drag 需要 from/to 参数");
        }
        return _input.Drag(
            RequireInt(from, "x"), RequireInt(from, "y"),
            RequireInt(to, "x"), RequireInt(to, "y"),
            GetInt(p, "durationMs", 300));
    }

    /// <summary>取参数属性</summary>
    internal static bool TryGet(JsonElement? p, string name, out JsonElement value)
    {
        value = default;
        return p.HasValue && p.Value.ValueKind == JsonValueKind.Object && p.Value.TryGetProperty(name, out value);
    }

    /// <summary>取可选字符串参数</summary>
    internal static string? GetString(JsonElement? p, string name)
    {
        return TryGet(p, name, out var v) && v.ValueKind == JsonValueKind.String ? v.GetString() : null;
    }

    /// <summary>取必选字符串参数</summary>
    internal static string RequireString(JsonElement? p, string name)
    {
        return GetString(p, name) ?? throw new CuaException(JsonRpc.InternalError, $"缺少参数: {name}");
    }

    /// <summary>取带默认值整型参数</summary>
    internal static int GetInt(JsonElement? p, string name, int def)
    {
        return TryGet(p, name, out var v) && v.ValueKind == JsonValueKind.Number && v.TryGetInt32(out var i) ? i : def;
    }

    /// <summary>取必选整型参数</summary>
    internal static int RequireInt(JsonElement? p, string name)
    {
        if (TryGet(p, name, out var v) && v.ValueKind == JsonValueKind.Number && v.TryGetInt32(out var i)) return i;
        throw new CuaException(JsonRpc.InternalError, $"缺少参数: {name}");
    }

    /// <summary>取可选长整型参数</summary>
    internal static long? GetLong(JsonElement? p, string name)
    {
        return TryGet(p, name, out var v) && v.ValueKind == JsonValueKind.Number && v.TryGetInt64(out var l) ? l : null;
    }

    /// <summary>重载：直接对 JsonElement 取整</summary>
    internal static int GetInt(JsonElement p, string name, int def) => GetInt((JsonElement?)p, name, def);

    /// <summary>重载：直接对 JsonElement 取必选整型</summary>
    internal static int RequireInt(JsonElement p, string name) => RequireInt((JsonElement?)p, name);
}
