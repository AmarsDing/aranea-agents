# 19-cli CLI 测试
$ErrorActionPreference = "Continue"
. (Join-Path $PSScriptRoot "..\_lib.ps1")
$M = "19"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

$exe = "f:\myproject\aranea-agents\bin\aranea.exe"
$env:ARANEA_BASE_URL = "http://localhost:8810"
$env:ARANEA_TOKEN = Get-Token
$env:ARANEA_NO_COLOR = "1"

function Cli-Run([string]$Id, [string]$Name, [string[]]$CmdArgs, [string]$OutName, [int]$TimeoutSec = 90) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $out = & $exe @CmdArgs 2>&1 | Out-String
    $code = $LASTEXITCODE
    $sw.Stop()
    if ($OutName) { [IO.File]::WriteAllText((Join-Path $ev $OutName), $out, [Text.UTF8Encoding]::new($false)) }
    return @{ Code = $code; Ms = $sw.ElapsedMilliseconds; Out = $out }
}

# CLI-01 version
$r = Cli-Run "CLI-01" "version" @("version") "cli01-version.txt"
Record $M "CLI-01" "aranea version" ($(if ($r.Code -eq 0 -and $r.Out.Trim().Length -gt 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) out=$($r.Out.Trim().Substring(0,[Math]::Min(60,$r.Out.Trim().Length)))" $r.Ms

# CLI-02 help
$r = Cli-Run "CLI-02" "help" @("--help") "cli02-help.txt"
$cmds = ([regex]::Matches($r.Out, "^\s+(agent|session|team|graph|tool|skill|mcp|memory|knowledge|monitor|cron|chat|login|system|version)\s", "Multiline")).Count
Record $M "CLI-02" "aranea --help lists commands" ($(if ($r.Code -eq 0 -and $cmds -ge 8) { "PASS" } else { "FAIL" })) "exit=$($r.Code) topLevelCmds~$cmds" $r.Ms

# CLI-03 agent ls -o json
$r = Cli-Run "CLI-03" "agent ls" @("agent", "ls", "-o", "json") "cli03-agent-ls.json"
$cnt = 0; try { $j = $r.Out | ConvertFrom-Json; if ($j.items) { $cnt = @($j.items).Count } elseif ($j -is [array]) { $cnt = $j.Count } } catch {}
Record $M "CLI-03" "agent ls -o json" ($(if ($r.Code -eq 0 -and $cnt -gt 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) agents=$cnt" $r.Ms

# CLI-04 session ls -o json
$r = Cli-Run "CLI-04" "session ls" @("session", "ls", "-o", "json") "cli04-session-ls.json"
$scnt = 0; try { $j = $r.Out | ConvertFrom-Json; if ($j.items) { $scnt = @($j.items).Count } elseif ($j -is [array]) { $scnt = $j.Count } } catch {}
Record $M "CLI-04" "session ls -o json" ($(if ($r.Code -eq 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) sessions=$scnt" $r.Ms

# CLI-05 tool ls -o json
$r = Cli-Run "CLI-05" "tool ls" @("tool", "ls", "-o", "json") "cli05-tool-ls.json"
$tcnt = 0; try { $j = $r.Out | ConvertFrom-Json; if ($j.items) { $tcnt = @($j.items).Count } elseif ($j -is [array]) { $tcnt = $j.Count } } catch {}
Record $M "CLI-05" "tool ls -o json" ($(if ($r.Code -eq 0 -and $tcnt -gt 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) tools=$tcnt" $r.Ms

# CLI-06 monitor events -o json
$r = Cli-Run "CLI-06" "monitor events" @("monitor", "events", "-o", "json") "cli06-monitor-events.json"
Record $M "CLI-06" "monitor events -o json" ($(if ($r.Code -eq 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) len=$($r.Out.Length)" $r.Ms

# CLI-07 session send -y (real LLM roundtrip on FE-created spirit session)
$sid = "6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8"
$chk = Api-Get "/v1/sessions/$sid"
if ($chk.Code -ne "200") { $sid = $null; try { $sid = (Api-Get "/v1/sessions").Body.items[0].id } catch {} }
if ($sid) {
    $r = Cli-Run "CLI-07" "session send" @("session", "send", "-y", "--session", $sid, "--content", "Reply with exactly one word: OK") "cli07-send.txt" 240
    Record $M "CLI-07" "session send -y (real LLM roundtrip)" ($(if ($r.Code -eq 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) sid=$sid out=$($r.Out.Trim().Substring(0,[Math]::Min(80,$r.Out.Trim().Length)))" $r.Ms
} else {
    Record $M "CLI-07" "session send -y" "FAIL" "no session available" 0
}

# CLI-08 invalid command
$r = Cli-Run "CLI-08" "invalid cmd" @("nosuchcmd") "cli08-invalid.txt"
Record $M "CLI-08" "invalid command exit!=0 with error" ($(if ($r.Code -ne 0 -and $r.Out.Length -gt 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code)" $r.Ms

# CLI-09 system info
$r = Cli-Run "CLI-09" "system info" @("system", "info", "-o", "json") "cli09-sysinfo.json"
Record $M "CLI-09" "system info -o json" ($(if ($r.Code -eq 0) { "PASS" } else { "FAIL" })) "exit=$($r.Code) len=$($r.Out.Length)" $r.Ms

Write-Host "CLI DONE"
