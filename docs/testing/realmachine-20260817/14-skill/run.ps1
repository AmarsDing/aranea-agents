# 14-skill: list/fs-health/runs/evolution/tags/health
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "14"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Get "/v1/skills?page=1&page_size=20" -OutFile (Join-Path $ev "skl01-list.json")
$scount = 0
try { $scount = @($r.Body.items).Count } catch {}
Record $M "SKL-01" "skill list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) skills=$scount" $r.Ms

$r = Api-Get "/v1/skills/filesystem-health" -OutFile (Join-Path $ev "skl02-fshealth.json")
Record $M "SKL-02" "filesystem health" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$r = Api-Get "/v1/skill-runs?page=1&page_size=5" -OutFile (Join-Path $ev "skl03-runs.json")
Record $M "SKL-03" "skill runs" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$r = Api-Get "/v1/skill-evolution/proposals?page=1&page_size=5" -OutFile (Join-Path $ev "skl04-proposals.json")
Record $M "SKL-04" "evolution proposals" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$r = Api-Get "/v1/skill-tags" -OutFile (Join-Path $ev "skl05-tags.json")
Record $M "SKL-05" "skill tags" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# SKL-06: first skill detail + health
$sid = $null
try { $sid = (Api-Get "/v1/skills?page=1&page_size=1").Body.items[0].id } catch {}
if ($sid) {
    $r = Api-Get "/v1/skills/$sid" -OutFile (Join-Path $ev "skl06-detail.json")
    Record $M "SKL-06" "skill detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) id=$sid" $r.Ms
    $r = Api-Get "/v1/skills/$sid/health" -OutFile (Join-Path $ev "skl07-health.json")
    Record $M "SKL-07" "skill health" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
} else {
    Record $M "SKL-06" "skill detail" "SKIP" "no skills" 0
    Record $M "SKL-07" "skill health" "SKIP" "no skills" 0
}
