$ErrorActionPreference = 'Stop'
$page = 1
$found = @()
while ($page -le 20) {
  $r = Invoke-RestMethod -Uri ("http://localhost:8000/v1/sessions?page=" + $page + "&page_size=50") -Method Get
  $items = $r.sessions
  if (-not $items) { $items = $r.items }
  if (-not $items -or $items.Count -eq 0) { break }
  foreach ($s in $items) {
    $title = ($s.title -replace "`r?`n", ' ')
    if ($title -match 'Gather updated market data') {
      $found += $s
      Write-Output ("MATCH id=" + $s.id)
      Write-Output ("  ratio=" + $s.contextUsedRatio + " max=" + $s.maxContextUsedRatio + " usedTok=" + $s.contextUsedTokens + " win=" + $s.lastContextWindowTokens + " status=" + $s.contextStatus + " sessStatus=" + $s.status)
      Write-Output ("  title=" + $title)
      Write-Output ("  agentId=" + $s.agentId + " updatedAt=" + $s.updatedAt)
    }
  }
  Write-Output ("page " + $page + " count=" + $items.Count)
  if ($items.Count -lt 50) { break }
  $page++
}
if ($found.Count -eq 0) { Write-Output "NOT FOUND in listed sessions" }
