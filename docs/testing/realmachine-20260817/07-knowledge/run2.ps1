# 07-knowledge run2: create collection with container-valid root_path
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "07"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Post "/v1/knowledge/collections" @{ name = "realmachine-kb-test"; description = "20260817 real-machine test"; root_path = "/tmp/realmachine-kb-vault" } -OutFile (Join-Path $ev "kb09b-create.json")
$newCid = $r.Body.id; if (-not $newCid) { $newCid = $r.Body.collection.id }
Record $M "KB-09B" "create collection (root_path)" ($(if ($r.Code -eq "200" -and $newCid) { "PASS" } else { "FAIL" })) "code=$($r.Code) cid=$newCid msg=$($r.Body.message)" $r.Ms

if ($newCid) {
    $docBody = @{ collection_id = $newCid; title = "realmachine-test-doc"; content_markdown = "# 告警处置手册`n`nGNS3 故障注入后必须调用 gns3_fault_clear 清除故障，再复核链路状态。" }
    $r = Api-Post "/v1/knowledge/documents" $docBody -OutFile (Join-Path $ev "kb10-ingest.json")
    $did = $r.Body.id; if (-not $did) { $did = $r.Body.document.id }
    Record $M "KB-10" "ingest document" ($(if ($r.Code -eq "200" -and $did) { "PASS" } else { "FAIL" })) "code=$($r.Code) did=$did msg=$($r.Body.message)" $r.Ms

    Start-Sleep -Seconds 3
    $r = Api-Post "/v1/knowledge/search" @{ query = "故障清除"; collection_id = $newCid; limit = 5 } -OutFile (Join-Path $ev "kb11-search-new.json")
    $hit = ($r.Raw -match "fault_clear|fault")
    Record $M "KB-11" "search newly ingested doc" ($(if ($r.Code -eq "200" -and $hit) { "PASS" } else { "FAIL" })) "code=$($r.Code) hit=$hit len=$($r.Raw.Length)" $r.Ms

    if ($did) {
        $r = Api-Get "/v1/knowledge/documents/$did/content" -OutFile (Join-Path $ev "kb12-content.json")
        Record $M "KB-12" "document content" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
    }
}
