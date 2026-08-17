. (Join-Path $PSScriptRoot "_lib.ps1")
Renew-Token | Out-Null
$a = Api-Get "/v1/agents?page_size=500"
@($a.Body.items) | Where-Object { $_.agentKey -match 'memory|butler|__' } | ForEach-Object { "$($_.id) | $($_.agentKey) | $($_.displayName) | model=$($_.model)" }
