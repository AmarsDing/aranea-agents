using System.Text;

namespace Aranea.Cua.Win;

/// <summary>程序入口：配置 UTF-8 编码后进入 stdio JSON-RPC 主循环</summary>
public static class Program
{
    /// <summary>主循环：逐行读 stdin，逐行写 stdout，EOF 退出；单请求异常不崩进程</summary>
    public static int Main()
    {
        Console.OutputEncoding = new UTF8Encoding(false);
        Console.InputEncoding = new UTF8Encoding(false);

        using var uia = new UiaService();
        var input = new InputService();
        var capture = new CaptureService();
        var dispatcher = new Dispatcher(uia, input, capture);

        Diag("aranea-cua-win sidecar 已启动");

        string? line;
        while ((line = Console.In.ReadLine()) != null)
        {
            if (line.Length == 0) continue;
            string response;
            try
            {
                response = dispatcher.HandleLine(line);
            }
            catch (Exception ex)
            {
                Diag("主循环未捕获异常: " + ex);
                response = JsonRpc.ErrorResponse(null, JsonRpc.InternalError, "sidecar 内部错误: " + ex.Message);
            }
            Console.Out.WriteLine(response);
            Console.Out.Flush();
        }
        return 0;
    }

    /// <summary>诊断日志只写 stderr，绝不污染 stdout</summary>
    public static void Diag(string msg)
    {
        try { Console.Error.WriteLine("[cua-win] " + msg); }
        catch { /* stderr 不可用时静默 */ }
    }
}
