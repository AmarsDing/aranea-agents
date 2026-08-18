#Requires -Version 5.1
<#
.SYNOPSIS
  域 D 污染事故清理（f5）：删除 inbox 垃圾词条 + 关联边 + 垃圾驱动提案。
.DESCRIPTION
  垃圾词条 = rel_path 命中保留键（entries/fact-id.md 等）或 UUID 形态的文档。
  词条经 API 删除（保证 chunk 等派生数据清理）；边与提案按 SQL 三层校验
  （先 COUNT 确认命中 → 显式事务 → 核验 affected rows）清理。
#>
param()
$ErrorActionPreference = 'Stop'
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")

function Db-Query { param([string]$Sql)
    ((docker exec -i aranea-postgres psql -U postgres -d aranea -t -A -c $Sql | ForEach-Object { $_.TrimEnd("`r") }) -join "`n").Trim()
}

# 垃圾词条判定：与 entry_key_guard.go 同口径（保留键 slug + UUID/长hex 形态）
$garbageRe = "entries/(fact-id|session-id|agent-id|user-id|confidence|kind|tags|source|source-id|entry|statement|preference|profile|goal|constraint|decision|relationship)\.md$"
$uuidRe = "entries/[0-9a-f]{8}-[0-9a-f]{4}"

# ── 1. 垃圾词条：API 删除 ────────────────────────────────────────────────
$ids = Db-Query "SELECT id FROM knowledge_documents WHERE rel_path ~ '$garbageRe' OR rel_path ~ '$uuidRe';"
$idList = @($ids -split "`n" | Where-Object { $_ -match '^[0-9a-f]{16,32}$' })
Write-Host "垃圾词条 $($idList.Count) 条，经 API 删除..."
Renew-Token | Out-Null
$ok = 0; $fail = 0
foreach ($id in $idList) {
    $r = Api-Delete -Path "/v1/knowledge/documents/$id"
    if ($r.Code -eq "200") { $ok++ } else { $fail++; Write-Host "  FAIL id=$id code=$($r.Code) raw=$($r.Raw)" }
}
Write-Host "API 删除：ok=$ok fail=$fail"

$leftDocs = Db-Query "SELECT COUNT(*) FROM knowledge_documents WHERE rel_path ~ '$garbageRe' OR rel_path ~ '$uuidRe';"
Write-Host "残留垃圾词条：$leftDocs"

# ── 2. 孤儿边清理（指向已不存在文档的边）：三层校验 ──────────────────────
$orphanLinkCount = Db-Query "SELECT COUNT(*) FROM knowledge_links l WHERE NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.doc_id) OR NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.target_doc_id);"
Write-Host "孤儿边（端点文档已不存在）：$orphanLinkCount"
if ([int]$orphanLinkCount -gt 0) {
    $del = Db-Query "BEGIN; DELETE FROM knowledge_links l WHERE NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.doc_id) OR NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.target_doc_id); COMMIT;"
    $leftLinks = Db-Query "SELECT COUNT(*) FROM knowledge_links l WHERE NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.doc_id) OR NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.target_doc_id);"
    Write-Host "孤儿边清理后残留：$leftLinks（应为 0）"
}

# ── 3. 垃圾驱动提案（payload 引用垃圾词条路径）：三层校验 ────────────────
$propSql = "FROM knowledge_governance_proposal WHERE status='pending' AND (payload::text ~ 'entries/(fact-id|session-id|agent-id|user-id|confidence|kind|tags|source|source-id|entry|statement|preference|profile|goal|constraint|decision|relationship)[^/]*\.md' OR payload::text ~ 'entries/[0-9a-f]{8}-[0-9a-f]{4}');"
$propCount = Db-Query ("SELECT COUNT(*) " + $propSql)
Write-Host "垃圾驱动 pending 提案：$propCount"
if ([int]$propCount -gt 0) {
    Db-Query ("BEGIN; DELETE " + $propSql + " COMMIT;") | Out-Null
    $leftProp = Db-Query ("SELECT COUNT(*) " + $propSql)
    Write-Host "提案清理后残留：$leftProp（应为 0）"
}

# ── 4. 终态核验 ──────────────────────────────────────────────────────────
$finalDocs = Db-Query "SELECT COUNT(*) FROM knowledge_documents WHERE rel_path ~ '$garbageRe' OR rel_path ~ '$uuidRe';"
$finalLinks = Db-Query "SELECT COUNT(*) FROM knowledge_links l WHERE NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.doc_id) OR NOT EXISTS (SELECT 1 FROM knowledge_documents d WHERE d.id=l.target_doc_id);"
$finalProp = Db-Query ("SELECT COUNT(*) " + $propSql)
Write-Host "== 终态：garbage_docs=$finalDocs orphan_links=$finalLinks garbage_proposals=$finalProp =="
