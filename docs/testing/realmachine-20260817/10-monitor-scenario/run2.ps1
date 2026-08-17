# 10-monitor-scenario run2: grant tool policy then retest
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "10"
$ev = Join-Path $PSScriptRoot "evidence"

$twinRead = @("twin_alarm_query","twin_alarm_get","twin_alarm_rule_get","twin_line_status","twin_line_events","twin_line_probe","twin_device_get","twin_device_search","twin_device_metrics","twin_collector_status","twin_remediation_status","twin_inspection_query","gns3_health_check","gns3_exec")
$changeExec = $twinRead + @("gns3_fault_inject","gns3_fault_clear")

# MON-09 grant diagnosis agent twin read tools
$r = Api-Call -Method PUT -Path "/v1/agents/71096314087d86e2caa20488/tools/policy" -Body @{ tools_enabled = $true; profile = "minimal"; allow = $twinRead } -OutFile (Join-Path $ev "mon09-policy-diag.json")
Record $M "MON-09" "grant ops_fault_diagnosis twin read tools" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# MON-10 grant change-execution agent full monitor toolset
$r = Api-Call -Method PUT -Path "/v1/agents/90fb01daa4c14a1580d8c828/tools/policy" -Body @{ tools_enabled = $true; profile = "minimal"; allow = $changeExec } -OutFile (Join-Path $ev "mon10-policy-exec.json")
Record $M "MON-10" "grant ops_change_execution full toolset" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# MON-11 verify effective tools
$r = Api-Get "/v1/agents/71096314087d86e2caa20488/tools/effective" -OutFile (Join-Path $ev "mon11-effective.json")
$hasTwin = ($r.Raw -match "twin_alarm_query")
Record $M "MON-11" "effective tools include twin_alarm_query" ($(if ($r.Code -eq "200" -and $hasTwin) { "PASS" } else { "FAIL" })) "code=$($r.Code) hit=$hasTwin" $r.Ms
