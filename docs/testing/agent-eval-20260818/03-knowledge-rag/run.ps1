# Domain C: knowledge RAG evaluation (agent-eval-20260818)
# Flow: C-00 create collection -> C-02 ingest 5 docs -> poll indexed -> C-05/06/07 search 30 cases
#       -> C-12 hybrid comparison (dense/sparse/rrf) -> C-15 isolation -> C-17 cascade delete
# Usage: powershell -ExecutionPolicy Bypass -File run.ps1 [-Pilot] [-SkipCleanup]
param([switch]$Pilot, [switch]$SkipCleanup)

. (Join-Path (Join-Path $PSScriptRoot "..") "_lib.ps1")

$module = "03"
$scriptDir = $PSScriptRoot
$evDir = Join-Path $scriptDir "evidence"
New-Item -ItemType Directory -Force -Path $evDir | Out-Null
Renew-Token | Out-Null

$qa = Get-Content (Join-Path $scriptDir "sample-knowledge-qa.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($qa.cases)
$docNames = @($qa.docs)
if ($Pilot) { $docNames = $docNames[0..1]; $cases = @($cases | Where-Object { $_.source_doc -in $docNames }) }

$collName = "eval-ops-kb"
$embedModel = "bge-m3"

# C-00: idempotent create (drop stale first for clean baseline)
$existing = Api-Get -Path "/v1/knowledge/collections?limit=100"
$old = @($existing.Body.items) | Where-Object { $_.name -eq $collName } | Select-Object -First 1
if ($old) {
    Api-Delete -Path "/v1/knowledge/collections/$($old.id)" | Out-Null
    Write-Host "[C-00] deleted stale collection $($old.id)"
}
$rCreate = Api-Post -Path "/v1/knowledge/collections" -Body @{ name = $collName; description = "agent-eval-20260818 domain-C, cleanup after run"; embedding_model = $embedModel; vault_backend = "team" } -OutFile (Join-Path $evDir "c00-create-collection.json")
$collId = $rCreate.Body.id
if ($rCreate.Code -ne "200" -or -not $collId) { Write-Host "[FATAL] create collection failed: $($rCreate.Code) $($rCreate.Raw)"; exit 1 }
Record -Module $module -Id "C-00" -Name "create collection" -Result PASS -Detail "id=$collId embed=$embedModel" -Ms $rCreate.Ms

# BUG-C-01 workaround: CreateCollectionRequest has no dim field, server defaults 1536 but
# runtime embedder (bge-m3) outputs 1024 -> InsertChunks dimension mismatch.
# Correct the collection dim via DB to match the configured embedder.
$sqlDim = "UPDATE knowledge_collections SET dim=1024 WHERE id='$collId';"
$sqlDim | docker exec -i aranea-postgres psql -U postgres -d aranea 2>&1 | Write-Host
$dimRow = (docker exec -i aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT dim FROM knowledge_collections WHERE id='$collId';") | Select-Object -First 1
Record -Module $module -Id "C-00b" -Name "dim correct (BUG-C-01 workaround)" -Result $(if ($dimRow -eq "1024") { "PASS" } else { "FAIL" }) -Detail "dim=$dimRow"

# C-02/03: ingest docs then poll until indexed
$docIds = @{}
foreach ($doc in $docNames) {
    $path = Join-Path $scriptDir $doc
    $b64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes($path))
    $r = Api-Post -Path "/v1/knowledge/documents" -Body @{ collection_id = $collId; source = $doc; mime_type = "text/markdown"; content_base64 = $b64; chunk_strategy = "markdown" } -OutFile (Join-Path $evDir "c02-ingest-$doc.json")
    $docIds[$doc] = $r.Body.id
    $ok = $(if ($r.Code -eq "200" -and $r.Body.id) { "PASS" } else { "FAIL" })
    Record -Module $module -Id "C-02-$doc" -Name "ingest doc" -Result $ok -Detail "docId=$($r.Body.id) chunks=$($r.Body.chunkCount)" -Ms $r.Ms
}

$deadline = (Get-Date).AddSeconds(180)
$allIndexed = $false
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 3
    $rDocs = Api-Get -Path ("/v1/knowledge/documents?collection_id=" + $collId + "&limit=50")
    $items = @($rDocs.Body.items)
    $pending = @($items | Where-Object { $_.status -ne "indexed" -and $_.status -ne "error" })
    $errored = @($items | Where-Object { $_.status -eq "error" })
    if ($errored.Count -gt 0) { Record -Module $module -Id "C-03" -Name "index wait" -Result FAIL -Detail ("error: " + (($errored | ForEach-Object { $_.source + ":" + $_.errorMessage }) -join "; ")); break }
    if ($pending.Count -eq 0 -and $items.Count -ge $docNames.Count) { $allIndexed = $true; break }
}
$rDocs = Api-Get -Path ("/v1/knowledge/documents?collection_id=" + $collId + "&limit=50") -OutFile (Join-Path $evDir "c03-documents.json")
$docMap = @{}
foreach ($d in @($rDocs.Body.items)) { $docMap[$d.source] = $d.id }
$chunkTotal = (@($rDocs.Body.items) | Measure-Object -Property chunkCount -Sum).Sum
Record -Module $module -Id "C-03" -Name "index wait" -Result $(if ($allIndexed) { "PASS" } else { "FAIL" }) -Detail "indexed=$($docMap.Count)/$($docNames.Count) chunks=$chunkTotal"

