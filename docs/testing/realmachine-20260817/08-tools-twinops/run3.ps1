# 08-tools run3: include disabled tools, verify monitoring toolset state
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "08"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Get "/v1/tools?page_size=500&enabled=false" -OutFile (Join-Path $ev "tool01c-disabled.json")
$items = @($r.Body.items)
Record $M "TOOL-01C" "tools list (enabled=false)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$($items.Count)" $r.Ms

$keys = $items | ForEach-Object { $_.toolKey }
$need = @("gns3_exec", "gns3_fault_inject", "gns3_fault_clear", "gns3_health_check", "twin_alarm_get", "twin_line_status")
$missing = @($need | Where-Object { $keys -notcontains $_ })
Record $M "TOOL-02C" "monitoring tools in disabled set" ($(if ($missing.Count -eq 0) { "PASS" } else { "FAIL" })) "missing=$($missing -join ',')" 0

$hr = @($items | Where-Object { $_.toolKey -in @("gns3_fault_inject","gns3_fault_clear") })
$hrConf = @($hr | Where-Object { $_.requiresConfirmation -eq $true })
Record $M "TOOL-03C" "high-risk requires_confirmation (DB seed)" ($(if ($hr.Count -eq 2 -and $hrConf.Count -eq 2) { "PASS" } else { "FAIL" })) "found=$($hr.Count) reqConf=$($hrConf.Count)" 0
