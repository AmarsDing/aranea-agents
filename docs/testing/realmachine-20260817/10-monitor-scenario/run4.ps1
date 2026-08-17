# 10-monitor-scenario run4: async submit + HITL confirm + verify (inject then clear)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "10"
$ev = Join-Path $PSScriptRoot "evidence"

function Wait-ConfirmStep([string]$sid, [int]$maxSec = 240) {
    $deadline = (Get-Date).AddSeconds($maxSec)
    while ((Get-Date) -lt $deadline) {
        $q = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT id FROM steps_v2 WHERE session_id='$sid' AND kind='confirm' AND status='tool_blocked' ORDER BY id DESC LIMIT 1;"
        $q = "$q".Trim()
        if ($q) { return $q }
        Start-Sleep -Seconds 5
    }
    return $null
}

function Wait-ToolRun([string]$sid, [string]$tool, [int]$maxSec = 120) {
    $deadline = (Get-Date).AddSeconds($maxSec)
    while ((Get-Date) -lt $deadline) {
        $q = docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT status FROM tool_invocations WHERE session_id='$sid' AND tool_key='$tool' ORDER BY started_at DESC LIMIT 1;"
        $q = "$q".Trim()
        if ($q -eq "success") { return "success" }
        if ($q -and $q -ne "running" -and $q -ne "pending") { return $q }
        Start-Sleep -Seconds 5
    }
    return "timeout"
}

# new session for clean run
$r = Api-Post "/v1/sessions" @{ agent_id = "90fb01daa4c14a1580d8c828"; title = "realmachine-monitor-C"; owner_type = "agent" }
$sid = $r.Body.id
Record $M "MON-18" "create session C" ($(if ($sid) { "PASS" } else { "FAIL" })) "sid=$sid" $r.Ms

# --- INJECT ---
$r = Api-Post "/v1/chat/messages/submit" @{ session_id = $sid; agent_key = "ops_change_execution"; content = "Inject a fault on port eth1 using the gns3_fault_inject tool." } -OutFile (Join-Path $ev "mon19-submit-inject.json")
Record $M "MON-19" "async submit inject" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$csId = Wait-ConfirmStep $sid 240
Record $M "MON-20" "HITL confirm step appeared" ($(if ($csId) { "PASS" } else { "FAIL" })) "step=$csId" 0

if ($csId) {
    $r = Api-Post "/v1/chat/activities/$csId/confirm" @{ session_id = $sid; activity_id = $csId; approved = $true } -OutFile (Join-Path $ev "mon21-confirm.json")
    Record $M "MON-21" "HITL approve inject" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) body=$($r.Raw.Substring(0,[Math]::Min(120,$r.Raw.Length)))" $r.Ms
    $st = Wait-ToolRun $sid "gns3_fault_inject" 120
    Record $M "MON-22" "fault inject executed" ($(if ($st -eq "success") { "PASS" } else { "FAIL" })) "status=$st" 0
}

# --- CLEAR ---
$r = Api-Post "/v1/chat/messages/submit" @{ session_id = $sid; agent_key = "ops_change_execution"; content = "Now clear the fault on port eth1 using the gns3_fault_clear tool." } -OutFile (Join-Path $ev "mon23-submit-clear.json")
Record $M "MON-23" "async submit clear" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$csId2 = Wait-ConfirmStep $sid 240
Record $M "MON-24" "HITL confirm step (clear)" ($(if ($csId2) { "PASS" } else { "FAIL" })) "step=$csId2" 0

if ($csId2) {
    $r = Api-Post "/v1/chat/activities/$csId2/confirm" @{ session_id = $sid; activity_id = $csId2; approved = $true } -OutFile (Join-Path $ev "mon25-confirm-clear.json")
    Record $M "MON-25" "HITL approve clear" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
    $st2 = Wait-ToolRun $sid "gns3_fault_clear" 120
    Record $M "MON-26" "fault clear executed" ($(if ($st2 -eq "success") { "PASS" } else { "FAIL" })) "status=$st2" 0
}

"SESSION_C=$sid" | Out-File (Join-Path $ev "session_c.txt") -Encoding ascii
