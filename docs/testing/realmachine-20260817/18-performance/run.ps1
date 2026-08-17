# 18-performance 性能测试
$ErrorActionPreference = "Continue"
. (Join-Path $PSScriptRoot "..\_lib.ps1")
$M = "18"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# ---------- PERF-01: baseline latency of 9 read endpoints x3 ----------
$endpoints = @(
    "/healthz",
    "/v1/agents",
    "/v1/chat/sessions",
    "/v1/tools",
    "/v1/teams",
    "/v1/graphs",
    "/v1/memory/overview",
    "/v1/observability/flowlogs?page_size=20",
    "/v1/model-catalog/providers"
)
$baseline = @{}
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
        $baseline[$ep] = @{ mean = $mean; min = $min; max = $max; n = $samples.Count }
        $verdict = if ($mean -lt 500) { "PASS" } else { "FAIL" }
        Record $M "PERF-01" "baseline $ep" $verdict "mean=${mean}ms min=${min}ms max=${max}ms n=$($samples.Count)" $mean
    } else {
        Record $M "PERF-01" "baseline $ep" "FAIL" "all samples non-200" 0
    }
}
$baseline | ConvertTo-Json -Depth 5 | Out-File (Join-Path $ev "perf01-baseline.json") -Encoding utf8

# ---------- PERF-02: 10 concurrent x 5 waves GET /v1/agents ----------
$tok = Get-Token
$allMs = [System.Collections.Concurrent.ConcurrentBag[long]]::new()
$failCount = 0
$sw = [Diagnostics.Stopwatch]::StartNew()
foreach ($wave in 1..5) {
    $jobs = foreach ($i in 1..10) {
        Start-Job -ScriptBlock {
            param($u, $t)
            $s = [Diagnostics.Stopwatch]::StartNew()
            try {
                $null = Invoke-RestMethod -Uri $u -Headers @{ Authorization = "Bearer $t" } -TimeoutSec 30
                $s.Stop(); return @{ ok = $true; ms = $s.ElapsedMilliseconds }
            } catch { $s.Stop(); return @{ ok = $false; ms = $s.ElapsedMilliseconds } }
        } -ArgumentList "http://localhost:8810/v1/agents", $tok
    }
    $res = $jobs | Wait-Job | Receive-Job
    $jobs | Remove-Job -Force
    foreach ($r in $res) { if ($r.ok) { $allMs.Add($r.ms) } else { $script:failCount++ } }
}
$sw.Stop()
$arr = @($allMs.ToArray() | Sort-Object)
if ($arr.Count -gt 0) {
    $mean = [long]($arr | Measure-Object -Average).Average
    $p95idx = [Math]::Min([Math]::Ceiling($arr.Count * 0.95) - 1, $arr.Count - 1)
    $p95 = $arr[$p95idx]
    $verdict = if ($failCount -eq 0) { "PASS" } else { "FAIL" }
    Record $M "PERF-02" "10-concurrency x5 waves GET /v1/agents" $verdict "total=$($sw.ElapsedMilliseconds)ms n=$($arr.Count) fail=$failCount mean=${mean}ms p95=${p95}ms max=$($arr[-1])ms" $mean
    @{ totalMs = $sw.ElapsedMilliseconds; n = $arr.Count; fail = $failCount; mean = $mean; p95 = $p95; max = $arr[-1]; samples = $arr } | ConvertTo-Json | Out-File (Join-Path $ev "perf02-concurrency.json") -Encoding utf8
} else {
    Record $M "PERF-02" "10-concurrency x5 waves GET /v1/agents" "FAIL" "no successful samples fail=$failCount" 0
}

# ---------- PERF-03: docker stats snapshot ----------
$stats = docker stats --no-stream --format "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}" 2>$null
$stats | Out-File (Join-Path $ev "perf03-docker-stats.txt") -Encoding utf8
$aranea = $stats | Where-Object { $_ -match "aranea|postgres|redis" }
Record $M "PERF-03" "docker stats snapshot" "PASS" ("containers=" + @($stats).Count + "; " + (($aranea | Select-Object -First 3) -join " ; ")) 0

# ---------- PERF-04: large payload messages page_size=100 ----------
$sid = $null
try { $sid = (Api-Get "/v1/chat/sessions").Body.items[0].id } catch {}
if ($sid) {
    $r = Api-Get "/v1/chat/sessions/$sid/messages?page_size=100" -OutFile (Join-Path $ev "perf04-messages100.json") -TimeoutSec 60
    Record $M "PERF-04" "messages page_size=100" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) sid=$sid bytes=$($r.Raw.Length)" $r.Ms
} else {
    Record $M "PERF-04" "messages page_size=100" "FAIL" "no session found" 0
}

# ---------- PERF-05: DB response ----------
$sw5 = [Diagnostics.Stopwatch]::StartNew()
$db1 = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT 1;" 2>$null
$sw5.Stop()
$dbMs = $sw5.ElapsedMilliseconds
$sw6 = [Diagnostics.Stopwatch]::StartNew()
$counts = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT 'agents='||COUNT(*) FROM agents; SELECT 'sessions='||COUNT(*) FROM chat_sessions; SELECT 'turns='||COUNT(*) FROM turns_v2; SELECT 'events='||COUNT(*) FROM session_events;" 2>$null
$sw6.Stop()
Record $M "PERF-05" "DB SELECT 1 + table counts" ($(if ($db1 -match "1") { "PASS" } else { "FAIL" })) "select1=${dbMs}ms counts=$($sw6.ElapsedMilliseconds)ms [$(($counts) -join ', ')]" $dbMs
$counts | Out-File (Join-Path $ev "perf05-db-counts.txt") -Encoding utf8

# ---------- PERF-06: mixed concurrent reads ----------
$mixFail = 0
$mixMs = [System.Collections.Concurrent.ConcurrentBag[long]]::new()
$eps = @("/v1/agents", "/v1/tools", "/v1/chat/sessions")
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
    Record $M "PERF-06" "mixed concurrent reads (3 eps x5)" ($(if ($mixFail -eq 0) { "PASS" } else { "FAIL" })) "n=$($marr.Count) fail=$mixFail mean=${mMean}ms max=$($marr[-1])ms" $mMean
} else {
    Record $M "PERF-06" "mixed concurrent reads" "FAIL" "no success fail=$mixFail" 0
}
Write-Host "PERF DONE"