# C-05/06/07: search 30 cases (hybrid=auto, top-5 must contain source_doc)
$stat = @{}; $latencies = @()
foreach ($c in $cases) {
    $wantDocId = $docMap[$c.source_doc]
    $r = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $collId; query = $c.question; top_k = 5; hybrid_search = "auto" } -OutFile (Join-Path $evDir "$($c.id)-search.json")
    $hit = $false
    foreach ($ch in @($r.Body.chunks)) { if ($ch.docId -eq $wantDocId) { $hit = $true; break } }
    $top = @($r.Body.chunks) | Select-Object -First 1
    $result = $(if ($hit) { "PASS" } else { "FAIL" })
    $grp = ($c.id -split '-')[1]
    if (-not $stat.ContainsKey($grp)) { $stat[$grp] = @{ pass = 0; fail = 0 } }
    if ($hit) { $stat[$grp].pass++ } else { $stat[$grp].fail++ }
    $latencies += $r.Ms
    Record -Module $module -Id $c.id -Name "search top5" -Result $result -Detail ("chunks=" + @($r.Body.chunks).Count + " topScore=" + $(if ($top) { $top.score } else { 'n/a' })) -Ms $r.Ms
}

# C-12: hybrid comparison (dense / sparse / rrf)
$hybridStat = @{}
foreach ($mode in @("dense", "sparse", "rrf")) {
    $hybridStat[$mode] = @{ pass = 0; fail = 0; ms = @() }
    foreach ($c in $cases) {
        $wantDocId = $docMap[$c.source_doc]
        $r = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $collId; query = $c.question; top_k = 5; hybrid_search = $mode }
        $hit = $false
        foreach ($ch in @($r.Body.chunks)) { if ($ch.docId -eq $wantDocId) { $hit = $true; break } }
        if ($hit) { $hybridStat[$mode].pass++ } else { $hybridStat[$mode].fail++ }
        $hybridStat[$mode].ms += $r.Ms
    }
    $avg = 0; if ($hybridStat[$mode].ms.Count -gt 0) { $avg = [int](($hybridStat[$mode].ms | Measure-Object -Average).Average) }
    Record -Module $module -Id "C-12-$mode" -Name "hybrid comparison" -Result INFO -Detail "hit=$($hybridStat[$mode].pass)/$(($hybridStat[$mode].pass + $hybridStat[$mode].fail)) avgMs=$avg"
}

# C-15: isolation (search against nonexistent collection must not leak our chunks; 404 also acceptable)
$rIso = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = "nonexistent-coll-id"; query = $cases[0].question; top_k = 5 }
$isoChunks = @($rIso.Body.chunks)
$isoLeak = $false
foreach ($ch in $isoChunks) { if ($ch.collectionId -eq $collId) { $isoLeak = $true; break } }
$isoOk = $(if (($rIso.Code -eq "404") -or (-not $isoLeak)) { "PASS" } else { "FAIL" })
Record -Module $module -Id "C-15" -Name "cross-collection isolation" -Result $isoOk -Detail "code=$($rIso.Code) chunks=$($isoChunks.Count) leak=$isoLeak" -Ms $rIso.Ms

# C-17: cascade delete (docs list must be empty or 404 after delete)
if (-not $SkipCleanup) {
    $rDel = Api-Delete -Path "/v1/knowledge/collections/$collId"
    $rAfter = Api-Get -Path ("/v1/knowledge/documents?collection_id=" + $collId + "&limit=50") -OutFile (Join-Path $evDir "c17-after-delete.json")
    $left = 0
    if ($rAfter.Code -eq "200") { $left = @($rAfter.Body.items).Count }
    Record -Module $module -Id "C-17" -Name "cascade delete" -Result $(if ($left -eq 0) { "PASS" } else { "FAIL" }) -Detail "delCode=$($rDel.Code) afterCode=$($rAfter.Code) docsLeft=$left"
}

# summary
Write-Host ""
Write-Host "=== group hit (hybrid=auto) ==="
foreach ($g in $stat.Keys) { $s = $stat[$g]; Write-Host ("{0}: {1}/{2}" -f $g, $s.pass, ($s.pass + $s.fail)) }
Write-Host ""
Write-Host "=== C-12 hybrid comparison ==="
foreach ($m in $hybridStat.Keys) { $s = $hybridStat[$m]; Write-Host ("{0}: {1}/{2}" -f $m, $s.pass, ($s.pass + $s.fail)) }
$sorted = $latencies | Sort-Object
$p95 = $sorted[[Math]::Min([int]($sorted.Count * 0.95), $sorted.Count - 1)]
Write-Host ""
Write-Host ("search latency: min={0}ms max={1}ms p95={2}ms n={3}" -f $sorted[0], $sorted[-1], $p95, $sorted.Count)
