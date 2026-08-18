# Repro: evalstaletest01 (5 semantic edges, closed_ratio=0.8, no access log) -> curate stale mark
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
Renew-Token | Out-Null
$inboxId = "a7310ebb25e82766f6e6"
$butlerId = "agent___memory__"
$rButlerSess = Api-Post "/v1/sessions" @{ agent_id = $butlerId; title = "repro-stale-curate"; owner_type = "agent" }
$bsid = $rButlerSess.Body.id
Write-Host "session=$bsid"
$msg = 'Please call the memory_butler_knowledge_curate tool exactly once with parameters collection_id="' + $inboxId + '" and dry_run=false. Then report the raw result.'
$r = Api-Post "/v1/chat/messages" @{ session_id = $bsid; agent_key = "__memory__"; content = $msg } -TimeoutSec 420
Write-Host ("chat code=" + $r.Code)
if ($r.Body.agentMessage.content_markdown) { Write-Host $r.Body.agentMessage.content_markdown }
Start-Sleep -Seconds 3
$staleAt = (docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT stale_at IS NOT NULL FROM knowledge_documents WHERE id='evalstaletest01';" | Out-String).Trim()
$prop = (docker exec aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT COUNT(*) FROM knowledge_governance_proposal WHERE collection_id='$inboxId' AND kind='stale' AND payload::text LIKE '%evalstaletest01%';" | Out-String).Trim()
Write-Host ("stale_at=" + $staleAt + " staleProposals=" + $prop)
