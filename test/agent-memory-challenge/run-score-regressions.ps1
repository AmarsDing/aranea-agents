param(
    [string]$TestDsn = $env:ARANEA_TEST_PG_DSN,
    [switch]$WithRace
)

$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$taskContainer = $null
$containerStarted = $false
$originalDsn = $env:ARANEA_TEST_PG_DSN
$originalCache = $env:GOCACHE
Push-Location $taskRoot
try {
    if (-not $TestDsn) {
        # Own disposable database; never inspect existing container credentials.
        $taskContainer = "aranea-amc-regression-$PID-$([Guid]::NewGuid().ToString('N').Substring(0, 8))"
        $testPassword = [Guid]::NewGuid().ToString('N')
        docker run --rm --detach --name $taskContainer --publish '127.0.0.1::5432' --env "POSTGRES_PASSWORD=$testPassword" --env 'POSTGRES_DB=aranea_test' pgvector/pgvector:pg16 | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Could not start the disposable test database' }
        $containerStarted = $true
        $binding = docker port $taskContainer '5432/tcp'
        $testPort = ($binding | Select-Object -First 1).Split(':')[-1]
        $TestDsn = "postgres://postgres:${testPassword}@127.0.0.1:${testPort}/aranea_test?sslmode=disable"
        $ready = $false
        for ($attempt = 0; $attempt -lt 40; $attempt++) {
            docker exec $taskContainer pg_isready -U postgres -d aranea_test *> $null
            if ($LASTEXITCODE -eq 0) { $ready = $true; break }
            Start-Sleep -Milliseconds 500
        }
        if (-not $ready) { throw 'Disposable test database did not become ready' }
    }
    $testUri = [Uri]$TestDsn
    if ($testUri.AbsolutePath -ne '/aranea_test') { throw 'Only the dedicated aranea_test database is allowed' }
    $env:ARANEA_TEST_PG_DSN = $TestDsn
    $env:GOCACHE = 'F:\gocache'
    $goFlags = @('-json', '-count=1')
    if ($WithRace) { $goFlags += '-race' }

    go test @goFlags ./cmd/memoryeval | Tee-Object -FilePath (Join-Path $PSScriptRoot 'contract-results.jsonl')
    if ($LASTEXITCODE -ne 0) { throw 'HTTP contract regression failed' }

    $domainCases = 'TestClassifyEvalFactKind|TestShouldSupersedeEvalFact|TestSplitEval|TestAnalyzeEval|TestResolveEval|TestEvalSupersession|TestEvalCorrectionTargetsFact|TestEvalMemory|TestEvalContext'
    go test @goFlags ./internal/biz -run $domainCases | Tee-Object -FilePath (Join-Path $PSScriptRoot 'domain-results.jsonl')
    if ($LASTEXITCODE -ne 0) { throw 'Memory domain regression failed' }

    $dataCases = 'TestEvalStore|TestEvalEvidence|TestMaskEval|TestFilterEval|TestEvalMetadata|TestEvalItem|TestHasEval'
    go test @goFlags ./internal/data -run $dataCases | Tee-Object -FilePath (Join-Path $PSScriptRoot 'store-results.jsonl')
    if ($LASTEXITCODE -ne 0) { throw 'Memory store regression failed' }
} finally {
    $env:ARANEA_TEST_PG_DSN = $originalDsn
    $env:GOCACHE = $originalCache
    if ($containerStarted) {
        docker stop $taskContainer | Out-Null
        if ($LASTEXITCODE -ne 0) { Write-Warning "Disposable database cleanup needs attention: $taskContainer" }
    }
    Pop-Location
}
