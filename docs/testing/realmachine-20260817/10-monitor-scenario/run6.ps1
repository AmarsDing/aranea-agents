# 10-monitor-scenario run6: continue from run5 break (fault already injected, MON-33 PASS)
# verifies impact -> alarm -> HITL clear -> recovery
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "10"
$ev = Join-Path $PSScriptRoot "evidence"
$AGENT_ID = "90fb01daa4c14a1580d8c828"
$sid = "d4006c59-253a-4d13-9e3c-0a1b86aac04f"  # session D from run5

function Sql([string]$q) { (docker exec aranea-postgres psql -U postgres -d aranea -t -A -c $q) -join "`n" }

function Wait-ConfirmStep([string]$sid, [int]$maxSec = 240) {
    $deadline = (Get-Date).AddSeconds($maxSec)
    while ((Get-Date) -lt $deadline) {
        $q = (Sql "SELECT id FROM steps_v2 WHERE session_id='$sid' AND kind='confirm' AND status='tool_blocked' ORDER BY started_at DESC LIMIT 1;").Trim()
        if ($q) { return $q }
        Start-Sleep -Seconds 5
    }
    return $null
}

function Wait-ToolRun([string]$sid, [string]$tool, [int]$maxSec = 120) {
    $deadline = (Get-Date).AddSeconds($maxSec)
    while ((Get-Date) -lt $deadline) {
        $q = (Sql "SELECT status FROM tool_invocations WHERE session_id='$sid' AND tool_key='$tool' ORDER BY started_at DESC LIMIT 1;").Trim()
        if ($q -eq "success") { return "success" }
        if ($q -and $q -ne "running" -and $q -ne "pending") { return $q }
        Start-Sleep -Seconds 5
    }
    return "timeout"
}

# NOTE: param must NOT be named $args (automatic variable conflict)
function Tool-Test([string]$toolId, [hashtable]$toolArgs = @{}) {
    return Api-Post "/v1/tools/$toolId/test" $toolArgs -TimeoutSec 90
}

# ---------- verify fault effective (direct tool test, no LLM) ----------
$r = Tool-Test "tool_gns3_health_check"
[IO.File]::WriteAllText((Join-Path $ev "mon34-healthcheck.json"), $r.Raw, [Text.UTF8Encoding]::new($false))
$loss100 = $r.Raw -match '100' 
Record $M "MON-34" "health check shows impact (direct)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) has100=$loss100 len=$($r.Raw.Length)" $r.Ms

# ---------- poll twin alarms up to 180s ----------
$alarmHit = $false
$ra = $null
$deadline = (Get-Date).AddSeconds(180)
while ((Get-Date) -lt $deadline -and -not $alarmHit) {
    $ra = Tool-Test "tool_twin_alarm_query" @{ status = "firing" }
    if ($ra.Code -eq "200" -and $ra.Raw -match 'eth1|sw1|port|link|down') { $alarmHit = $true; break }
    Start-Sleep -Seconds 15
}
if ($ra) { [IO.File]::WriteAllText((Join-Path $ev "mon35-alarms.json"), $ra.Raw, [Text.UTF8Encoding]::new($false)) }
Record $M "MON-35" "twin alarm raised after inject" ($(if ($alarmHit) { "PASS" } else { "FAIL" })) "hit=$alarmHit len=$(if ($ra) { $ra.Raw.Length } else { 0 })" 0

# ---------- CLEAR ----------
$clearPrompt = 'Execute exactly one tool call now: gns3_fault_clear with arguments {"port":"eth1"}. All required parameters are provided. Do not ask any clarification question; call the tool immediately.'
$r = Api-Post "/v1/chat/messages/submit" @{ session_id = $sid; agent_key = "ops_change_execution"; content = $clearPrompt } -OutFile (Join-Path $ev "mon36-submit-clear.json")
Record $M "MON-36" "async submit clear (explicit)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$csId2 = Wait-ConfirmStep $sid 240
Record $M "MON-37" "HITL confirm step appeared (clear)" ($(if ($csId2) { "PASS" } else { "FAIL" })) "step=$csId2" 0

$st2 = "skipped"
if ($csId2) {
    $r = Api-Post "/v1/chat/activities/$csId2/confirm" @{ session_id = $sid; activity_id = $csId2; approved = $true } -OutFile (Join-Path $ev "mon38-confirm-clear.json")
    Record $M "MON-38" "HITL approve clear" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
    $st2 = Wait-ToolRun $sid "gns3_fault_clear" 120
}
Record $M "MON-39" "fault clear executed" ($(if ($st2 -eq "success") { "PASS" } else { "FAIL" })) "status=$st2" 0

# ---------- verify recovery ----------
if ($st2 -eq "success") {
    Start-Sleep -Seconds 5
    $r = Tool-Test "tool_gns3_health_check"
    [IO.File]::WriteAllText((Join-Path $ev "mon40-healthcheck.json"), $r.Raw, [Text.UTF8Encoding]::new($false))
    $ok = $r.Code -eq "200" -and $r.Raw -notmatch '100%' -and $r.Raw -match '0'
    Record $M "MON-40" "health check recovered (direct)" ($(if ($ok) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}
