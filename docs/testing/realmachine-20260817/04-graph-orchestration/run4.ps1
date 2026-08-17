# 04-graph run4: fix camelCase field + time-travel with step_index=1
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "04"
$ev = Join-Path $PSScriptRoot "evidence"
$exId = "e5bbbe96-3003-43d4-9a13-205af6993a40"
$cpid = "7763b4fd-4a33-4681-a4ff-c6f425d3a0bb"

$r = Api-Get "/v1/graph/executions/$exId/state-snapshot?checkpoint_id=$cpid" -OutFile (Join-Path $ev "graph10c-snapshot-byid.json")
Record $M "GRAPH-10C" "state snapshot (by checkpoint_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Post "/v1/graph/executions/$exId/time-travel" -Body (@{ execution_id = $exId; step_index = 1 }) -OutFile (Join-Path $ev "graph13b-timetravel.json")
Record $M "GRAPH-13B" "time travel (step 1)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
