# 17-frontend 静态可达性测试
$ErrorActionPreference = "Continue"
. (Join-Path $PSScriptRoot "..\_lib.ps1")
$M = "17"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
$FE = "http://localhost:9301"

function Fe-Get([string]$Path) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    try {
        $r = Invoke-WebRequest -Uri "$FE$Path" -UseBasicParsing -TimeoutSec 15
        $sw.Stop()
        return @{ Code = $r.StatusCode; Ms = $sw.ElapsedMilliseconds; Content = $r.Content; Headers = $r.Headers }
    } catch {
        $sw.Stop()
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Code = $code; Ms = $sw.ElapsedMilliseconds; Content = ""; Err = $_.Exception.Message }
    }
}

# FE-01 index.html
$r = Fe-Get "/"
$ok = ($r.Code -eq 200 -and $r.Content -match "Aranea Agent Orchestrator")
[IO.File]::WriteAllText((Join-Path $ev "fe01-index.html"), $r.Content, [Text.UTF8Encoding]::new($false))
Record $M "FE-01" "GET / returns index.html with title" ($(if ($ok) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Content.Length)" $r.Ms

# FE-02 main JS bundle
$jsPath = ([regex]::Match($r.Content, '/assets/index-[^"]+\.js')).Value
if ($jsPath) {
    $rj = Fe-Get $jsPath
    Record $M "FE-02" "main JS bundle reachable" ($(if ($rj.Code -eq 200 -and $rj.Content.Length -gt 1MB) { "PASS" } else { "FAIL" })) "code=$($rj.Code) path=$jsPath size=$($rj.Content.Length)" $rj.Ms
} else {
    Record $M "FE-02" "main JS bundle reachable" "FAIL" "no js path in html" 0
}

# FE-03 CSS
$cssPath = ([regex]::Match($r.Content, '/assets/index-[^"]+\.css')).Value
if ($cssPath) {
    $rc = Fe-Get $cssPath
    Record $M "FE-03" "main CSS reachable" ($(if ($rc.Code -eq 200) { "PASS" } else { "FAIL" })) "code=$($rc.Code) path=$cssPath size=$($rc.Content.Length)" $rc.Ms
} else {
    Record $M "FE-03" "main CSS reachable" "FAIL" "no css path in html" 0
}

# FE-04 runtime-config.json
$rc4 = Fe-Get "/assets/config/runtime-config.json"
$validJson = $false
try { $null = $rc4.Content | ConvertFrom-Json; $validJson = $true } catch {}
Record $M "FE-04" "runtime-config.json reachable & valid JSON" ($(if ($rc4.Code -eq 200 -and $validJson) { "PASS" } else { "FAIL" })) "code=$($rc4.Code) content=$($rc4.Content)" $rc4.Ms

# FE-05 favicon
$rf = Fe-Get "/favicon.svg"
Record $M "FE-05" "favicon.svg reachable" ($(if ($rf.Code -eq 200) { "PASS" } else { "FAIL" })) "code=$($rf.Code) size=$($rf.Content.Length)" $rf.Ms

# FE-06 SPA fallback /overview
$ro = Fe-Get "/overview"
$isIndex = ($ro.Content -match "q-app")
Record $M "FE-06" "SPA route fallback /overview" ($(if ($ro.Code -eq 200 -and $isIndex) { "PASS" } else { "FAIL" })) "code=$($ro.Code) isIndex=$isIndex" $ro.Ms

# FE-07 backend healthz (frontend dependency)
$rh = Api-Get "/healthz" -TimeoutSec 10
Record $M "FE-07" "backend /healthz reachable" ($(if ($rh.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($rh.Code)" $rh.Ms

# FE-08 inject backendUrl into runtime-config, verify served
$cfgFile = "f:\myproject\aranea-agents\web\dist\spa\assets\config\runtime-config.json"
$origCfg = [IO.File]::ReadAllText($cfgFile)
[IO.File]::WriteAllText((Join-Path $ev "fe08-runtime-config.orig.json"), $origCfg, [Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText($cfgFile, '{"backendUrl":"http://localhost:8810","wsOrigin":"http://localhost:8810"}', [Text.UTF8Encoding]::new($false))
Start-Sleep -Milliseconds 300
$rc8 = Fe-Get "/assets/config/runtime-config.json"
$injected = ($rc8.Content -match "8810")
Record $M "FE-08" "runtime-config backendUrl injection served" ($(if ($rc8.Code -eq 200 -and $injected) { "PASS" } else { "FAIL" })) "served=$($rc8.Content)" $rc8.Ms
[IO.File]::WriteAllText((Join-Path $ev "fe08-runtime-config.active.json"), $rc8.Content, [Text.UTF8Encoding]::new($false))
Write-Host "runtime-config left configured for browser tests (restorable from fe08-runtime-config.orig.json)"
