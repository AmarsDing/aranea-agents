using System.Runtime.InteropServices;

namespace Aranea.Cua.Win;

/// <summary>组合键（"ctrl+shift+s"）解析为修饰键 + 主键虚拟键码</summary>
public static class ComboKeys
{
    private const ushort VK_CONTROL = 0x11;
    private const ushort VK_MENU = 0x12; // Alt
    private const ushort VK_SHIFT = 0x10;
    private const ushort VK_LWIN = 0x5B;

    private static readonly Dictionary<string, ushort> NamedKeys = new(StringComparer.OrdinalIgnoreCase)
    {
        ["enter"] = 0x0D, ["return"] = 0x0D, ["tab"] = 0x09,
        ["esc"] = 0x1B, ["escape"] = 0x1B,
        ["backspace"] = 0x08, ["delete"] = 0x2E, ["del"] = 0x2E,
        ["insert"] = 0x2D, ["ins"] = 0x2D, ["space"] = 0x20,
        ["up"] = 0x26, ["down"] = 0x28, ["left"] = 0x25, ["right"] = 0x27,
        ["home"] = 0x24, ["end"] = 0x23,
        ["pageup"] = 0x21, ["pgup"] = 0x21, ["pagedown"] = 0x22, ["pgdn"] = 0x22,
        ["f1"] = 0x70, ["f2"] = 0x71, ["f3"] = 0x72, ["f4"] = 0x73, ["f5"] = 0x74, ["f6"] = 0x75,
        ["f7"] = 0x76, ["f8"] = 0x77, ["f9"] = 0x78, ["f10"] = 0x79, ["f11"] = 0x7A, ["f12"] = 0x7B,
    };

    /// <summary>解析组合键；modifiers 为修饰键虚拟键码列表（按下顺序），key 为主键虚拟键码</summary>
    public static bool TryParse(string? combo, out List<ushort> modifiers, out ushort key, out string? error)
    {
        modifiers = new List<ushort>();
        key = 0;
        error = null;
        if (string.IsNullOrWhiteSpace(combo))
        {
            error = "组合键为空";
            return false;
        }
        var parts = combo.Split('+', StringSplitOptions.TrimEntries | StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length == 0)
        {
            error = "组合键为空";
            return false;
        }
        for (var i = 0; i < parts.Length; i++)
        {
            var part = parts[i];
            var isLast = i == parts.Length - 1;
            switch (part.ToLowerInvariant())
            {
                case "ctrl" or "control":
                    modifiers.Add(VK_CONTROL);
                    continue;
                case "alt":
                    modifiers.Add(VK_MENU);
                    continue;
                case "shift":
                    modifiers.Add(VK_SHIFT);
                    continue;
                case "win" or "cmd" or "meta":
                    modifiers.Add(VK_LWIN);
                    continue;
            }
            if (!isLast)
            {
                error = $"非末尾位置出现主键: {part}";
                return false;
            }
            if (NamedKeys.TryGetValue(part, out var vk))
            {
                key = vk;
                return true;
            }
            if (part.Length == 1)
            {
                var c = part[0];
                if (c is >= 'a' and <= 'z') { key = (ushort)(c - 'a' + 'A'); return true; }
                if (c is >= 'A' and <= 'Z') { key = c; return true; }
                if (c is >= '0' and <= '9') { key = c; return true; }
            }
            error = $"无法识别的键: {part}";
            return false;
        }
        // 全部是修饰键（如 "ctrl+shift"）：以最后一个修饰键作为主键
        if (modifiers.Count > 0 && key == 0)
        {
            key = modifiers[^1];
            modifiers.RemoveAt(modifiers.Count - 1);
            return true;
        }
        error = "组合键缺少主键";
        return false;
    }
}

/// <summary>键鼠输入注入（user32 SendInput，物理像素坐标）</summary>
public sealed class InputService
{
    // ---------- Win32 ----------
    private const uint INPUT_MOUSE = 0;
    private const uint INPUT_KEYBOARD = 1;
    private const uint KEYEVENTF_KEYUP = 0x0002;
    private const uint KEYEVENTF_UNICODE = 0x0004;
    private const uint MOUSEEVENTF_LEFTDOWN = 0x0002;
    private const uint MOUSEEVENTF_LEFTUP = 0x0004;
    private const uint MOUSEEVENTF_RIGHTDOWN = 0x0008;
    private const uint MOUSEEVENTF_RIGHTUP = 0x0010;
    private const uint MOUSEEVENTF_MIDDLEDOWN = 0x0020;
    private const uint MOUSEEVENTF_MIDDLEUP = 0x0040;
    private const uint MOUSEEVENTF_WHEEL = 0x0800;

