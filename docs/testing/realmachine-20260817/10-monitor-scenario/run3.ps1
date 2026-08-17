# 10-monitor-scenario run3: retest closed loop after policy grant
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "10"
$ev = Join-Path $PSScriptRoot "evidence"

# Phase A retry: alarm query
$r = Api-Post "/v1/sessions" @{ agent_id = "71096314087d86e2caa20488"; title = "realmachine-monitor-A2"; owner_type = "agent" }
$sidA = $r.Body.id
$r = Api-Post "/v1/chat/messages" @{ session_id = $sidA; agent_key = "ops_fault_diagnosis"; content = "Call twin_alarm_query to list current active alarms, then summarize count and the top alarm briefly." } -OutFile (Join-Path $ev "mon12-send.json") -TimeoutSec 300
$replyLen = 0; if ($r.Body.agentMessage) { $replyLen = ($r.Body.agentMessage.content_markdown).Length }
Record $M "MON-12" "alarm query retry" ($(if ($r.Code -eq "200" -and $replyLen -gt 0) { "PASS" } else { "FAIL" })) "code=$($r.Code) replyLen=$replyLen" $r.Ms

$r = Api-Get "/v1/tools/runs?session_id=$sidA&page_size=20" -OutFile (Join-Path $ev "mon13-runs.json")
$runs = @($r.Body.items)
$twinRuns = @($runs | Where-Object { $_.toolKey -match 'twin_' })
$twinOk = @($twinRuns | Where-Object { $_.status -eq "success" })
Record $M "MON-13" "twin tool executed via agent" ($(if ($twinOk.Count -ge 1) { "PASS" } else { "FAIL" })) "runs=$($runs.Count) twin=$($twinRuns.Count) ok=$($twinOk.Count)" $r.Ms

# Phase B retry: fault inject with HITL
$r = Api-Post "/v1/sessions" @{ agent_id = "90fb01daa4c14a1580d8c828"; title = "realmachine-monitor-B2"; owner_type = "agent" }
$sidB = $r.Body.id
$r = Api-Post "/v1/chat/messages" @{ session_id = $sidB; agent_key = "ops_change_execution"; content = "Inject a fault on port eth1 using the gns3_fault_inject tool. If the platform asks for confirmation, return the pending confirmation activity." } -OutFile (Join-Path $ev "mon14-inject-req.json") -TimeoutSec 300
Record $M "MON-14" "fault inject request" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$actId = $null
if ($r.Body.pendingActivity) { $actId = $r.Body.pendingActivity.id }
if (-not $actId -and $r.Body.activities) { $actId = (@($r.Body.activities) | Where-Object { $_.status -eq "pending" } | Select-Object -First 1).id }
if (-not $actId) {
    $p = Api-Get "/v1/chat/pending?session_id=$sidB" -OutFile (Join-Path $ev "mon14b-pending.json")
    if ($p.Body.items) { $actId = (@($p.Body.items)[0]).id }
}
Record $M "MON-15" "HITL pending activity" ($(if ($actId) { "PASS" } else { "FAIL" })) "actId=$actId" 0

if ($actId) {
    $r = Api-Post "/v1/chat/activities/$actId/confirm" @{ approved = $true } -OutFile (Join-Path $ev "mon16-confirm.json") -TimeoutSec 300
    Record $M "MON-16" "HITL approve fault inject" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

Start-Sleep -Seconds 5
$r = Api-Get "/v1/tools/runs?session_id=$sidB&page_size=20" -OutFile (Join-Path $ev "mon17-runs.json")
$runsB = @($r.Body.items)
$inj = @($runsB | Where-Object { $_.toolKey -eq "gns3_fault_inject" })
$injOk = @($inj | Where-Object { $_.status -eq "success" })
Record $M "MON-17" "fault inject executed" ($(if ($injOk.Count -ge 1) { "PASS" } else { "FAIL" })) "inject=$($inj.Count) ok=$($injOk.Count) all=$($runsB.Count)" $r.Ms

"SESSION_B=$sidB" | Out-File (Join-Path $ev "session_b.txt") -Encoding ascii
