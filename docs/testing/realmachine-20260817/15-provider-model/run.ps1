# 15-provider-model: llm-provider-models + model-catalog
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "15"
$ev = Join-Path $PSScriptRoot "evidence"

$r = Api-Get "/v1/llm-provider-models?page=1&page_size=20" -OutFile (Join-Path $ev "prv01-models.json")
$pcount = 0
try { $pcount = @($r.Body.items).Count } catch {}
Record $M "PRV-01" "llm provider models list" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) providers=$pcount" $r.Ms

$r = Api-Get "/v1/model-catalog/status" -OutFile (Join-Path $ev "prv02-status.json")
Record $M "PRV-02" "model catalog status" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

$r = Api-Get "/v1/model-catalog/providers" -OutFile (Join-Path $ev "prv03-catalog-providers.json")
$ccount = 0
try { $ccount = @($r.Body.items).Count } catch {}
Record $M "PRV-03" "catalog providers" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) providers=$ccount" $r.Ms

# PRV-04: models of first catalog provider (avoid $pid - read-only automatic var)
$provId = $null
try { $provId = (Api-Get "/v1/model-catalog/providers").Body.items[0].id } catch {}
if (-not $provId) { try { $provId = (Api-Get "/v1/model-catalog/providers").Body.items[0].providerId } catch {} }
if ($provId) {
    $r = Api-Get "/v1/model-catalog/providers/$provId/models" -OutFile (Join-Path $ev "prv04-catalog-models.json")
    $mcount = 0
    try { $mcount = @($r.Body.items).Count } catch {}
    Record $M "PRV-04" "catalog provider models" ($(if ($r.Code -eq "200" -and $mcount -gt 0) { "PASS" } else { "FAIL" })) "code=$($r.Code) pid=$provId models=$mcount" $r.Ms
} else {
    Record $M "PRV-04" "catalog provider models" "SKIP" "no catalog providers" 0
}

# PRV-05: provider model detail (first llm provider model)
$pmid = $null
try { $pmid = (Api-Get "/v1/llm-provider-models?page=1&page_size=1").Body.items[0].id } catch {}
if ($pmid) {
    $r = Api-Get "/v1/llm-provider-models/$pmid" -OutFile (Join-Path $ev "prv05-detail.json")
    Record $M "PRV-05" "provider model detail" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) id=$pmid" $r.Ms
} else {
    Record $M "PRV-05" "provider model detail" "SKIP" "no provider models" 0
}

$r = Api-Get "/v1/model-catalog/sync-logs?page=1&page_size=5" -OutFile (Join-Path $ev "prv06-synclogs.json")
Record $M "PRV-06" "catalog sync logs" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
