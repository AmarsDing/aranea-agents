# 12-usage-quota: overview/trends/top/quotas/budget-alerts/breakdown
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "12"
$ev = Join-Path $PSScriptRoot "evidence"

$gets = @(
    @("USG-01", "/v1/usage/overview", "usage overview"),
    @("USG-02", "/v1/usage/trends", "usage trends"),
    @("USG-03", "/v1/usage/top-models", "top models"),
    @("USG-04", "/v1/usage/top-agents", "top agents"),
    @("USG-05", "/v1/usage/events?page=1&page_size=5", "usage events"),
    @("USG-06", "/v1/usage/all-models-breakdown", "all models breakdown"),
    @("USG-07", "/v1/usage/context-budget-stats", "context budget stats"),
    @("USG-08", "/v1/usage/budget-alerts", "budget alerts list")
)
foreach ($g in $gets) {
    $r = Api-Get $g[1] -OutFile (Join-Path $ev ("{0}.json" -f $g[0].ToLower()))
    Record $M $g[0] $g[2] ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

# USG-09: quota read for workspace scope
$r = Api-Get "/v1/usage/quotas/workspace/default" -OutFile (Join-Path $ev "usg09-quota.json")
Record $M "USG-09" "quota read (workspace/default)" ($(if ($r.Code -eq "200" -or $r.Code -eq "404") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# USG-10: quota put + check + restore (write with cleanup)
$orig = $r.Raw
$r = Api-Call -Method PUT -Path "/v1/usage/quotas/workspace/default" -Body @{ daily_token_limit = 100000000; monthly_token_limit = 3000000000 } -OutFile (Join-Path $ev "usg10-quota-put.json")
Record $M "USG-10" "quota upsert" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
$r = Api-Get "/v1/usage/quotas/workspace/default/check" -OutFile (Join-Path $ev "usg11-quota-check.json")
Record $M "USG-11" "quota check" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
