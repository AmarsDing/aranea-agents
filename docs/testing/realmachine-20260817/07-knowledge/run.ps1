# 07-knowledge real-machine test
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "07"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# KB-01 collections list
$r = Api-Get "/v1/knowledge/collections" -OutFile (Join-Path $ev "kb01-collections.json")
$cid = $null
if ($r.Body.items) { $cid = (@($r.Body.items)[0]).id }
Record $M "KB-01" "collections list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) count=$(@($r.Body.items).Count) first=$cid" $r.Ms

if ($cid) {
    # KB-02 collection detail
    $r = Api-Get "/v1/knowledge/collections/$cid" -OutFile (Join-Path $ev "kb02-detail.json")
    Record $M "KB-02" "collection detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

    # KB-03 vault tree
    $r = Api-Get "/v1/knowledge/vaults/$cid/tree" -OutFile (Join-Path $ev "kb03-tree.json")
    Record $M "KB-03" "vault tree" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    # KB-04 documents list
    $r = Api-Get "/v1/knowledge/documents?collection_id=$cid&page_size=5" -OutFile (Join-Path $ev "kb04-docs.json")
    Record $M "KB-04" "documents list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

    # KB-05 collection graph
    $r = Api-Get "/v1/knowledge/vaults/$cid/graph" -OutFile (Join-Path $ev "kb05-graph.json")
    Record $M "KB-05" "collection graph" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}

# KB-06 search
$r = Api-Post "/v1/knowledge/search" @{ query = "告警"; limit = 5 } -OutFile (Join-Path $ev "kb06-search.json")
Record $M "KB-06" "knowledge search" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# KB-07 embedder config
$r = Api-Get "/v1/knowledge/embedder-config" -OutFile (Join-Path $ev "kb07-embedder.json")
Record $M "KB-07" "embedder config" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# KB-08 governance proposals
$r = Api-Get "/v1/knowledge/governance-proposals" -OutFile (Join-Path $ev "kb08-governance.json")
Record $M "KB-08" "governance proposals" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# KB-09 create collection + ingest document + search (write path closed loop)
$r = Api-Post "/v1/knowledge/collections" @{ name = "realmachine-kb-test"; description = "20260817 real-machine test" } -OutFile (Join-Path $ev "kb09-create.json")
$newCid = $r.Body.id; if (-not $newCid) { $newCid = $r.Body.collection.id }
Record $M "KB-09" "create collection" ($(if ($r.Code -eq "200" -and $newCid) { "PASS" } else { "FAIL" })) "code=$($r.Code) cid=$newCid msg=$($r.Body.message)" $r.Ms

if ($newCid) {
    $docBody = @{ collection_id = $newCid; title = "realmachine-test-doc"; content_markdown = "# 告警处置手册`n`nGNS3 故障注入后必须调用 gns3_fault_clear 清除故障，再复核链路状态。" }
    $r = Api-Post "/v1/knowledge/documents" $docBody -OutFile (Join-Path $ev "kb10-ingest.json")
    $did = $r.Body.id; if (-not $did) { $did = $r.Body.document.id }
    Record $M "KB-10" "ingest document" ($(if ($r.Code -eq "200" -and $did) { "PASS" } else { "FAIL" })) "code=$($r.Code) did=$did msg=$($r.Body.message)" $r.Ms

    Start-Sleep -Seconds 3
    $r = Api-Post "/v1/knowledge/search" @{ query = "故障清除"; collection_id = $newCid; limit = 5 } -OutFile (Join-Path $ev "kb11-search-new.json")
    $hit = ($r.Raw -match "fault_clear|告警|故障")
    Record $M "KB-11" "search newly ingested doc" ($(if ($r.Code -eq "200" -and $hit) { "PASS" } else { "FAIL" })) "code=$($r.Code) hit=$hit len=$($r.Raw.Length)" $r.Ms

    if ($did) {
        $r = Api-Get "/v1/knowledge/documents/$did/content" -OutFile (Join-Path $ev "kb12-content.json")
        Record $M "KB-12" "document content" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
    }
}
