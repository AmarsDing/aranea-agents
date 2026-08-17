# 04-graph 补充：执行详情/检查点/快照（用 TEAM-03 触发的图执行）
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "04"
$ev = Join-Path $PSScriptRoot "evidence"
$exId = "31e815d5-7182-4f49-a927-be366b71ef03"
$gid = "9f0ce9b2-9de6-4f1c-bee0-1d39ef0142b6"

$r = Api-Get "/v1/graphs/$gid/executions" -OutFile (Join-Path $ev "graph07-execs.json")
Record $M "GRAPH-07" "Graph 执行列表" ($(if ($r.Code -eq "200" -and @($r.Body.items).Count -ge 1) { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$(@($r.Body.items).Count)" $r.Ms

$r = Api-Get "/v1/graph/executions/$exId" -OutFile (Join-Path $ev "graph08-exec.json")
Record $M "GRAPH-08" "执行详情" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) status=$($r.Body.status)" $r.Ms

$r = Api-Get "/v1/graph/executions/$exId/checkpoints" -OutFile (Join-Path $ev "graph09-checkpoints.json")
Record $M "GRAPH-09" "检查点列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Get "/v1/graph/executions/$exId/state-snapshot" -OutFile (Join-Path $ev "graph10-snapshot.json")
Record $M "GRAPH-10" "状态快照" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Get "/v1/graph/executions/$exId/task-events" -OutFile (Join-Path $ev "graph11-events.json")
Record $M "GRAPH-11" "任务事件流" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# GRAPH-04b visualize 正常图复核
$r = Api-Get "/v1/graphs/a3608496-d68a-4f9e-b07b-396aecd31fef/visualize"
Record $M "GRAPH-04B" "visualize 复核(正常图)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
