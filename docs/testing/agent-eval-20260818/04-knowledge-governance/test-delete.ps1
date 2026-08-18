. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
Renew-Token | Out-Null
$id = "54a7ab54296d01a3096e"
$before = (docker exec -i aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT COUNT(*) FROM knowledge_documents WHERE id='$id';" | Out-String).Trim()
$r = Api-Delete -Path "/v1/knowledge/documents/$id"
Start-Sleep -Seconds 1
$after = (docker exec -i aranea-postgres psql -U postgres -d aranea -t -A -c "SELECT COUNT(*) FROM knowledge_documents WHERE id='$id';" | Out-String).Trim()
Write-Host "before=$before code=$($r.Code) raw=$($r.Raw) after=$after"
