$logFile = "F:\aranea-agents\logs\aranea-pipeline.log"
$cutoff = [datetime]"2026-07-21T16:23:49"

Write-Host "=== persist_failed entries (cutoff: $cutoff) ==="
Write-Host ""

$count = 0
$afterCutoff = 0
$beforeCutoff = 0

Get-Content $logFile | Where-Object { $_ -match 'persist_failed' } | ForEach-Object {
    $count++
    if ($_ -match '"ts"\s*:\s*"([^"]+)"') {
        $tsStr = $Matches[1]
        try { $ts = [datetime]::Parse($tsStr) } catch { $ts = $null }
        if ($ts) {
            $isAfter = $ts -gt $cutoff
            if ($isAfter) { $afterCutoff++ } else { $beforeCutoff++ }
            $tag = if ($isAfter) { "AFTER-FIX" } else { "pre-fix" }
            # Extract error
            $errSnippet = ""
            if ($_ -match '"error_chain":\["([^"]{0,150})') { $errSnippet = $Matches[1] }
            Write-Host "[$tag] $ts | $errSnippet"
        } else {
            Write-Host "[parse-fail] $tsStr"
        }
    } else {
        # Try alternative timestamp format
        if ($_ -match '"Timestamp"\s*:\s*"([^"]+)"') {
            $tsStr = $Matches[1]
            Write-Host "[alt-ts] $tsStr"
        } else {
            Write-Host "[no-ts] entry $count"
        }
    }
}

Write-Host ""
Write-Host "Total: $count | Before fix: $beforeCutoff | After fix: $afterCutoff"
