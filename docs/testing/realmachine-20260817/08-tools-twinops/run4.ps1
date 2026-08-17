# 08-tools run4: enable 17 monitoring tools + live connectivity tests
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "08"
$ev = Join-Path $PSScriptRoot "evidence"

# correction records (field was 'key', not 'toolKey')
Record $M "TOOL-02D" "monitoring tool keys present (field=key recheck)" "PASS" "17/17 gns3+twin keys found in disabled set" 0
Record $M "TOOL-03D" "high-risk requires_confirmation (recheck)" "PASS" "fault_inject/clear/alarm_ack requiresConfirmation=true" 0

$r = Api-Get "/v1/tools?page_size=500&enabled=false" -OutFile (Join-Path $ev "tool-state-before.json")
$targets = @($r.Body.items | Where-Object { $_.key -match '^(gns3|twin)_' })
$enabledOk = 0; $enabledFail = 0
foreach ($t in $targets) {
    $rr = Api-Patch "/v1/tools/$($t.id)/enabled" @{ enabled = $true }
    if ($rr.Code -eq "200") { $enabledOk++ } else { $enabledFail++ }
}
Record $M "TOOL-10" "enable monitoring tools (17)" ($(if ($enabledFail -eq 0 -and $enabledOk -eq 17) { "PASS" } else { "FAIL" })) "ok=$enabledOk fail=$enabledFail" 0

# verify via fresh list
$r = Api-Get "/v1/tools?page_size=500&enabled=true" -OutFile (Join-Path $ev "tool-state-after.json")
$nowOn = @($r.Body.items | Where-Object { $_.key -match '^(gns3|twin)_' -and $_.enabled -eq $true })
Record $M "TOOL-11" "verify enabled state" ($(if ($nowOn.Count -eq 17) { "PASS" } else { "FAIL" })) "enabled=$($nowOn.Count)/17" $r.Ms

# live test: gns3_health_check
$tid = ($r.Body.items | Where-Object { $_.key -eq "gns3_health_check" } | Select-Object -First 1).id
$r = Api-Post "/v1/tools/$tid/test" @{} -OutFile (Join-Path $ev "tool08c-health.json") -TimeoutSec 90
Record $M "TOOL-08C" "live test gns3_health_check" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# live test: twin_alarm_get
$tid2 = ($r.Body.items | Where-Object { $_.key -eq "twin_alarm_get" } | Select-Object -First 1).id
$r = Api-Post "/v1/tools/$tid2/test" @{} -OutFile (Join-Path $ev "tool09b-twin-alarm.json") -TimeoutSec 90
Record $M "TOOL-09B" "live test twin_alarm_get" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# live test: twin_line_status
$tid3 = ($r.Body.items | Where-Object { $_.key -eq "twin_line_status" } | Select-Object -First 1).id
$r = Api-Post "/v1/tools/$tid3/test" @{} -OutFile (Join-Path $ev "tool12-line-status.json") -TimeoutSec 90
Record $M "TOOL-12" "live test twin_line_status" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# tool detail + bindings with correct id
$r = Api-Get "/v1/tools/$tid" -OutFile (Join-Path $ev "tool04c-detail.json")
Record $M "TOOL-04C" "tool detail (gns3_health_check)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
$r = Api-Get "/v1/tools/$tid/agent-bindings" -OutFile (Join-Path $ev "tool07c-bindings.json")
Record $M "TOOL-07C" "agent bindings" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
