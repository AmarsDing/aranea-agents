# 03-team-orchestration 真机测试
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "03"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
$teamId = "9755a723338ec796b02c36c9"  # V3 修复验证, sequential, 2 members

# TEAM-01 列表
$r = Api-Get "/v1/teams"
Record $M "TEAM-01" "Team 列表" ($(if ($r.Code -eq "200" -and $r.Body.total -gt 0) { "PASS" } else { "FAIL" })) "total=$($r.Body.total)" $r.Ms

# TEAM-02 详情
$r = Api-Get "/v1/teams/$teamId" -OutFile (Join-Path $ev "team02-detail.json")
Record $M "TEAM-02" "Team 详情" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# TEAM-03 run-test（真实 LLM，两个成员顺序执行）
$r = Api-Post "/v1/teams/$teamId/run-test" @{ content = "请用一句话自我介绍并说明你在团队中的职责。" } -OutFile (Join-Path $ev "team03-runtest.json") -TimeoutSec 600
$runId = ""; if ($r.Body.run) { $runId = $r.Body.run.id }
$replyLen = 0; if ($r.Body.reply) { $replyLen = $r.Body.reply.Length }
Record $M "TEAM-03" "Team run-test 真实执行" ($(if ($r.Code -eq "200" -and $runId) { "PASS" } else { "FAIL" })) "code=$($r.Code) run=$runId reply_len=$replyLen" $r.Ms

# TEAM-04 run 详情
if ($runId) {
    $r = Api-Get "/v1/team-runs/$runId" -OutFile (Join-Path $ev "team04-run.json")
    Record $M "TEAM-04" "TeamRun 详情" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) status=$($r.Body.status)" $r.Ms

    # TEAM-05 steps
    $r = Api-Get "/v1/team-runs/$runId/steps" -OutFile (Join-Path $ev "team05-steps.json")
    $sc = 0; if ($r.Body.items) { $sc = @($r.Body.items).Count }
    Record $M "TEAM-05" "TeamRun steps" ($(if ($r.Code -eq "200" -and $sc -ge 1) { "PASS" } else { "FAIL" })) "code=$($r.Code) steps=$sc" $r.Ms

    # TEAM-06 summary
    $r = Api-Get "/v1/team-runs/$runId/summary"
    Record $M "TEAM-06" "TeamRun summary" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    # TEAM-07 observatory
    $r = Api-Get "/v1/team-runs/$runId/observatory"
    Record $M "TEAM-07" "TeamRun observatory" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
}

# TEAM-08 team-runs 列表
$r = Api-Get "/v1/team-runs"
Record $M "TEAM-08" "team-runs 列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# TEAM-09 死信列表
$r = Api-Get "/v1/task-dead-letters"
Record $M "TEAM-09" "task-dead-letters" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# TEAM-10 compile-graph
$r = Api-Post "/v1/teams/$teamId/compile-graph" @{} -OutFile (Join-Path $ev "team10-compile.json")
Record $M "TEAM-10" "Team 编译为 Graph" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
