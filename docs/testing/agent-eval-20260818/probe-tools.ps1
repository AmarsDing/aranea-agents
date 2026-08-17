$j = Get-Content (Join-Path $PSScriptRoot "probe-out\tools.json") -Raw -Encoding UTF8 | ConvertFrom-Json
"total=$($j.total) items=$(@($j.items).Count)"
@($j.items) | Where-Object { $_.key -match 'butler|governance|curate|distill|knowledge' } | ForEach-Object { "$($_.key) | enabled=$($_.enabled) | risk=$($_.riskLevel) | confirm=$($_.requiresConfirmation)" }
