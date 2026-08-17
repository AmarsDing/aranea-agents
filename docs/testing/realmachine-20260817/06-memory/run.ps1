# 06-memory five-layer memory real-machine test
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "06"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
$sid = "6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8"   # 05-spirit session

# MEM-01 layer overview
$r = Api-Get "/v1/memory/layer-overview" -OutFile (Join-Path $ev "mem01-overview.json")
Record $M "MEM-01" "layer overview" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-02 L0 snapshots
$r = Api-Get "/v1/sessions/$sid/l0/snapshots" -OutFile (Join-Path $ev "mem02-l0.json")
Record $M "MEM-02" "L0 snapshots" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-03 L1 tasks
$r = Api-Get "/v1/sessions/$sid/l1/tasks" -OutFile (Join-Path $ev "mem03-l1.json")
Record $M "MEM-03" "L1 tasks" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-04 L3 facts list
$r = Api-Get "/v1/memory/l3/facts?page_size=5" -OutFile (Join-Path $ev "mem04-facts.json")
Record $M "MEM-04" "L3 facts list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-05 L3 conflicts
$r = Api-Get "/v1/memory/l3/facts/conflicts" -OutFile (Join-Path $ev "mem05-conflicts.json")
Record $M "MEM-05" "L3 fact conflicts" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-06 L4 entities
$r = Api-Get "/v1/memory/l4/entities?page_size=5" -OutFile (Join-Path $ev "mem06-l4.json")
Record $M "MEM-06" "L4 entities" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-07 episodes
$r = Api-Get "/v1/memory/episodes?page_size=5" -OutFile (Join-Path $ev "mem07-episodes.json")
Record $M "MEM-07" "episodes (L2)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-08 worker status
$r = Api-Get "/v1/memory/worker/status" -OutFile (Join-Path $ev "mem08-worker.json")
Record $M "MEM-08" "memory worker status" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-09 dead letters
$r = Api-Get "/v1/memory/worker/dead-letters" -OutFile (Join-Path $ev "mem09-deadletters.json")
Record $M "MEM-09" "worker dead letters" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-10 platform settings
$r = Api-Get "/v1/memory/platform/settings" -OutFile (Join-Path $ev "mem10-settings.json")
Record $M "MEM-10" "platform settings" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-11 upsert fact + verify
$factKey = "realmachine_test_" + (Get-Date -Format "HHmmss")
$r = Api-Post "/v1/memory/l3/facts" @{ fact_key = $factKey; content = "realmachine 20260817 test fact"; category = "test"; confidence = 0.9 } -OutFile (Join-Path $ev "mem11-upsert.json")
$fid = $r.Body.fact.id; if (-not $fid) { $fid = $r.Body.id }
Record $M "MEM-11" "upsert L3 fact" ($(if ($r.Code -eq "200" -and $fid) { "PASS" } else { "FAIL" })) "code=$($r.Code) fid=$fid" $r.Ms

# MEM-12 recall debug
$r = Api-Post "/v1/memory/recall/debug" @{ query = "realmachine test fact"; session_id = $sid } -OutFile (Join-Path $ev "mem12-recall.json")
Record $M "MEM-12" "recall debug" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-13 composite search
$r = Api-Post "/v1/memory/search/composite" @{ query = "realmachine"; limit = 5 } -OutFile (Join-Path $ev "mem13-composite.json")
Record $M "MEM-13" "composite search" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-14 agent identity
$r = Api-Get "/v1/agents/agent___spirit__/identity" -OutFile (Join-Path $ev "mem14-identity.json")
Record $M "MEM-14" "agent identity" ($(if ($r.Code -in @("200","404")) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-15 unified graph
$r = Api-Get "/v1/memory/graph/unified" -OutFile (Join-Path $ev "mem15-graph.json")
Record $M "MEM-15" "unified memory graph" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# MEM-16 cascade proposals
$r = Api-Get "/v1/memory/cascade/proposals" -OutFile (Join-Path $ev "mem16-cascade.json")
Record $M "MEM-16" "cascade proposals" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
