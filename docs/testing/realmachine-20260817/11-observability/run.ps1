# 11-observability: audit/events/traces/logs/flow-logs/alert/runner/self-check/heal
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "11"
$ev = Join-Path $PSScriptRoot "evidence"

$gets = @(
    @("OBS-01", "/v1/monitor/audit?page=1&page_size=5", "audit log list"),
    @("OBS-02", "/v1/monitor/events?page=1&page_size=5", "monitor events list"),
    @("OBS-03", "/v1/monitor/traces?page=1&page_size=5", "traces list"),
    @("OBS-04", "/v1/monitor/logs?page=1&page_size=5", "logs query"),
    @("OBS-05", "/v1/monitor/flow-logs?page=1&page_size=5", "flow logs"),
    @("OBS-06", "/v1/monitor/alert-rules", "alert rules"),
    @("OBS-07", "/v1/monitor/alert-metrics", "alert metrics"),
    @("OBS-08", "/v1/monitor/runner-metrics", "runner metrics"),
    @("OBS-09", "/v1/monitor/code-executor-capabilities", "code executor caps"),
    @("OBS-10", "/v1/monitor/self-check-reports", "self-check reports"),
    @("OBS-11", "/v1/monitor/heal-stats", "heal stats"),
    @("OBS-12", "/v1/monitor/heal-records?page=1&page_size=5", "heal records")
)
foreach ($g in $gets) {
    $r = Api-Get $g[1] -OutFile (Join-Path $ev ("{0}.json" -f $g[0].ToLower()))
    $len = $r.Raw.Length
    Record $M $g[0] $g[2] ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$len" $r.Ms
}

# OBS-13: trace detail from first trace id (if any)
$tr = Api-Get "/v1/monitor/traces?page=1&page_size=1"
$tid = $null
try { $tid = $tr.Body.items[0].id } catch {}
if ($tid) {
    $r = Api-Get "/v1/monitor/traces/$tid" -OutFile (Join-Path $ev "obs13-trace-detail.json")
    Record $M "OBS-13" "trace detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) id=$tid" $r.Ms
} else {
    Record $M "OBS-13" "trace detail" "SKIP" "no traces available" 0
}
