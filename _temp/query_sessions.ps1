$ErrorActionPreference = 'Stop'
$r = Invoke-RestMethod -Uri 'http://localhost:8000/v1/sessions?page=1&page_size=200' -Method Get
$items = $r.sessions
if (-not $items) { $items = $r.items }
if (-not $items) { $items = $r.data }
Write-Output ("total=" + $items.Count)
foreach ($s in $items) {
  $ratio = 0.0; $max = 0.0
  if ($null -ne $s.contextUsedRatio) { $ratio = [double]$s.contextUsedRatio }
  if ($null -ne $s.maxContextUsedRatio) { $max = [double]$s.maxContextUsedRatio }
  if ($ratio -gt 0.3 -or $max -gt 0.3) {
    $title = ($s.title -replace "`r?`n", ' ')
    if ($title.Length -gt 60) { $title = $title.Substring(0, 60) }
    Write-Output ("{0} | ratio={1:N3} max={2:N3} usedTok={3} win={4} | {5}" -f $s.id, $ratio, $max, $s.contextUsedTokens, $s.lastContextWindowTokens, $title)
  }
}
