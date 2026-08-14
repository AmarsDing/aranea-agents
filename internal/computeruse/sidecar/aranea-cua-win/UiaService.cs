using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.RegularExpressions;
using FlaUI.Core.AutomationElements;
using FlaUI.Core.Definitions;
using FlaUI.UIA3;

namespace Aranea.Cua.Win;

/// <summary>UIA ControlType → CDP type 字符串映射（契约 §2.3）</summary>
public static class ControlTypeMap
{
    /// <summary>映射 ControlType 到契约小写类型串，未列举的一律 other</summary>
    public static string Map(ControlType ct) => ct switch
    {
        ControlType.Button => "button",
        ControlType.Edit => "edit",
        ControlType.MenuItem => "menuitem",
        ControlType.Text => "text",
        ControlType.ComboBox => "combobox",
        ControlType.CheckBox => "checkbox",
        ControlType.ListItem => "listitem",
        ControlType.TabItem => "tabitem",
        ControlType.Window => "window",
        _ => "other",
    };
}

/// <summary>纯逻辑树遍历器：先序 DFS，深度与数量双重截断，select 返回 null 表示跳过</summary>
public static class TreeWalker
{
    /// <summary>遍历并收集：maxDepth 限制下钻层数（根为 0），maxElements 限制收集条数</summary>
    public static List<TOut> Collect<TNode, TOut>(
        TNode root,
        Func<TNode, IEnumerable<TNode>> children,
        Func<TNode, TOut?> select,
        int maxDepth,
        int maxElements) where TOut : class
    {
        var result = new List<TOut>();
        if (maxElements <= 0) return result;
        var stack = new Stack<(TNode Node, int Depth)>();
        stack.Push((root, 0));
        while (stack.Count > 0)
        {
            var (node, depth) = stack.Pop();
            var item = select(node);
            if (item != null)
            {
                result.Add(item);
                if (result.Count >= maxElements) return result; // 截断
            }
            if (depth >= maxDepth) continue;
            List<TNode> kids;
            try
            {
                kids = children(node).ToList();
            }
            catch
            {
                continue; // 子树枚举失败不炸整树
            }
            for (var i = kids.Count - 1; i >= 0; i--)
            {
                stack.Push((kids[i], depth + 1));
            }
        }
        return result;
    }
}

/// <summary>UIA 服务：元素快照/调用、窗口枚举与聚焦、应用启动</summary>
public sealed class UiaService : IDisposable
{
    private readonly UIA3Automation _automation = new();
    private readonly Dictionary<string, AutomationElement> _refs = new();
    private readonly Dictionary<int, string> _processNames = new();
    private int _generation;

    private static readonly HashSet<ControlType> InteractiveTypes = new()
    {
        ControlType.Button, ControlType.Edit, ControlType.MenuItem, ControlType.ComboBox,
        ControlType.CheckBox, ControlType.ListItem, ControlType.TabItem, ControlType.Hyperlink,
        ControlType.Slider, ControlType.SplitButton, ControlType.Spinner,
    };

    /// <summary>释放 UIA3 自动化对象</summary>
    public void Dispose() => _automation.Dispose();

    // ---------- perception ----------

    /// <summary>对目标窗口做 UIA 树快照（深度≤12、maxElements 截断、空名非交互跳过），generation 自增</summary>
    public SnapshotResultDto Snapshot(string? windowTitle, int maxElements)
    {
        if (maxElements <= 0) maxElements = 500;
        _generation++;
        _refs.Clear();
        var root = ResolveRoot(windowTitle);
        var generation = _generation;
        var counter = new RefCounter();

        var elements = TreeWalker.Collect<AutomationElement, UIElementDto>(
            root,
            el =>
            {
                try { return el.FindAllChildren(); }
                catch (Exception ex)
                {
                    Program.Diag("枚举子元素失败: " + ex.Message);
                    return Array.Empty<AutomationElement>();
                }
            },
            el => TryBuildElement(el, generation, counter),
            maxDepth: 12,
            maxElements: maxElements);

        return new SnapshotResultDto { Elements = elements, Generation = generation };
    }

    /// <summary>按 ref 直调元素 Invoke 模式；跨代/失效返回 -32001，不支持 Invoke 返回 -32002</summary>
    public object Invoke(string refText, int expectedGeneration = -1)
    {
        if (!RefParser.TryParse(refText, out var gen, out _))
        {
            throw new CuaException(JsonRpc.ElementNotFound, $"ref 格式非法: {refText}");
        }
        if (RefParser.ParamGenerationMismatch(expectedGeneration, gen))
        {
            throw new CuaException(JsonRpc.ElementNotFound, $"generation 与 ref 不一致（param={expectedGeneration}，ref 代={gen}）");
        }
        if (gen != _generation)
        {
            throw new CuaException(JsonRpc.ElementNotFound, $"ref 已过期（ref 代={gen}，当前代={_generation}）");
        }
        if (!_refs.TryGetValue(refText, out var el))
        {
            throw new CuaException(JsonRpc.ElementNotFound, $"元素未找到: {refText}");
        }
        try
        {
            var invoke = el.Patterns.Invoke.PatternOrDefault;
            if (invoke == null)
            {
                throw new CuaException(JsonRpc.NotInteractable, $"元素不支持 Invoke 模式: {refText}");
            }
            if (!el.IsEnabled)
            {
                throw new CuaException(JsonRpc.NotInteractable, $"元素已禁用: {refText}");
            }
            invoke.Invoke();
            return new { ok = true, via = "invoke" };
        }
        catch (CuaException) { throw; }
        catch (Exception ex)
        {
            _refs.Remove(refText);
            throw new CuaException(JsonRpc.ElementNotFound, $"元素已失效: {refText}: {ex.Message}");
        }
    }

