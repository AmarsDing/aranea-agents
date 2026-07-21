$ErrorActionPreference = 'Stop'
$s = Invoke-RestMethod -Uri 'http://localhost:8000/v1/sessions/25604a9c-a54f-4fb4-8820-53b325cf9c11' -Method Get
Write-Output ("id=" + $s.id)
Write-Output ("agentId=" + $s.agentId)
Write-Output ("teamId=" + $s.teamId)
Write-Output ("ownerType=" + $s.ownerType)
Write-Output ("dialogMode=" + $s.dialogMode)
Write-Output ("provider/model=" + $s.defaultProvider + "/" + $s.defaultModel)
Write-Output ("defaultCtxWin=" + $s.defaultContextWindowTokens + " lastCtxWin=" + $s.lastContextWindowTokens)
Write-Output ("usedTok=" + $s.contextUsedTokens + " ratio=" + $s.contextUsedRatio + " status=" + $s.contextStatus)
Write-Output ("msgCount=" + $s.messageCount + " runCount=" + $s.runCount + " toolCalls=" + $s.toolCallCount + " modelCalls=" + $s.modelCallCount)
Write-Output ("status=" + $s.status + " created=" + $s.createdAt + " updated=" + $s.updatedAt)
if ($s.agentId) {
  try {
    $a = Invoke-RestMethod -Uri ("http://localhost:8000/v1/agents/" + $s.agentId) -Method Get
    Write-Output ("agent.key=" + $a.agentKey + " model=" + $a.model + " ctxWindow=" + $a.contextWindow)
  } catch { Write-Output ("agent fetch failed: " + $_.Exception.Message) }
}
