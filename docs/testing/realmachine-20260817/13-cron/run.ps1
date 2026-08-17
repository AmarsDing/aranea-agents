# 13-cron: task list / runs / detail / trigger(conditional)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "13"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Get "/v1/cron-tasks?page=1&page_size=20" -OutFile (Join-Path $ev "cron01-tasks.json")
$count = 0
try { $count = @($r.Body.items).Count } catch {}
Record $M "CRON-01" "cron task list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) tasks=$count" $r.Ms

$r = Api-Get "/v1/cron-task-runs?page=1&page_size=10" -OutFile (Join-Path $ev "cron02-runs.json")
$runs = 0
try { $runs = @($r.Body.items).Count } catch {}
Record $M "CRON-02" "cron task runs" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) runs=$runs" $r.Ms

# CRON-03: detail of first task
$tid = $null
try { $tid = (Api-Get "/v1/cron-tasks?page=1&page_size=1").Body.items[0].id } catch {}
if ($tid) {
    $r = Api-Get "/v1/cron-tasks/$tid" -OutFile (Join-Path $ev "cron03-detail.json")
    Record $M "CRON-03" "cron task detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) id=$tid" $r.Ms
} else {
    Record $M "CRON-03" "cron task detail" "SKIP" "no cron tasks" 0
}
