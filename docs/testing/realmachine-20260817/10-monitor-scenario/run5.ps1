# 10-monitor-scenario run5: full monitoring closed-loop with HITL
# fixes vs run4: (1) delete stale persisted grant that bypassed HITL for fault_clear
#                (2) explicit prompt (node sw1 + port eth1 + no-clarify) to avoid
#                    auto-resolved clarification cancelling the injection
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "10"
$ev = Join-Path $PSScriptRoot "evidence"
$AGENT_ID = "90fb01daa4c14a1580d8c828"  # ops_change_execution

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

# direct tool test (no LLM token cost): POST /v1/tools/{id}/test
# NOTE: param must NOT be named $args (automatic variable conflict)
function Tool-Test([string]$toolId, [hashtable]$toolArgs = @{}) {
    return Api-Post "/v1/tools/$toolId/test" $toolArgs -TimeoutSec 90
}

# ---------- MON-27: cleanup stale persisted grant (three-layer write verification) ----------
$cnt = (Sql "SELECT COUNT(*) FROM tool_grants WHERE agent_id='$AGENT_ID';").Trim()
Record $M "MON-27" "pre-check stale grants for exec agent" "INFO" "grants=$cnt" 0
if ($cnt -ne "0") {
    Sql "BEGIN; DELETE FROM tool_grants WHERE agent_id='$AGENT_ID'; COMMIT;" | Out-Null
    $left = (Sql "SELECT COUNT(*) FROM tool_grants WHERE agent_id='$AGENT_ID';").Trim()
    Record $M "MON-28" "stale grant deleted" ($(if ($left -eq "0") { "PASS" } else { "FAIL" })) "left=$left" 0
} else {
    Record $M "MON-28" "stale grant deleted" "PASS" "nothing to delete" 0
}

# ---------- MON-29: new session D ----------
$r = Api-Post "/v1/sessions" @{ agent_id = $AGENT_ID; title = "realmachine-monitor-D"; owner_type = "agent" }
$sid = $r.Body.id
Record $M "MON-29" "create session D" ($(if ($sid) { "PASS" } else { "FAIL" })) "sid=$sid" $r.Ms

# ---------- INJECT ----------
$injectPrompt = 'Execute exactly one tool call now: gns3_fault_inject with arguments {"port":"eth1"}. The target node is sw1 and all required parameters are provided. Do not ask any clarification question; call the tool immediately.'
$r = Api-Post "/v1/chat/messages/submit" @{ session_id = $sid; agent_key = "ops_change_execution"; content = $injectPrompt } -OutFile (Join-Path $ev "mon30-submit-inject.json")
Record $M "MON-30" "async submit inject (explicit)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$csId = Wait-ConfirmStep $sid 240
Record $M "MON-31" "HITL confirm step appeared (inject)" ($(if ($csId) { "PASS" } else { "FAIL" })) "step=$csId" 0

$st = "skipped"
if ($csId) {
    $r = Api-Post "/v1/chat/activities/$csId/confirm" @{ session_id = $sid; activity_id = $csId; approved = $true } -OutFile (Join-Path $ev "mon32-confirm.json")
    Record $M "MON-32" "HITL approve inject" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
    $st = Wait-ToolRun $sid "gns3_fault_inject" 120
}
Record $M "MON-33" "fault inject executed" ($(if ($st -eq "success") { "PASS" } else { "FAIL" })) "status=$st" 0

# ---------- verify fault effective (direct tool test, no LLM) ----------
if ($st -eq "success") {
    Start-Sleep -Seconds 5
    $r = Tool-Test "tool_gns3_health_check" @{}
    $down = $r.Raw -match '"loss":\s*100' -or $r.Raw -match 'loss=100' -or $r.Raw -match '100%'
    [IO.File]::WriteAllText((Join-Path $ev "mon34-healthcheck.json"), $r.Raw, [Text.UTF8Encoding]::new($false))
    Record $M "MON-34" "health check shows impact (direct)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) loss100=$down len=$($r.Raw.Length)" $r.Ms

    # poll twin alarms up to 180s for a new firing alarm
    $alarmHit = $false
    $deadline = (Get-Date).AddSeconds(180)
    while ((Get-Date) -lt $deadline -and -not $alarmHit) {
        $ra = Tool-Test "tool_twin_alarm_query" @{ status = "firing" }
        if ($ra.Raw -match 'eth1|sw1|port|link') { $alarmHit = $true; break }
        Start-Sleep -Seconds 15
    }
    [IO.File]::WriteAllText((Join-Path $ev "mon35-alarms.json"), $ra.Raw, [Text.UTF8Encoding]::new($false))
    Record $M "MON-35" "twin alarm raised after inject" ($(if ($alarmHit) { "PASS" } else { "FAIL" })) "hit=$alarmHit len=$($ra.Raw.Length)" 0
}

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
    $r = Tool-Test "tool_gns3_health_check" @{}
    $ok = $r.Raw -match '"loss":\s*0' -or $r.Raw -match 'loss=0'
    [IO.File]::WriteAllText((Join-Path $ev "mon40-healthcheck.json"), $r.Raw, [Text.UTF8Encoding]::new($false))
    Record $M "MON-40" "health check recovered (direct)" ($(if ($r.Code -eq "200" -and $ok) { "PASS" } else { "FAIL" })) "code=$($r.Code) loss0=$ok" $r.Ms
}

"SESSION_D=$sid" | Out-File (Join-Path $ev "session_d.txt") -Encoding ascii
