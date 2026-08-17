# 08-tools run2: correct pagination (page_size)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "08"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Get "/v1/tools?page_size=500" -OutFile (Join-Path $ev "tool01b-list.json")
$items = @($r.Body.items)
Record $M "TOOL-01B" "tools list (page_size=500)" ($(if ($r.Code -eq "200" -and $items.Count -ge 100) { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$($items.Count) total=$($r.Body.total)" $r.Ms

$keys = $items | ForEach-Object { $_.toolKey }
$need = @("gns3_exec", "gns3_fault_inject", "gns3_fault_clear", "gns3_health_check", "twin_alarm_get", "twin_line_status")
$missing = @($need | Where-Object { $keys -notcontains $_ })
Record $M "TOOL-02B" "monitoring tool keys present" ($(if ($missing.Count -eq 0) { "PASS" } else { "FAIL" })) "missing=$($missing -join ',')" 0

$hr = @($items | Where-Object { $_.toolKey -in @("gns3_fault_inject","gns3_fault_clear") })
$hrConf = @($hr | Where-Object { $_.requiresConfirmation -eq $true })
Record $M "TOOL-03B" "high-risk tools requires_confirmation" ($(if ($hr.Count -eq 2 -and $hrConf.Count -eq 2) { "PASS" } else { "FAIL" })) "found=$($hr.Count) reqConf=$($hrConf.Count)" 0

$tid = ($items | Where-Object { $_.toolKey -eq "gns3_health_check" } | Select-Object -First 1).id
if ($tid) {
    $r = Api-Get "/v1/tools/$tid" -OutFile (Join-Path $ev "tool04b-detail.json")
    Record $M "TOOL-04B" "tool detail (gns3_health_check)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    $r = Api-Get "/v1/tools/$tid/agent-bindings" -OutFile (Join-Path $ev "tool07b-bindings.json")
    Record $M "TOOL-07B" "agent bindings" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    $r = Api-Post "/v1/tools/$tid/test" @{} -OutFile (Join-Path $ev "tool08b-test.json") -TimeoutSec 90
    Record $M "TOOL-08B" "live test gns3_health_check" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

# twin_alarm_get live test (TwinOps read path)
$tal = ($items | Where-Object { $_.toolKey -eq "twin_alarm_get" } | Select-Object -First 1).id
if ($tal) {
    $r = Api-Post "/v1/tools/$tal/test" @{} -OutFile (Join-Path $ev "tool09-twin-alarm.json") -TimeoutSec 90
    Record $M "TOOL-09" "live test twin_alarm_get (Twin API)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}
