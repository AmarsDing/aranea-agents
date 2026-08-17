# 08-tools / twinops connectivity real-machine test
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "08"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# TOOL-01 list all tools
$r = Api-Get "/v1/tools?limit=500" -OutFile (Join-Path $ev "tool01-list.json")
$items = @($r.Body.items)
Record $M "TOOL-01" "tools list" ($(if ($r.Code -eq "200" -and $items.Count -gt 0) { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$($items.Count)" $r.Ms

# key monitoring tools present?
$keys = $items | ForEach-Object { $_.toolKey }
$need = @("gns3_exec", "gns3_fault_inject", "gns3_fault_clear", "gns3_health_check", "twin_alarm_get", "twin_line_status")
$missing = @($need | Where-Object { $keys -notcontains $_ })
Record $M "TOOL-02" "monitoring tool keys present" ($(if ($missing.Count -eq 0) { "PASS" } else { "FAIL" })) "missing=$($missing -join ',')" 0

# TOOL-03 high-risk tools require confirmation
$hr = @($items | Where-Object { $_.toolKey -in @("gns3_fault_inject","gns3_fault_clear") })
$hrOk = ($hr.Count -eq 2 -and ($hr | Where-Object { $_.requiresConfirmation -eq $true }).Count -eq 2)
Record $M "TOOL-03" "high-risk tools requires_confirmation" ($(if ($hrOk) { "PASS" } else { "FAIL" })) "found=$($hr.Count) confirmed=$($hrOk)" 0

# TOOL-04 tool detail
$tid = ($items | Where-Object { $_.toolKey -eq "gns3_health_check" } | Select-Object -First 1).id
$r = Api-Get "/v1/tools/$tid" -OutFile (Join-Path $ev "tool04-detail.json")
Record $M "TOOL-04" "tool detail (gns3_health_check)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# TOOL-05 tool runs history
$r = Api-Get "/v1/tools/runs?page_size=5" -OutFile (Join-Path $ev "tool05-runs.json")
Record $M "TOOL-05" "tool runs history" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# TOOL-06 invocation audits
$r = Api-Get "/v1/tools/audits?page_size=5" -OutFile (Join-Path $ev "tool06-audits.json")
Record $M "TOOL-06" "invocation audits" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# TOOL-07 agent bindings for gns3_health_check
$r = Api-Get "/v1/tools/$tid/agent-bindings" -OutFile (Join-Path $ev "tool07-bindings.json")
Record $M "TOOL-07" "agent bindings" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# TOOL-08 live test twinops read-only tool via /v1/tools/{id}/test (gns3_health_check -> TwinMonitor connectivity)
$r = Api-Post "/v1/tools/$tid/test" @{} -OutFile (Join-Path $ev "tool08-test.json") -TimeoutSec 90
$ok = ($r.Code -eq "200") -and ($r.Raw -notmatch '"error"')
Record $M "TOOL-08" "live test gns3_health_check (TwinOps conn)" ($(if ($ok) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
