# 10-monitor-scenario: alarm->diagnosis->fault inject/clear closed loop (real LLM + HITL)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "10"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# --- Phase A: alarm query via ops_fault_diagnosis ---
$r = Api-Post "/v1/sessions" @{ agent_id = "71096314087d86e2caa20488"; title = "realmachine-monitor-A"; owner_type = "agent" } -OutFile (Join-Path $ev "mon01-session.json")
$sidA = $r.Body.id
Record $M "MON-01" "create diagnosis session" ($(if ($sidA) { "PASS" } else { "FAIL" })) "sid=$sidA" $r.Ms

$r = Api-Post "/v1/chat/messages" @{ session_id = $sidA; agent_key = "ops_fault_diagnosis"; content = "Call twin_alarm_query to list current active alarms, then summarize count and top alarm in one sentence." } -OutFile (Join-Path $ev "mon02-send.json") -TimeoutSec 300
$replyLen = 0; if ($r.Body.agentMessage) { $replyLen = ($r.Body.agentMessage.content_markdown).Length }
Record $M "MON-02" "alarm query via agent (real tool call)" ($(if ($r.Code -eq "200" -and $replyLen -gt 0) { "PASS" } else { "FAIL" })) "code=$($r.Code) replyLen=$replyLen" $r.Ms

$r = Api-Get "/v1/tools/runs?session_id=$sidA&page_size=20" -OutFile (Join-Path $ev "mon03-runs.json")
$runs = @($r.Body.items)
$twinRuns = @($runs | Where-Object { $_.toolKey -match 'twin_' -or $_.tool_key -match 'twin_' })
Record $M "MON-03" "tool runs evidence (twin calls)" ($(if ($twinRuns.Count -ge 1) { "PASS" } else { "FAIL" })) "total=$($runs.Count) twin=$($twinRuns.Count)" $r.Ms

# --- Phase B: fault inject + clear via ops_change_execution (HITL) ---
$r = Api-Post "/v1/sessions" @{ agent_id = "90fb01daa4c14a1580d8c828"; title = "realmachine-monitor-B"; owner_type = "agent" } -OutFile (Join-Path $ev "mon04-session.json")
$sidB = $r.Body.id
Record $M "MON-04" "create change-execution session" ($(if ($sidB) { "PASS" } else { "FAIL" })) "sid=$sidB" $r.Ms

$r = Api-Post "/v1/chat/messages" @{ session_id = $sidB; agent_key = "ops_change_execution"; content = "Inject a fault on port eth1 using gns3_fault_inject. If confirmation is required, request it." } -OutFile (Join-Path $ev "mon05-inject-req.json") -TimeoutSec 300
$raw = $r.Raw
Record $M "MON-05" "fault inject request (expect HITL)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($raw.Length)" $r.Ms

# find pending confirmation activity
$actId = $null
if ($r.Body.pendingActivity) { $actId = $r.Body.pendingActivity.id }
if (-not $actId -and $r.Body.activities) { $actId = (@($r.Body.activities) | Where-Object { $_.status -eq "pending" } | Select-Object -First 1).id }
if (-not $actId) {
    $p = Api-Get "/v1/chat/pending?session_id=$sidB"
    if ($p.Body.items) { $actId = (@($p.Body.items)[0]).id }
    [IO.File]::WriteAllText((Join-Path $ev "mon05b-pending.json"), $p.Raw, [Text.UTF8Encoding]::new($false))
}
Record $M "MON-06" "HITL pending activity found" ($(if ($actId) { "PASS" } else { "FAIL" })) "actId=$actId" 0

if ($actId) {
    $r = Api-Post "/v1/chat/activities/$actId/confirm" @{ approved = $true } -OutFile (Join-Path $ev "mon07-confirm.json") -TimeoutSec 300
    Record $M "MON-07" "confirm fault inject (HITL approve)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

Start-Sleep -Seconds 5
$r = Api-Get "/v1/tools/runs?session_id=$sidB&page_size=20" -OutFile (Join-Path $ev "mon08-runs.json")
$runsB = @($r.Body.items)
$inj = @($runsB | Where-Object { ($_.toolKey -eq "gns3_fault_inject" -or $_.tool_key -eq "gns3_fault_inject") })
$injOk = @($inj | Where-Object { $_.status -eq "success" -or $_.status -eq "ok" })
Record $M "MON-08" "fault inject executed" ($(if ($injOk.Count -ge 1) { "PASS" } elseif ($inj.Count -ge 1) { "FAIL" } else { "FAIL" })) "injectRuns=$($inj.Count) ok=$($injOk.Count) status=$(@($inj | Select-Object -First 1).status)" $r.Ms
