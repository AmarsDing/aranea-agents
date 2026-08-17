# 06-memory run3: fix fact upsert fields
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "06"
$ev = Join-Path $PSScriptRoot "evidence"
$ag = "agent___spirit__"

$r = Api-Post "/v1/memory/l3/facts" @{ fact = @{ scope_type = "agent"; scope_id = $ag; agent_id = $ag; statement = "realmachine 20260817 test fact: aranea memory write path OK"; fact_kind = "observation"; confidence = 0.9; tags_json = '["realmachine","test"]' } } -OutFile (Join-Path $ev "mem11c-upsert.json")
$fid = $r.Body.fact.id; if (-not $fid) { $fid = $r.Body.id }
Record $M "MEM-11C" "upsert L3 fact (scope_type+statement)" ($(if ($r.Code -eq "200" -and $fid) { "PASS" } else { "FAIL" })) "code=$($r.Code) fid=$fid msg=$($r.Body.message)" $r.Ms

# verify via list
$r = Api-Get "/v1/memory/l3/facts?agent_id=$ag&page_size=5" -OutFile (Join-Path $ev "mem11d-verify.json")
$hit = ($r.Raw -match "realmachine 20260817")
Record $M "MEM-11D" "fact list verify" ($(if ($r.Code -eq "200" -and $hit) { "PASS" } else { "FAIL" })) "code=$($r.Code) hit=$hit" $r.Ms

# review action: confirm the fact
if ($fid) {
    $r = Api-Post "/v1/memory/l3/facts/$fid/review" @{ fact_id = $fid; action = "confirm" } -OutFile (Join-Path $ev "mem17-review.json")
    Record $M "MEM-17" "fact review (confirm)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) msg=$($r.Body.message)" $r.Ms
}
