# 07-knowledge run3: ingest via source+content_base64
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "07"
$ev = Join-Path $PSScriptRoot "evidence"
$newCid = "1c87a93675155d3164c8"

$md = "# 告警处置手册`n`nGNS3 故障注入后必须调用 gns3_fault_clear 清除故障，再复核链路状态。"
$b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($md))
$r = Api-Post "/v1/knowledge/documents" @{ collection_id = $newCid; source = "realmachine-test-doc.md"; mime_type = "text/markdown"; content_base64 = $b64; organize_to_markdown = $false } -OutFile (Join-Path $ev "kb10b-ingest.json")
$did = $r.Body.id; if (-not $did) { $did = $r.Body.document.id }
Record $M "KB-10B" "ingest document (base64)" ($(if ($r.Code -eq "200" -and $did) { "PASS" } else { "FAIL" })) "code=$($r.Code) did=$did msg=$($r.Body.message)" $r.Ms

Start-Sleep -Seconds 3
$r = Api-Post "/v1/knowledge/search" @{ query = "故障清除"; collection_id = $newCid; limit = 5 } -OutFile (Join-Path $ev "kb11b-search-new.json")
$hit = ($r.Raw -match "fault_clear|fault")
Record $M "KB-11B" "search newly ingested doc" ($(if ($r.Code -eq "200" -and $hit) { "PASS" } else { "FAIL" })) "code=$($r.Code) hit=$hit len=$($r.Raw.Length)" $r.Ms

if ($did) {
    $r = Api-Get "/v1/knowledge/documents/$did/content" -OutFile (Join-Path $ev "kb12b-content.json")
    Record $M "KB-12B" "document content" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms
}
