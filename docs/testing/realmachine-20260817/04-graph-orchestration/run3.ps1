# 04-graph run3: use execution with real lineage/checkpoints
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "04"
$ev = Join-Path $PSScriptRoot "evidence"
$exId = "e5bbbe96-3003-43d4-9a13-205af6993a40"

$r = Api-Get "/v1/graph/executions/$exId/checkpoints" -OutFile (Join-Path $ev "graph09b-checkpoints.json")
$cnt = 0; if ($r.Body.items) { $cnt = @($r.Body.items).Count }
Record $M "GRAPH-09B" "checkpoint list (ckpt-enabled exec)" ($(if ($r.Code -eq "200" -and $cnt -ge 1) { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$cnt" $r.Ms

$cpid = $null
if ($cnt -ge 1) { $cpid = (@($r.Body.items)[0]).checkpoint_id }

$r = Api-Get "/v1/graph/executions/$exId/state-snapshot" -OutFile (Join-Path $ev "graph10b-snapshot.json")
Record $M "GRAPH-10B" "state snapshot (latest)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

if ($cpid) {
    $r = Api-Get "/v1/graph/executions/$exId/state-snapshot?checkpoint_id=$cpid" -OutFile (Join-Path $ev "graph10c-snapshot-byid.json")
    Record $M "GRAPH-10C" "state snapshot (by checkpoint_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

# time-travel by step index 0
$r = Api-Post "/v1/graph/executions/$exId/time-travel" -Body (@{ execution_id = $exId; step_index = 0 }) -OutFile (Join-Path $ev "graph13-timetravel.json")
Record $M "GRAPH-13" "time travel (step 0)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
