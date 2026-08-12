# aranea-cua-win sidecar 手工冒烟脚本
#
# 用法：
#   powershell -ExecutionPolicy Bypass -File f:\aranea-agents\test\cua-win-smoke\smoke.ps1
#   可用 -Exe 指定 sidecar 路径（默认 f:\aranea-agents\bin\cua\aranea-cua-win.exe）
#
# 流程：device.ping → device.info → app.launch(notepad) → window.focus(记事本|Notepad)
#       → perception.snapshot → action.type("hello") → action.key("ctrl+s") → esc 关掉保存对话框
# 注意：真实注入键鼠，请在无重要操作的桌面会话中运行；ctrl+s 会弹出"另存为"，脚本随后发 esc 关闭。

param(
    [string]$Exe = "f:\aranea-agents\bin\cua\aranea-cua-win.exe",
    [int]$ReadTimeoutMs = 15000
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Exe)) {
    Write-Host "sidecar 不存在: $Exe（请先 dotnet publish）" -ForegroundColor Red
    exit 1
}

$psi = [System.Diagnostics.ProcessStartInfo]::new($Exe)
$psi.RedirectStandardInput = $true
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.UseShellExecute = $false
$psi.StandardInputEncoding = [System.Text.UTF8Encoding]::new($false)
$psi.StandardOutputEncoding = [System.Text.UTF8Encoding]::new($false)

$proc = [System.Diagnostics.Process]::Start($psi)
Write-Host "[smoke] sidecar 已启动, pid=$($proc.Id)" -ForegroundColor Cyan

$script:reqId = 0

function Send-Request {
    param([string]$Method, [hashtable]$Params = $null)
    $script:reqId++
    $frame = @{ jsonrpc = "2.0"; id = $script:reqId; method = $Method }
    if ($null -ne $Params) { $frame["params"] = $Params }
    $json = $frame | ConvertTo-Json -Compress -Depth 8
    Write-Host "[--> ] $json" -ForegroundColor DarkGray
    $proc.StandardInput.WriteLine($json)
    $proc.StandardInput.Flush()
    $task = $proc.StandardOutput.ReadLineAsync()
    if ($task.Wait($ReadTimeoutMs)) {
        $resp = $task.Result
        # 截图等大响应截断展示
        $show = if ($resp.Length -gt 400) { $resp.Substring(0, 400) + "…(截断, 共 $($resp.Length) 字符)" } else { $resp }
        Write-Host "[ <--] $show" -ForegroundColor Green
        return $resp
    }
    Write-Host "[ <--] 超时（${ReadTimeoutMs}ms）无响应" -ForegroundColor Red
    return $null
}

try {
    Send-Request "device.ping" | Out-Null
    Send-Request "device.info" | Out-Null

    Send-Request "app.launch" @{ target = "notepad" } | Out-Null
    Start-Sleep -Milliseconds 2000  # 等记事本窗口起来

    Send-Request "window.list" | Out-Null
    Send-Request "window.focus" @{ titleRegex = "记事本|Notepad" } | Out-Null
    Start-Sleep -Milliseconds 500

    $snap = Send-Request "perception.snapshot" @{ maxElements = 100 }
    if ($snap) {
        try {
            $gen = ($snap | ConvertFrom-Json).result.generation
            Write-Host "[smoke] snapshot generation=$gen" -ForegroundColor Cyan
        } catch { }
    }

    Send-Request "action.type" @{ text = "hello"; intervalMs = 20 } | Out-Null
    Send-Request "action.key" @{ combo = "ctrl+s" } | Out-Null
    Start-Sleep -Milliseconds 800
    Send-Request "action.key" @{ combo = "esc" } | Out-Null   # 关闭“另存为”对话框
}
finally {
    try { $proc.StandardInput.Close() } catch { }  # EOF → sidecar 自行退出
    if (-not $proc.WaitForExit(5000)) {
        Write-Host "[smoke] sidecar 未在 5s 内退出，强制结束" -ForegroundColor Yellow
        $proc.Kill()
    }
    $stderr = $proc.StandardError.ReadToEnd()
    if ($stderr) { Write-Host "[stderr] $stderr" -ForegroundColor DarkYellow }
    Write-Host "[smoke] 完成，sidecar 退出码=$($proc.ExitCode)" -ForegroundColor Cyan
}
