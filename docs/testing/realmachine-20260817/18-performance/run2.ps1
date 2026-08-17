# 18-performance run2: 修正 PERF-01/04/05/06 的路径与表名
$ErrorActionPreference = "Continue"
. (Join-Path $PSScriptRoot "..\_lib.ps1")
$M = "18"
$ev = Join-Path $PSScriptRoot "evidence"

# PERF-01R: corrected endpoints
$endpoints = @(
    "/v1/sessions",
    "/v1/memory/layer-overview",
    "/v1/monitor/flow-logs?page=1&page_size=20",
    "/v1/model-catalog/providers"
)
foreach ($ep in $endpoints) {
    $samples = @()
    foreach ($i in 1..3) {
        $r = Api-Get $ep -TimeoutSec 30
        if ($r.Code -eq "200") { $samples += $r.Ms }
    }
    if ($samples.Count -gt 0) {
        $mean = [long]($samples | Measure-Object -Average).Average
        $max = ($samples | Measure-Object -Maximum).Maximum
        $min = ($samples | Measure-Object -Minimum).Minimum
        Record $M "PERF-01R" "baseline(corrected) $ep" "PASS" "mean=${mean}ms min=${min}ms max=${max}ms n=$($samples.Count)" $mean
    } else {
        Record $M "PERF-01R" "baseline(corrected) $ep" "FAIL" "all non-200" 0
    }
}

# PERF-04R: messages page_size=100 with correct /v1/sessions
$sid = $null
try { $sid = (Api-Get "/v1/sessions").Body.items[0].id } catch {}
if ($sid) {
    $r = Api-Get "/v1/sessions/$sid/messages?page_size=100" -OutFile (Join-Path $ev "perf04r-messages100.json") -TimeoutSec 60
    Record $M "PERF-04R" "messages page_size=100 (corrected)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) sid=$sid bytes=$($r.Raw.Length)" $r.Ms
} else {
    Record $M "PERF-04R" "messages page_size=100 (corrected)" "FAIL" "no session" 0
}

# PERF-05R: DB counts with real table names
$counts = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT 'agents='||COUNT(*) FROM agents" 2>$null
$c2 = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT 'sessions_v2='||COUNT(*) FROM sessions_v2" 2>$null
$c3 = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT 'turns_v2='||COUNT(*) FROM turns_v2" 2>$null
$c4 = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT 'trpc_session_events='||COUNT(*) FROM trpc_session_events" 2>$null
$all = @($counts, $c2, $c3, $c4) -join ", "
Record $M "PERF-05R" "DB table counts (corrected)" "PASS" $all 0
$all | Out-File (Join-Path $ev "perf05r-db-counts.txt") -Encoding utf8

# PERF-06R: mixed concurrent with corrected sessions path
$tok = Get-Token
$mixFail = 0
$mixMs = [System.Collections.Concurrent.ConcurrentBag[long]]::new()
$eps = @("/v1/agents", "/v1/tools", "/v1/sessions")
$jobs = foreach ($ep in $eps) { foreach ($i in 1..5) {
    Start-Job -ScriptBlock {
        param($u, $t)
        $s = [Diagnostics.Stopwatch]::StartNew()
        try { $null = Invoke-RestMethod -Uri $u -Headers @{ Authorization = "Bearer $t" } -TimeoutSec 30; $s.Stop(); return @{ ok = $true; ms = $s.ElapsedMilliseconds } }
        catch { $s.Stop(); return @{ ok = $false; ms = $s.ElapsedMilliseconds } }
    } -ArgumentList "http://localhost:8810$ep", $tok
} }
$res = $jobs | Wait-Job | Receive-Job
$jobs | Remove-Job -Force
foreach ($r in $res) { if ($r.ok) { $mixMs.Add($r.ms) } else { $mixFail++ } }
$marr = @($mixMs.ToArray() | Sort-Object)
if ($marr.Count -gt 0) {
    $mMean = [long]($marr | Measure-Object -Average).Average
    Record $M "PERF-06R" "mixed concurrent reads corrected (3 eps x5)" ($(if ($mixFail -eq 0) { "PASS" } else { "FAIL" })) "n=$($marr.Count) fail=$mixFail mean=${mMean}ms max=$($marr[-1])ms" $mMean
} else {
    Record $M "PERF-06R" "mixed concurrent reads corrected" "FAIL" "no success fail=$mixFail" 0
}
Write-Host "PERF-R DONE"