    [StructLayout(LayoutKind.Sequential)]
    private struct MOUSEINPUT
    {
        public int dx;
        public int dy;
        public uint mouseData;
        public uint dwFlags;
        public uint time;
        public UIntPtr dwExtraInfo;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct KEYBDINPUT
    {
        public ushort wVk;
        public ushort wScan;
        public uint dwFlags;
        public uint time;
        public UIntPtr dwExtraInfo;
    }

    [StructLayout(LayoutKind.Explicit)]
    private struct INPUTUNION
    {
        [FieldOffset(0)] public MOUSEINPUT mi;
        [FieldOffset(0)] public KEYBDINPUT ki;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct INPUT
    {
        public uint type;
        public INPUTUNION U;
    }

    [DllImport("user32.dll", SetLastError = true)]
    private static extern uint SendInput(uint nInputs, INPUT[] pInputs, int cbSize);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool SetCursorPos(int x, int y);

    [StructLayout(LayoutKind.Sequential)]
    private struct POINT
    {
        public int X;
        public int Y;
    }

    [DllImport("user32.dll")]
    private static extern IntPtr WindowFromPoint(POINT point);

    [DllImport("user32.dll")]
    private static extern IntPtr GetAncestor(IntPtr hwnd, uint gaFlags);

    [DllImport("user32.dll")]
    private static extern IntPtr GetForegroundWindow();

    [DllImport("user32.dll")]
    private static extern bool SetForegroundWindow(IntPtr hWnd);

    private const uint GA_ROOT = 2;

    /// <summary>
    /// 坐标注入前校验：命中窗口须在前台，否则尝试置前；仍失败则 -32002。
    /// </summary>
    internal static void EnsureForegroundAt(int x, int y)
    {
        var hwnd = WindowFromPoint(new POINT { X = x, Y = y });
        if (hwnd == IntPtr.Zero)
        {
            throw new CuaException(JsonRpc.NotInteractable, $"坐标 ({x},{y}) 未命中窗口");
        }
        var root = GetAncestor(hwnd, GA_ROOT);
        if (root == IntPtr.Zero)
        {
            root = hwnd;
        }
        var fg = GetForegroundWindow();
        if (fg == root)
        {
            return;
        }
        if (!SetForegroundWindow(root) || GetForegroundWindow() != root)
        {
            throw new CuaException(JsonRpc.NotInteractable, "坐标所在窗口未在前台，拒绝注入");
        }
    }

    /// <summary>无前台窗口时拒绝键盘注入（锁屏/安全桌面）。</summary>
    public static bool HasForegroundWindow(IntPtr hwnd) => hwnd != IntPtr.Zero;

    internal static void EnsureHasForeground()
    {
        if (!HasForegroundWindow(GetForegroundWindow()))
        {
            throw new CuaException(JsonRpc.NotInteractable, "当前无前台窗口，拒绝键盘注入");
        }
    }

    // ---------- 公开方法 ----------

    /// <summary>坐标级点击（button: left/right/middle，clickCount 次连击，钳制 [1,10] 防超长阻塞）</summary>
    public object Click(int x, int y, string button, int clickCount)
    {
        EnsureForegroundAt(x, y);
        MoveCursor(x, y);
        var (down, up) = button.ToLowerInvariant() switch
        {
            "right" => (MOUSEEVENTF_RIGHTDOWN, MOUSEEVENTF_RIGHTUP),
            "middle" => (MOUSEEVENTF_MIDDLEDOWN, MOUSEEVENTF_MIDDLEUP),
            _ => (MOUSEEVENTF_LEFTDOWN, MOUSEEVENTF_LEFTUP),
        };
        // 75 复审：clickCount 外部传入，上限钳制防止超大值长时间阻塞请求线程
        var count = Math.Clamp(clickCount, 1, 10);
        for (var i = 0; i < count; i++)
        {
            SendMouse(down, 0);
            SendMouse(up, 0);
            if (i + 1 < count) Thread.Sleep(60);
        }
        return new { ok = true };
    }

