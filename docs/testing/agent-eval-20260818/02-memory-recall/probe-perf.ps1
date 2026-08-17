# Domain B/C recall perf probe (LLM-free, retrieval segment only)
# Usage: powershell -File probe-perf.ps1 [-Rounds 20] [-Tag baseline]
param([int]$Rounds = 20, [string]$Tag = "baseline")
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")

$evDir = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $evDir | Out-Null

$agentId = "a21265a2d8f24072fb638b50"   # eval_memory_probe
$userId  = "1"                           # dev user (110 facts: 75 fresh emb)
$collSem = "c271973f7530ebc788b2"        # UX lib bge-m3 (semantic layer)
$collLex = "a7310ebb25e82766f6e6"        # team inbox (lexical BM25 fallback)

$queries = Get-Content (Join-Path $PSScriptRoot "sample-perf-queries.json") -Raw -Encoding UTF8 | ConvertFrom-Json

function Measure-Endpoint {
    param([string]$Name, [string]$Method, [string]$Path, $BodyTemplate, [int]$N)
    $lat = @()
    $hits = @()
    $codes = @{}
    for ($i = 0; $i -lt $N; $i++) {
        $q = $queries[$i % $queries.Count]
        $body = $null
        if ($null -ne $BodyTemplate) {
            $qesc = ($q -replace '\\','\\') -replace '"','\"'
            $body = ($BodyTemplate | ConvertTo-Json -Depth 10 -Compress) -replace '__QUERY__', $qesc
        }
        $r = Api-Call -Method $Method -Path $Path -Body $body -TimeoutSec 60
        $lat += $r.Ms
        if ($codes.ContainsKey($r.Code)) { $codes[$r.Code]++ } else { $codes[$r.Code] = 1 }
        if ($r.Code -eq "200" -and $r.Body) {
            if ($r.Body.items) { $hits += $r.Body.items.Count }
            elseif ($r.Body.l3Hits -or $r.Body.l2Hits) { $hits += ([int]($r.Body.l3Hits.Count) + [int]($r.Body.l2Hits.Count)) }
            elseif ($r.Body.chunks) { $hits += $r.Body.chunks.Count }
        }
        Start-Sleep -Milliseconds 40
    }
    $sorted = $lat | Sort-Object
    $p95idx = [Math]::Min([Math]::Ceiling($N * 0.95) - 1, $N - 1)
    $p50idx = [Math]::Min([Math]::Ceiling($N * 0.50) - 1, $N - 1)
    $hitsAvg = 0
    if ($hits.Count -gt 0) { $hitsAvg = [Math]::Round(($hits | Measure-Object -Average).Average, 1) }
    $codeStr = ($codes.GetEnumerator() | ForEach-Object { "$($_.Key)x$($_.Value)" }) -join ","
    return [pscustomobject]@{
        name = $Name; n = $N
        min = $sorted[0]; p50 = $sorted[$p50idx]; p95 = $sorted[$p95idx]; max = $sorted[-1]
        avg = [Math]::Round(($lat | Measure-Object -Average).Average, 1)
        hits_avg = $hitsAvg
        codes = $codeStr
        raw = $lat
    }
}

$results = @()

# 1) memory composite recall (L2+L3, DB segment, no embed/LLM)
$results += Measure-Endpoint -Name "memory_composite_recall" -Method POST -Path "/v1/memory/search/composite" -N $Rounds -BodyTemplate @{ agent_id = $agentId; user_id = $userId; query = "__QUERY__"; limit = 10 }

# 2) memory recall debug (L2/L3 split, DB segment)
$results += Measure-Endpoint -Name "memory_recall_debug" -Method POST -Path "/v1/memory/recall/debug" -N $Rounds -BodyTemplate @{ agent_id = $agentId; user_id = $userId; query = "__QUERY__"; l2_limit = 5; l3_limit = 8 }

# 3) knowledge semantic search (bge-m3 collection, ollama embed inline)
$results += Measure-Endpoint -Name "knowledge_search_dense" -Method POST -Path "/v1/knowledge/search" -N $Rounds -BodyTemplate @{ collection_id = $collSem; query = "__QUERY__"; top_k = 5; hybrid_search = "dense" }

# 4) knowledge hybrid rrf
$results += Measure-Endpoint -Name "knowledge_search_rrf" -Method POST -Path "/v1/knowledge/search" -N $Rounds -BodyTemplate @{ collection_id = $collSem; query = "__QUERY__"; top_k = 5; hybrid_search = "rrf" }

# 5) knowledge lexical fallback (BM25/tsvector, no embed)
$results += Measure-Endpoint -Name "knowledge_search_lexical" -Method POST -Path "/v1/knowledge/search" -N $Rounds -BodyTemplate @{ collection_id = $collLex; query = "__QUERY__"; top_k = 5 }

# 6) federated search (no collection, all-lib routing)
$results += Measure-Endpoint -Name "knowledge_search_federated" -Method POST -Path "/v1/knowledge/search" -N $Rounds -BodyTemplate @{ query = "__QUERY__"; top_k = 5 }

$out = [pscustomobject]@{ tag = $Tag; at = (Get-Date).ToString("s"); rounds = $Rounds; results = $results }
$outFile = Join-Path $evDir "perf-$Tag.json"
$out | ConvertTo-Json -Depth 6 | Out-File -Encoding utf8 $outFile
$results | Select-Object name, n, min, p50, avg, p95, max, hits_avg, codes | Format-Table -AutoSize
Write-Host "saved: $outFile"
