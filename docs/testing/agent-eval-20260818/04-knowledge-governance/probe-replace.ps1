#Requires -Version 5.1
# 复现探针：同 fact_id 二次写入后整段替换内容校验（域 D D16/D08 FAIL 归因）
$ErrorActionPreference = 'Stop'
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")

function Db-Query { param([string]$Sql)
    ((docker exec -i aranea-postgres psql -U postgres -d aranea -t -A -c $Sql | ForEach-Object { $_.TrimEnd("`r") }) -join "`n").Trim()
}
function Chat-Say { param([string]$Key, [string]$Sid, [string]$Content)
    Api-Post "/v1/chat/messages" @{ session_id = $Sid; agent_key = $Key; content = $Content } -TimeoutSec 180
}

$inboxId  = "a7310ebb25e82766f6e6"
$agentKey = "eval_memory_probe"
$agentId  = "a21265a2d8f24072fb638b50"

Renew-Token | Out-Null
$rSess = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "probe-replace-20260818"; owner_type = "agent" }
$sid = $rSess.Body.id
if (-not $sid) { Write-Host "FATAL no session: $($rSess.Raw)"; exit 1 }
Write-Host "session=$sid"

# 写 1：192.0.2.111
$r1 = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="探针-替换条目PX-9的管理IP为192.0.2.111"，tags=["探针-替换条目"]，fact_id="probe-rep-1"，confidence=0.95。只调用这一个工具。'
Write-Host "write1 code=$($r1.Code)"
Start-Sleep -Seconds 8
$docId = Db-Query "SELECT id FROM knowledge_documents WHERE collection_id='$inboxId' AND rel_path LIKE 'entries/%探针-替换条目%' ORDER BY created_at DESC LIMIT 1;"
Write-Host "entry doc=$docId"
$b1 = Db-Query "SELECT content_text FROM knowledge_documents WHERE id='$docId';"
Write-Host "== after write1: has111=$($b1.Contains('192.0.2.111')) len=$($b1.Length)"
Write-Host "---- body1 ----"
Write-Host $b1
Write-Host "---------------"

# 写 2：同 fact_id 改 192.0.2.222
$r2 = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具更新一条事实，参数：statement="探针-替换条目PX-9的管理IP为192.0.2.222"，tags=["探针-替换条目"]，fact_id="probe-rep-1"，confidence=0.95。只调用这一个工具。'
Write-Host "write2 code=$($r2.Code)"
Start-Sleep -Seconds 8
$b2 = Db-Query "SELECT content_text FROM knowledge_documents WHERE id='$docId';"
Write-Host "== after write2: has222=$($b2.Contains('192.0.2.222')) has111=$($b2.Contains('192.0.2.111')) len=$($b2.Length)"
Write-Host "---- body2 ----"
Write-Host $b2
Write-Host "---------------"
$ver = Db-Query "SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id='$docId' AND fact_id='probe-rep-1';"
Write-Host "fact_version rows=$ver"

# 清理
if ($docId -match '^[0-9a-f]+$') { $null = Api-Delete -Path "/v1/knowledge/documents/$docId"; Write-Host "cleaned doc $docId" }
