# A/B isolate reranker cost in knowledge search (interleaved rounds)
param([int]$N = 12)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$coll = "c271973f7530ebc788b2"
$q = "核心交换机巡检周期"

# warm up ollama embedder
curl.exe -s -o nul http://localhost:11434/api/embeddings -H "Content-Type: application/json" -d "@$PSScriptRoot\probe-embed-body.json" | Out-Null

$modes = @("false", "true", "unset")
$acc = @{ "false" = @(); "true" = @(); "unset" = @() }
for ($i = 0; $i -lt $N; $i++) {
    foreach ($m in $modes) {
        if ($m -eq "unset") {
            $body = @{ collection_id = $coll; query = $q; top_k = 5; hybrid_search = "dense" } | ConvertTo-Json -Compress
        } else {
            $useRerank = ($m -eq "true")
            $body = @{ collection_id = $coll; query = $q; top_k = 5; hybrid_search = "dense"; use_rerank = $useRerank } | ConvertTo-Json -Compress
        }
        $r = Api-Post -Path "/v1/knowledge/search" -Body $body
        $acc[$m] += $r.Ms
        Start-Sleep -Milliseconds 30
    }
}
foreach ($m in $modes) {
    $s = $acc[$m] | Sort-Object
    $avg = [Math]::Round(($acc[$m] | Measure-Object -Average).Average, 1)
    Write-Host ("use_rerank={0,-6} min={1} p50={2} avg={3} max={4}  raw=[{5}]" -f $m, $s[0], $s[[int]($N/2)], $avg, $s[-1], ($acc[$m] -join ","))
}