    /// <summary>Unicode 文本注入（KEYEVENTF_UNICODE，按字符间隔 intervalMs）</summary>
    public object TypeText(string text, int intervalMs)
    {
        EnsureHasForeground();
        foreach (var ch in text) // char 为单位遍历，代理对由两个 UTF-16 码元分别发送
        {
            var inputs = new[]
            {
                new INPUT { type = INPUT_KEYBOARD, U = new INPUTUNION { ki = new KEYBDINPUT { wVk = 0, wScan = ch, dwFlags = KEYEVENTF_UNICODE } } },
                new INPUT { type = INPUT_KEYBOARD, U = new INPUTUNION { ki = new KEYBDINPUT { wVk = 0, wScan = ch, dwFlags = KEYEVENTF_UNICODE | KEYEVENTF_KEYUP } } },
            };
            Send(inputs);
            if (intervalMs > 0) Thread.Sleep(intervalMs);
        }
        return new { ok = true };
    }

    /// <summary>组合键注入（先按修饰键，点按主键，再逆序释放）</summary>
    public object Key(string combo)
    {
        EnsureHasForeground();
        if (!ComboKeys.TryParse(combo, out var modifiers, out var key, out var error))
        {
            throw new CuaException(JsonRpc.InternalError, $"组合键解析失败: {error}");
        }
        foreach (var mod in modifiers) SendKey(mod, false);
        SendKey(key, false);
        SendKey(key, true);
        for (var i = modifiers.Count - 1; i >= 0; i--) SendKey(modifiers[i], true);
        return new { ok = true };
    }

    /// <summary>滚轮（delta 正上负下，120 为一格）</summary>
    public object Wheel(int x, int y, int delta)
    {
        EnsureForegroundAt(x, y);
        MoveCursor(x, y);
        SendMouse(MOUSEEVENTF_WHEEL, (uint)delta);
        return new { ok = true };
    }

    /// <summary>左键拖拽：平滑路径插值（smoothstep 缓动），总时长 durationMs</summary>
    public object Drag(int x1, int y1, int x2, int y2, int durationMs)
    {
        EnsureForegroundAt(x1, y1);
        MoveCursor(x1, y1);
        Thread.Sleep(30);
        SendMouse(MOUSEEVENTF_LEFTDOWN, 0);
        try
        {
            var steps = Math.Clamp(durationMs / 15, 5, 120);
            for (var i = 1; i <= steps; i++)
            {
                var t = (double)i / steps;
                var e = t * t * (3 - 2 * t); // smoothstep
                var cx = (int)Math.Round(x1 + (x2 - x1) * e);
                var cy = (int)Math.Round(y1 + (y2 - y1) * e);
                MoveCursor(cx, cy);
                Thread.Sleep(Math.Max(1, durationMs / steps));
            }
        }
        finally
        {
            SendMouse(MOUSEEVENTF_LEFTUP, 0);
        }
        return new { ok = true };
    }

    // ---------- 内部 ----------

    /// <summary>移动光标，失败抛注入拒绝</summary>
    private static void MoveCursor(int x, int y)
    {
        if (!SetCursorPos(x, y))
        {
            throw new CuaException(JsonRpc.InjectionDenied, $"SetCursorPos({x},{y}) 被拒绝");
        }
        Thread.Sleep(10);
    }

    /// <summary>发送一次鼠标事件</summary>
    private static void SendMouse(uint flags, uint data)
    {
        Send(new[] { new INPUT { type = INPUT_MOUSE, U = new INPUTUNION { mi = new MOUSEINPUT { dx = 0, dy = 0, mouseData = data, dwFlags = flags } } } });
    }

    /// <summary>发送一次虚拟键按下/释放</summary>
    private static void SendKey(ushort vk, bool keyUp)
    {
        Send(new[] { new INPUT { type = INPUT_KEYBOARD, U = new INPUTUNION { ki = new KEYBDINPUT { wVk = vk, wScan = 0, dwFlags = keyUp ? KEYEVENTF_KEYUP : 0 } } } });
    }

    /// <summary>SendInput 包装：全部注入失败视为 OS 拒绝</summary>
    private static void Send(INPUT[] inputs)
    {
        var sent = SendInput((uint)inputs.Length, inputs, Marshal.SizeOf<INPUT>());
        if (sent == 0)
        {
            throw new CuaException(JsonRpc.InjectionDenied, "SendInput 被 OS 拒绝（可能处于安全桌面或权限不足）");
        }
    }
}
