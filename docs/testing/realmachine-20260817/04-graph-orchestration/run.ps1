# 04-graph-orchestration 真机测试
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "04"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# GRAPH-01 列表
$r = Api-Get "/v1/graphs" -OutFile (Join-Path $ev "graph01-list.json")
$gid = $null
if ($r.Body.items) { $gid = (@($r.Body.items)[0]).id }
Record $M "GRAPH-01" "Graph 列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) first=$gid" $r.Ms

if ($gid) {
    $r = Api-Get "/v1/graphs/$gid" -OutFile (Join-Path $ev "graph02-detail.json")
    Record $M "GRAPH-02" "Graph 详情" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    $r = Api-Post "/v1/graphs/$gid/validate" @{}
    Record $M "GRAPH-03" "Graph 校验" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) $($r.Raw)" $r.Ms

    $r = Api-Get "/v1/graphs/$gid/visualize" -OutFile (Join-Path $ev "graph04-viz.json")
    Record $M "GRAPH-04" "Graph 可视化数据" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    $r = Api-Get "/v1/graphs/$gid/versions"
    Record $M "GRAPH-05" "Graph 版本" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    $r = Api-Get "/v1/graphs/$gid/export" -OutFile (Join-Path $ev "graph06-export.json")
    Record $M "GRAPH-06" "Graph 导出" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    # GRAPH-07 执行列表
    $r = Api-Get "/v1/graphs/$gid/executions" -OutFile (Join-Path $ev "graph07-execs.json")
    $exId = $null
    if ($r.Body.items) { $exId = (@($r.Body.items)[0]).id }
    Record $M "GRAPH-07" "Graph 执行列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) exec=$exId" $r.Ms

    if ($exId) {
        $r = Api-Get "/v1/graph/executions/$exId" -OutFile (Join-Path $ev "graph08-exec.json")
        Record $M "GRAPH-08" "执行详情" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) status=$($r.Body.status)" $r.Ms

        $r = Api-Get "/v1/graph/executions/$exId/checkpoints" -OutFile (Join-Path $ev "graph09-checkpoints.json")
        Record $M "GRAPH-09" "检查点列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

        $r = Api-Get "/v1/graph/executions/$exId/state-snapshot"
        Record $M "GRAPH-10" "状态快照" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

        $r = Api-Get "/v1/graph/executions/$exId/task-events"
        Record $M "GRAPH-11" "任务事件流" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
    }
}

# GRAPH-12 模板列表
$r = Api-Get "/v1/graph/templates"
Record $M "GRAPH-12" "Graph 模板列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
