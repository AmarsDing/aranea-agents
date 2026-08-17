# 06-memory run2: fix required params (agent_id / fact wrapper)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "06"
$ev = Join-Path $PSScriptRoot "evidence"
$ag = "agent___spirit__"

$r = Api-Get "/v1/memory/layer-overview?agent_id=$ag" -OutFile (Join-Path $ev "mem01b-overview.json")
Record $M "MEM-01B" "layer overview (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Get "/v1/memory/l3/facts/conflicts?agent_id=$ag" -OutFile (Join-Path $ev "mem05b-conflicts.json")
Record $M "MEM-05B" "L3 conflicts (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Get "/v1/memory/episodes?agent_id=$ag&page_size=5" -OutFile (Join-Path $ev "mem07b-episodes.json")
Record $M "MEM-07B" "episodes (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$factKey = "realmachine_test_" + (Get-Date -Format "HHmmss")
$r = Api-Post "/v1/memory/l3/facts" @{ fact = @{ fact_key = $factKey; content = "realmachine 20260817 test fact"; category = "test"; confidence = 0.9 }; agent_id = $ag } -OutFile (Join-Path $ev "mem11b-upsert.json")
$fid = $r.Body.fact.id; if (-not $fid) { $fid = $r.Body.id }
Record $M "MEM-11B" "upsert L3 fact (fact wrapper)" ($(if ($r.Code -eq "200" -and $fid) { "PASS" } else { "FAIL" })) "code=$($r.Code) fid=$fid" $r.Ms

$r = Api-Post "/v1/memory/recall/debug" @{ query = "realmachine test fact"; agent_id = $ag } -OutFile (Join-Path $ev "mem12b-recall.json")
Record $M "MEM-12B" "recall debug (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Post "/v1/memory/search/composite" @{ query = "realmachine"; agent_id = $ag; limit = 5 } -OutFile (Join-Path $ev "mem13b-composite.json")
Record $M "MEM-13B" "composite search (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Get "/v1/memory/graph/unified?agent_id=$ag" -OutFile (Join-Path $ev "mem15b-graph.json")
Record $M "MEM-15B" "unified graph (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

$r = Api-Get "/v1/memory/cascade/proposals?agent_id=$ag" -OutFile (Join-Path $ev "mem16b-cascade.json")
Record $M "MEM-16B" "cascade proposals (agent_id)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