    /// <summary>ref 序号计数器（lambda 可捕获的可变容器）</summary>
    private sealed class RefCounter
    {
        public int Value;
    }

    /// <summary>构造单个元素 DTO 并登记 ref 映射；单元素异常返回 null（跳过）</summary>
    private UIElementDto? TryBuildElement(AutomationElement el, int generation, RefCounter counter)
    {
        try
        {
            var ct = el.Properties.ControlType.ValueOrDefault;
            var name = el.Name ?? "";
            var interactive = IsInteractive(el, ct);
            if (name.Length == 0 && !interactive)
            {
                return null; // 减噪：空名且非交互
            }
            var rect = el.BoundingRectangle;
            var pid = el.Properties.ProcessId.ValueOrDefault;
            var refText = RefParser.Format(generation, counter.Value++);
            var dto = new UIElementDto
            {
                Ref = refText,
                Type = ControlTypeMap.Map(ct),
                Name = name,
                Bbox = new BBoxDto { X = rect.X, Y = rect.Y, W = rect.Width, H = rect.Height },
                Interactivity = interactive,
                Source = "uia",
                AppName = ProcessNameOf(pid),
                Enabled = el.IsEnabled,
            };
            _refs[refText] = el;
            return dto;
        }
        catch (Exception ex)
        {
            Program.Diag("元素读取失败（已跳过）: " + ex.Message);
            return null;
        }
    }

    /// <summary>交互性判定：类型白名单或支持 Invoke/Toggle/Value 模式</summary>
    private static bool IsInteractive(AutomationElement el, ControlType ct)
    {
        if (InteractiveTypes.Contains(ct)) return true;
        try
        {
            if (el.Patterns.Invoke.PatternOrDefault != null) return true;
            if (el.Patterns.Toggle.PatternOrDefault != null) return true;
            if (el.Patterns.Value.PatternOrDefault != null) return true;
        }
        catch
        {
            // 模式探测失败按非交互处理
        }
        return false;
    }

    /// <summary>定位快照根：默认前台窗口，否则按标题子串（忽略大小写）匹配顶层窗口</summary>
    private AutomationElement ResolveRoot(string? windowTitle)
    {
        IntPtr hwnd;
        if (string.IsNullOrEmpty(windowTitle))
        {
            hwnd = GetForegroundWindow();
            if (hwnd == IntPtr.Zero)
            {
                throw new CuaException(JsonRpc.NotInteractable, "无前台窗口可快照");
            }
        }
        else
        {
            var hit = EnumTopWindows().FirstOrDefault(w =>
                w.Title.Contains(windowTitle, StringComparison.OrdinalIgnoreCase));
            if (hit == null)
            {
                throw new CuaException(JsonRpc.ElementNotFound, $"未找到标题匹配窗口: {windowTitle}");
            }
            hwnd = hit.Hwnd;
        }
        try
        {
            return _automation.FromHandle(hwnd);
        }
        catch (Exception ex)
        {
            throw new CuaException(JsonRpc.InternalError, $"无法附加窗口 UIA: 0x{hwnd.ToInt64():X}: {ex.Message}");
        }
    }

    /// <summary>进程名缓存查询（形如 notepad.exe），失败回退 "pid:N"</summary>
    private string ProcessNameOf(int pid)
    {
        if (pid <= 0) return "";
        if (_processNames.TryGetValue(pid, out var cached)) return cached;
        string name;
        try
        {
            name = Process.GetProcessById(pid).ProcessName + ".exe";
        }
        catch
        {
            name = "pid:" + pid;
        }
        _processNames[pid] = name;
        return name;
    }

    // ---------- window ----------

    private sealed class TopWindow
    {
        public IntPtr Hwnd;
        public string Title = "";
        public uint Pid;
    }

    /// <summary>枚举可见且有标题的顶层窗口</summary>
    public WindowListResultDto ListWindows()
    {
        var foreground = GetForegroundWindow();
        var windows = EnumTopWindows().Select(w =>
        {
            GetWindowRect(w.Hwnd, out var rc);
            return new WindowDto
            {
                Hwnd = w.Hwnd.ToInt64(),
                Title = w.Title,
                ProcessName = ProcessNameOf((int)w.Pid),
                IsForeground = w.Hwnd == foreground,
                Bounds = new BBoxDto { X = rc.Left, Y = rc.Top, W = rc.Right - rc.Left, H = rc.Bottom - rc.Top },
            };
        }).ToList();
        return new WindowListResultDto { Windows = windows };
    }

