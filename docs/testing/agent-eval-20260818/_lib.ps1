﻿﻿﻿# Aranea 指令驱动评测公共库（agent-eval-20260818）
# 与 realmachine-20260817/_lib.ps1 同口径，TestRoot 指向本评测目录
$script:BaseUrl = "http://127.0.0.1:8810"
$script:TokenFile = "f:\myproject\aranea-agents\docker\.test-token.txt"
$script:LoginFile = "f:\myproject\aranea-agents\docker\.test-login.json"
$script:TestRoot = "f:\myproject\aranea-agents\docs\testing\agent-eval-20260818"

function Get-Token {
    $t = (Get-Content $script:TokenFile -Raw -ErrorAction SilentlyContinue)
    if ($t) { return $t.Trim() }
    $resp = Invoke-RestMethod -Method Post -Uri "$($script:BaseUrl)/v1/admins/login" -ContentType "application/json" -InFile $script:LoginFile
    $resp.token | Out-File -Encoding ascii $script:TokenFile
    return $resp.token
}

function Renew-Token {
    $resp = Invoke-RestMethod -Method Post -Uri "$($script:BaseUrl)/v1/admins/login" -ContentType "application/json" -InFile $script:LoginFile
    $resp.token | Out-File -Encoding ascii $script:TokenFile
    return $resp.token
}

# Api-Call -Method GET -Path "/v1/agents" -Body obj -OutFile path
function Api-Call {
    param([string]$Method = "GET", [Parameter(Mandatory)][string]$Path, $Body = $null, [string]$OutFile = $null, [int]$TimeoutSec = 60)
    $tok = Get-Token
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $respFile = [IO.Path]::GetTempFileName()
    # 响应体落临时文件按字节读回 UTF-8，避免 curl 经控制台代码页捕获导致中文 mojibake/JSON 解析失败。
    $curlArgLine = @("-s", "-o", "$respFile", "-w", "%{http_code}", "-m", "$TimeoutSec", "-X", $Method, "-H", "Authorization: Bearer $tok", "-H", "Content-Type: application/json")
    $tmp = $null
    if ($null -ne $Body) {
        $bf = $Body
        if ($Body -isnot [string] -or (Test-Path $Body -ErrorAction SilentlyContinue) -eq $false) {
            if ($Body -is [string]) { $bf = $Body } else { $bf = ($Body | ConvertTo-Json -Depth 20 -Compress) }
        }
        $tmp = [IO.Path]::GetTempFileName()
        [IO.File]::WriteAllText($tmp, $bf, [Text.UTF8Encoding]::new($false))
        $curlArgLine += @("--data-binary", "@$tmp")
    }
    $code = (& curl.exe @curlArgLine "$($script:BaseUrl)$Path" 2>$null | Out-String).Trim()
    $sw.Stop()
    if ($tmp) { Remove-Item $tmp -Force -ErrorAction SilentlyContinue }
    $bodyText = [IO.File]::ReadAllText($respFile, [Text.UTF8Encoding]::new($false))
    Remove-Item $respFile -Force -ErrorAction SilentlyContinue
    if ($OutFile) { [IO.File]::WriteAllText($OutFile, $bodyText, [Text.UTF8Encoding]::new($false)) }
    $parsed = $null
    try { $parsed = $bodyText | ConvertFrom-Json } catch {}
    return [pscustomobject]@{ Code = $code; Ms = $sw.ElapsedMilliseconds; Raw = $bodyText; Body = $parsed }
}

function Api-Get { param([string]$Path, [string]$OutFile, [int]$TimeoutSec = 60) Api-Call -Method GET -Path $Path -OutFile $OutFile -TimeoutSec $TimeoutSec }
function Api-Post { param([string]$Path, $Body, [string]$OutFile, [int]$TimeoutSec = 180) Api-Call -Method POST -Path $Path -Body $Body -OutFile $OutFile -TimeoutSec $TimeoutSec }
function Api-Patch { param([string]$Path, $Body, [int]$TimeoutSec = 60) Api-Call -Method PATCH -Path $Path -Body $Body -TimeoutSec $TimeoutSec }
function Api-Delete { param([string]$Path, [int]$TimeoutSec = 60) Api-Call -Method DELETE -Path $Path -TimeoutSec $TimeoutSec }

# Record -Module "02" -Id "B02-01" -Name "..." -Result PASS -Detail "..." -Ms 123
function Record {
    param([string]$Module, [string]$Id, [string]$Name, [string]$Result, [string]$Detail = "", [long]$Ms = 0)
    $dir = Get-ChildItem "$($script:TestRoot)" -Directory | Where-Object { $_.Name -like "$Module-*" } | Select-Object -First 1
    $evDir = Join-Path $dir.FullName "evidence"
    New-Item -ItemType Directory -Force -Path $evDir | Out-Null
    $line = "| $Id | $Name | $Result | ${Ms}ms | $Detail |"
    Add-Content -Path (Join-Path $evDir "results.md") -Value $line -Encoding UTF8
    Write-Host "[$Result] $Id $Name ($Ms ms) $Detail"
}