    /// <summary>聚焦窗口：titleRegex 或 hwnd 二选一；SetForegroundWindow 失败时走 AttachThreadInput 兜底</summary>
    public object FocusWindow(string? titleRegex, long? hwndValue)
    {
        IntPtr hwnd;
        if (hwndValue.HasValue)
        {
            hwnd = new IntPtr(hwndValue.Value);
        }
        else if (!string.IsNullOrEmpty(titleRegex))
        {
            Regex rx;
            try { rx = new Regex(titleRegex, RegexOptions.IgnoreCase); }
            catch (ArgumentException ex)
            {
                throw new CuaException(JsonRpc.InternalError, $"titleRegex 非法: {ex.Message}");
            }
            var hit = EnumTopWindows().FirstOrDefault(w => rx.IsMatch(w.Title));
            if (hit == null)
            {
                throw new CuaException(JsonRpc.ElementNotFound, $"未找到标题匹配窗口: {titleRegex}");
            }
            hwnd = hit.Hwnd;
        }
        else
        {
            throw new CuaException(JsonRpc.InternalError, "window.focus 需要 titleRegex 或 hwnd 参数");
        }

        if (IsIconic(hwnd))
        {
            ShowWindow(hwnd, 9); // SW_RESTORE
        }
        if (!SetForegroundWindow(hwnd))
        {
            var fore = GetForegroundWindow();
            var foreThread = GetWindowThreadProcessId(fore, out _);
            var curThread = GetCurrentThreadId();
            if (foreThread != 0 && foreThread != curThread)
            {
                AttachThreadInput(curThread, foreThread, true);
                BringWindowToTop(hwnd);
                SetForegroundWindow(hwnd);
                AttachThreadInput(curThread, foreThread, false);
            }
            else
            {
                BringWindowToTop(hwnd);
                SetForegroundWindow(hwnd);
            }
        }
        if (GetForegroundWindow() != hwnd)
        {
            throw new CuaException(JsonRpc.NotInteractable, "窗口未能置于前台");
        }
        return new { ok = true, hwnd = hwnd.ToInt64() };
    }

    /// <summary>枚举顶层窗口（可见 + 有标题）</summary>
    private List<TopWindow> EnumTopWindows()
    {
        var list = new List<TopWindow>();
        EnumWindows((hWnd, _) =>
        {
            try
            {
                if (!IsWindowVisible(hWnd)) return true;
                var len = GetWindowTextLength(hWnd);
                if (len <= 0) return true;
                var sb = new StringBuilder(len + 1);
                GetWindowText(hWnd, sb, sb.Capacity);
                var title = sb.ToString();
                if (string.IsNullOrWhiteSpace(title)) return true;
                GetWindowThreadProcessId(hWnd, out var pid);
                list.Add(new TopWindow { Hwnd = hWnd, Title = title, Pid = pid });
            }
            catch
            {
                // 单窗口枚举失败不中断
            }
            return true;
        }, IntPtr.Zero);
        return list;
    }

    // ---------- app ----------

    /// <summary>启动应用：绝对路径或 PATH/PATHEXT 可解析的名称（ShellExecute）</summary>
    public object Launch(string target, string? args, string? workDir)
    {
        try
        {
            var psi = new ProcessStartInfo { FileName = target, UseShellExecute = true };
            if (!string.IsNullOrEmpty(args)) psi.Arguments = args;
            if (!string.IsNullOrEmpty(workDir)) psi.WorkingDirectory = workDir;
            var p = Process.Start(psi);
            if (p == null)
            {
                throw new CuaException(JsonRpc.InternalError, $"Process.Start 未返回进程: {target}");
            }
            return new { ok = true, pid = p.Id };
        }
        catch (CuaException) { throw; }
        catch (Win32Exception wex) when (wex.NativeErrorCode is 2 or 3) // FILE/PATH NOT FOUND
        {
            throw new CuaException(JsonRpc.ElementNotFound, $"启动目标未找到: {target}");
        }
        catch (Win32Exception wex) when (wex.NativeErrorCode == 5) // ACCESS DENIED
        {
            throw new CuaException(JsonRpc.InjectionDenied, $"启动被拒绝（权限不足）: {target}");
        }
        catch (Exception ex)
        {
            throw new CuaException(JsonRpc.InternalError, $"启动失败: {target}: {ex.Message}");
        }
    }

    // ---------- Win32 ----------

    private delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);

    [DllImport("user32.dll")]
    private static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);

    [DllImport("user32.dll")]
    private static extern bool IsWindowVisible(IntPtr hWnd);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetWindowText(IntPtr hWnd, StringBuilder lpString, int nMaxCount);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int GetWindowTextLength(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);

    [DllImport("user32.dll")]
    private static extern IntPtr GetForegroundWindow();

    [DllImport("user32.dll")]
    private static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

    [DllImport("user32.dll")]
    private static extern bool IsIconic(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern bool BringWindowToTop(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern bool AttachThreadInput(uint idAttach, uint idAttachTo, bool fAttach);

    [DllImport("kernel32.dll")]
    private static extern uint GetCurrentThreadId();

    [DllImport("user32.dll")]
    private static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);
}
