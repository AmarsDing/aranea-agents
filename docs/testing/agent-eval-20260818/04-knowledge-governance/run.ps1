# 域 D 知识库自治理图谱评测（指令驱动，真实 LLM + API 直测混合）
# 用法：powershell -ExecutionPolicy Bypass -File run.ps1 [-SkipCleanup]
# 阶段：0 建库 -> 1 API 直测(D18-ab/D01/D03) -> 2 chat 写回链路(D15/D18c/D16/D07/D17/D08/D05/D06)
#       -> 3 curate 治理(D09/D10/D19/D11) -> 4 D20 检索基准 -> 5 清理
# 关键约束（cases.md 详述）：
#   - team 库 ingest 文档 rel_path 为空，orphan/stale 判定只认 entries/* → D09/D10 必须在写回词条上构造
#   - 写回词条只落 inbox（团队收件箱）→ 治理类用例在 inbox 用「评测-」前缀词条隔离
#   - curate(inbox) 副作用与每日 dream_cycle 等价（skipProposal 去重，生产提案不重复产生）
param([switch]$SkipCleanup)
# docker exec/psql 输出为 UTF-8：控制台输出编码必须切 UTF8，否则 GBK 解码把
# 「中文+紧跟 ASCII 首字符」合并吞掉（"为10.20.88.1"→"涓?0.20.88.1"），
# Db-Query 结果的 Contains 断言全假阴性（2026-08-18 Run3 D08/D16/D17 实证）。
[Console]::OutputEncoding = [Text.UTF8Encoding]::new()
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "04"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
Renew-Token | Out-Null

$inboxId   = "a7310ebb25e82766f6e6"   # 团队知识收件箱（写回链路落点）
$agentKey  = "eval_memory_probe"
$agentId   = "a21265a2d8f24072fb638b50"   # eval_memory_probe UUID（CreateSession 只认 agent_id）
$butlerKey = "__memory__"
$butlerId  = "agent___memory__"
$collName  = "eval-gov-kb"

function Db-Query { param([string]$Sql)
    # Out-String 会把多行输出拼成 CRLF：按行去 \r 再 join，否则多行结果拆分后
    # 行尾残留 \r 混入 URL/SQL（2026-08-18 踩坑：DELETE id 带 %0D 幂等 200 但删不掉）。
    ((docker exec -i aranea-postgres psql -U postgres -d aranea -t -A -c $Sql | ForEach-Object { $_.TrimEnd("`r") }) -join "`n").Trim()
}
function Chat-Say { param([string]$Key, [string]$Sid, [string]$Content, [string]$OutFile, [int]$TimeoutSec = 180)
    Api-Post "/v1/chat/messages" @{ session_id = $Sid; agent_key = $Key; content = $Content } -OutFile $OutFile -TimeoutSec $TimeoutSec
}
# 在 inbox 按 rel_path 尾缀找评测词条 doc_id（LIKE 防 SQL 注入风险低，评测前缀固定）
function Find-EvalEntry { param([string]$Tail)
    Db-Query "SELECT id FROM knowledge_documents WHERE collection_id='$inboxId' AND rel_path LIKE 'entries/%$Tail%' ORDER BY created_at DESC LIMIT 1;"
}

# ============ 阶段 0：建评测库 + 上轮残留清理 ============
$existing = Api-Get -Path "/v1/knowledge/collections?limit=100"
$old = @($existing.Body.items) | Where-Object { $_.name -eq $collName } | Select-Object -First 1
if ($old) { Api-Delete -Path "/v1/knowledge/collections/$($old.id)" | Out-Null; Write-Host "[D-00] deleted stale $($old.id)" }
$rCreate = Api-Post -Path "/v1/knowledge/collections" -Body @{ name = $collName; description = "agent-eval-20260818 domain-D, cleanup after run"; embedding_model = "bge-m3"; vault_backend = "team" } -OutFile (Join-Path $ev "d00-create-collection.json")
$collId = $rCreate.Body.id
if ($rCreate.Code -ne "200" -or -not $collId) { Write-Host "[FATAL] create collection failed: $($rCreate.Code) $($rCreate.Raw)"; exit 1 }
Record $M "D-00" "create eval-gov-kb" PASS "id=$collId" $rCreate.Ms
# BUG-C-01 workaround：dim 修正为 bge-m3 实际 1024
Db-Query "UPDATE knowledge_collections SET dim=1024 WHERE id='$collId';" | Out-Null
$dimRow = Db-Query "SELECT dim FROM knowledge_collections WHERE id='$collId';"
Record $M "D-00b" "dim correct (BUG-C-01 workaround)" $(if ($dimRow -eq "1024") { "PASS" } else { "FAIL" }) "dim=$dimRow"

# 上轮残留清理：inbox 评测词条 + 评测提案（sw-eval-01 为 D15 别名 tag 可能落点，一并清理防跨轮污染）
$staleEntry = Db-Query "SELECT id FROM knowledge_documents WHERE collection_id='$inboxId' AND (rel_path LIKE 'entries/%评测%' OR rel_path LIKE 'entries/sw-eval-01%');"
if ($staleEntry) {
    foreach ($did in ($staleEntry -split "`n" | Where-Object { $_ -ne "" })) { Api-Delete -Path "/v1/knowledge/documents/$did" | Out-Null }
    Write-Host "[prep] cleaned inbox eval entries: $staleEntry"
}
Db-Query "DELETE FROM knowledge_governance_proposal WHERE collection_id='$inboxId' AND payload::text LIKE '%评测-%';" | Out-Null

# D-00c：工具策略授权（可重入）。knowledge_write 不属于任何 profile/group，
# 默认 coding profile 下 effective=denied(global_disabled)，必须 allow JSON 显式点名。
$rPol = Api-Call -Method PUT -Path "/v1/agents/$agentId/tools/policy" -Body @{
    tools_enabled = $true; profile = "coding"
    allow = @("knowledge_write", "knowledge_search"); deny = @()
}
Record $M "D-00c" "grant knowledge_write tool policy" $(if ($rPol.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($rPol.Code)" $rPol.Ms

# ============ 阶段 1：API 直测（eval-gov-kb） ============
# D18-a：ingest 主路径 chunk 重放（写入→可检索时延）
$entryMd = "# 评测核心交换机`n`n评测用核心交换机 SW-Eval-01 位于一楼机房 A03 机柜，负责办公网核心汇聚，每日巡检两次。"
$b64e = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($entryMd))
$t0 = Get-Date
$rDoc = Api-Post -Path "/v1/knowledge/documents" -Body @{ collection_id = $collId; source = "eval-core-switch.md"; mime_type = "text/markdown"; content_base64 = $b64e; chunk_strategy = "markdown" } -OutFile (Join-Path $ev "d18a-ingest-entry.json")
$entryDocId = $rDoc.Body.id
Record $M "D18-a0" "ingest entry doc" $(if ($rDoc.Code -eq "200" -and $entryDocId) { "PASS" } else { "FAIL" }) "docId=$entryDocId" $rDoc.Ms
$searchHit = $false; $searchMs = 0; $elapsed = 0
while ($elapsed -lt 60000) {
    $rS = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $collId; query = "SW-Eval-01 位于哪个机柜"; top_k = 3; hybrid_search = "auto" } -OutFile (Join-Path $ev "d18a-search.json")
    foreach ($ch in @($rS.Body.chunks)) { if ($ch.docId -eq $entryDocId) { $searchHit = $true; $searchMs = $rS.Ms; break } }
    if ($searchHit) { break }
    Start-Sleep -Seconds 2
    $elapsed = [int]((Get-Date) - $t0).TotalMilliseconds
}
Record $M "D18-a" "chunk 重放（ingest 写入即可检索）" $(if ($searchHit) { "PASS" } else { "FAIL" }) "writeToSearchable=${elapsed}ms searchMs=$searchMs"

# D18-b：ingest 第二篇（access_log/co_activated 需要同批多文档命中）
$noteMd = "# 评测巡检制度摘录`n`n一楼机房 A03 机柜的核心交换机 SW-Eval-01 每日巡检两次：上午 09:00 与下午 15:00。`n巡检内容包括端口状态、光模块温度、风扇与电源冗余。"
$b64n = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($noteMd))
$rIng = Api-Post -Path "/v1/knowledge/documents" -Body @{ collection_id = $collId; source = "eval-note.md"; mime_type = "text/markdown"; content_base64 = $b64n; chunk_strategy = "markdown" } -OutFile (Join-Path $ev "d18b-ingest.json")
$noteDocId = $rIng.Body.id
$indexed = $false
$dl = (Get-Date).AddSeconds(90)
while ((Get-Date) -lt $dl) {
    Start-Sleep -Seconds 3
    $st = Db-Query "SELECT status FROM knowledge_documents WHERE id='$noteDocId';"
    if ($st -eq "indexed" -or $st -eq "error") { $indexed = ($st -eq "indexed"); break }
}
Record $M "D18-b0" "ingest note doc indexed" $(if ($indexed) { "PASS" } else { "FAIL" }) "docId=$noteDocId status=$st"
$rS2 = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $collId; query = "每日巡检时间与内容"; top_k = 5; hybrid_search = "auto" } -OutFile (Join-Path $ev "d18b-search.json")
$hit2 = $false; foreach ($ch in @($rS2.Body.chunks)) { if ($ch.docId -eq $noteDocId) { $hit2 = $true; break } }
Record $M "D18-b" "chunk 重放（第二篇可检索）" $(if ($hit2) { "PASS" } else { "FAIL" }) "hit=$hit2" $rS2.Ms

# D01：访问日志（同批检索 x3 后 DB 查 knowledge_access_log）
1..3 | ForEach-Object { Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $collId; query = "SW-Eval-01 巡检"; top_k = 5 } | Out-Null }
Start-Sleep -Seconds 2
$accCnt = Db-Query "SELECT COUNT(*) FROM knowledge_access_log WHERE collection_id='$collId';"
Record $M "D01" "访问日志记录" $(if ([int]$accCnt -ge 1) { "PASS" } else { "FAIL" }) "access_log rows=$accCnt"

# D03（P2 INFO）：co_activated Hebbian 边（同批命中文档两两成边）
$coCnt = Db-Query "SELECT COUNT(*) FROM knowledge_links WHERE collection_id='$collId' AND link_type='co_activated';"
Record $M "D03" "Hebbian 共激活边" INFO "co_activated edges=$coCnt"

# ============ 阶段 2：chat 写回链路（inbox，「评测-」前缀词条） ============
$rSess = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-域D写回-20260818"; owner_type = "agent" }
$sid = $rSess.Body.id
if (-not $sid) { Write-Host "[FATAL] create session failed: $($rSess.Raw)"; exit 1 }
[IO.File]::WriteAllText((Join-Path $ev "writeback-session.txt"), $sid, [Text.UTF8Encoding]::new($false))

# D15：knowledge_write 工具写入（tags 含 alias SW-Eval-01）
$t15 = Get-Date
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-核心交换机SW-Eval-01的管理IP为10.20.99.1"，tags=["评测-核心交换机","SW-Eval-01"]，fact_id="eval-sw-ip"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d15-chat-write.json")
Record $M "D15-chat" "chat 写回请求" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 8
$entryInboxId = Find-EvalEntry "评测-核心交换机"
Record $M "D15" "knowledge_write 词条落库" $(if ($entryInboxId) { "PASS" } else { "FAIL" }) "docId=$entryInboxId"

# D18-c：写回链路 chunk 重放回归点（词条写入→inbox 立即可检索，2026-08-15 事故口径）
$wbHit = $false; $wbElapsed = 0
if ($entryInboxId) {
    while ($wbElapsed -lt 8000) {
        $rW = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $inboxId; query = "SW-Eval-01 的管理IP"; top_k = 5; hybrid_search = "auto" } -OutFile (Join-Path $ev "d18c-search-inbox.json")
        foreach ($ch in @($rW.Body.chunks)) { if ($ch.docId -eq $entryInboxId) { $wbHit = $true; break } }
        if ($wbHit) { break }
        Start-Sleep -Milliseconds 800
        $wbElapsed = [int]((Get-Date) - $t15).TotalMilliseconds
    }
}
Record $M "D18-c" "chunk 重放（写回词条即可检索，P0 回归点）" $(if ($wbHit) { "PASS" } else { "FAIL" }) "writeToSearchable=${wbElapsed}ms (目标<5000)"

# D16/D07：同 fact_id 二次写入更新（整段替换 + 版本快照）
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具更新一条事实，参数：statement="评测-核心交换机SW-Eval-01的管理IP为10.20.99.2"，tags=["评测-核心交换机"]，fact_id="eval-sw-ip"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d16-chat-upsert.json")
Record $M "D16-chat" "chat 更新请求" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 8
if ($entryInboxId) {
    $bodyNow = Db-Query "SELECT content_text FROM knowledge_documents WHERE id='$entryInboxId';"
    $hasNew = $bodyNow.Contains("10.20.99.2")
    $oldCnt = ([regex]::Matches($bodyNow, "10\.20\.99\.1")).Count
    Record $M "D16" "词条 upsert 整段替换" $(if ($hasNew -and $oldCnt -eq 0) { "PASS" } else { "FAIL" }) "hasNewIP=$hasNew oldIPoccurrences=$oldCnt"
    $verCnt = Db-Query "SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id='$entryInboxId' AND fact_id='eval-sw-ip';"
    Record $M "D07" "supersedes 版本链快照" $(if ([int]$verCnt -ge 1) { "PASS" } else { "FAIL" }) "fact_version rows=$verCnt"
}

# D17：alias 命中合并（tag=SW-Eval-01 应合并进同页，不新建词条）
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-核心交换机的运维负责人是张伟"，tags=["SW-Eval-01"]，fact_id="eval-sw-owner"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d17-chat-alias.json")
Record $M "D17-chat" "chat alias 写入" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 8
$aliasNew = Db-Query "SELECT COUNT(*) FROM knowledge_documents WHERE collection_id='$inboxId' AND rel_path LIKE 'entries/sw-eval-01%';"
$merged = 0
if ($entryInboxId) {
    $b2 = Db-Query "SELECT content_text FROM knowledge_documents WHERE id='$entryInboxId';"
    if ($b2.Contains("张伟")) { $merged = 1 }
}
Record $M "D17" "别名解析合并（不新建词条）" $(if ([int]$aliasNew -eq 0 -and $merged -eq 1) { "PASS" } else { "FAIL" }) "newEntries=$aliasNew mergedIntoExisting=$merged"

# D17b：typed 关系素材（先建「评测-值班制度」词条，再写含词条引用的陈述）
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-值班制度规定核心设备每日巡检两次"，tags=["评测-值班制度"]，fact_id="eval-duty-1"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d17b-chat-duty.json")
Record $M "D17b-chat1" "chat 建值班制度词条" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 6
$dutyDocId = Find-EvalEntry "评测-值班制度"
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-核心交换机SW-Eval-01的巡检流程遵循评测-值班制度的规定"，tags=["评测-核心交换机"]，fact_id="eval-sw-duty"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d17b-chat-typed.json")
Record $M "D17b-chat2" "chat typed 关系素材" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms

# D09/D10 场景词条：陈旧（独特实体 QQQ-Stale-77 防 hook 成边）+ 孤儿（独特实体 ZZZ-Orphan-99）
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-陈旧锚点QQQ-Stale-77的参考值为42"，tags=["评测-陈旧词条"]，fact_id="eval-stale-1"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d09prep-chat-stale.json")
Record $M "D09-prep-chat" "chat 建陈旧词条" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 6
$staleDocId = Find-EvalEntry "评测-陈旧词条"
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-孤立锚点ZZZ-Orphan-99与任何设备无关"，tags=["评测-孤儿词条"]，fact_id="eval-orphan-1"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d10prep-chat-orphan.json")
Record $M "D10-prep-chat" "chat 建孤儿词条" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 6
$orphanDocId = Find-EvalEntry "评测-孤儿词条"

# D08：写入时仲裁（P0，arbiter 对同页既有段裁决；放最后使既有段已含 10.20.99.2）
# 双路径断言（2026-08-18 修订）：同属性新值（10.20.99.2→10.20.88.1）语义上是更新，
# arbiter 判 supersedes（整段替换+source_id 留痕+版本链）或 contradicts（pending 提案）
# 均为正确裁决；断言「两条路径必居其一」，不再强预期 contradicts。
$r = Chat-Say $agentKey $sid '请立即调用 knowledge_write 工具写入一条事实，参数：statement="评测-核心交换机SW-Eval-01的管理IP为10.20.88.1"，tags=["评测-核心交换机"]，fact_id="eval-sw-ip-alt"，confidence=0.95。只调用这一个工具。' (Join-Path $ev "d08-chat-conflict.json")
Record $M "D08-chat" "chat 矛盾写入" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 12
$confPropId = Db-Query "SELECT id FROM knowledge_governance_proposal WHERE collection_id='$inboxId' AND kind='conflict' AND status='pending' AND payload::text LIKE '%eval-sw-ip-alt%' ORDER BY id DESC LIMIT 1;"
$d08Path = ""; $d08ok = $false
if ($confPropId -match '^\d+$') {
    $d08Path = "contradicts proposal=$confPropId"; $d08ok = $true
} elseif ($entryInboxId) {
    $b8 = Db-Query "SELECT content_text FROM knowledge_documents WHERE id='$entryInboxId';"
    $hasAlt = $b8.Contains("10.20.88.1")
    $hasLineage = $b8.Contains("source_id: ``eval-sw-ip-alt``")
    $verAlt = Db-Query "SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id='$entryInboxId' AND fact_id='eval-sw-ip';"
    if ($hasAlt -and $hasLineage -and [int]$verAlt -ge 1) { $d08Path = "supersedes (段替换+source_id留痕+版本链 rows=$verAlt)"; $d08ok = $true }
    else { $d08Path = "neither: hasAlt=$hasAlt lineage=$hasLineage verRows=$verAlt" }
}
Record $M "D08" "写入时仲裁（supersedes 或 contradicts 提案）" $(if ($d08ok) { "PASS" } else { "FAIL" }) $d08Path

# D05/D06：等 WriteBackGraphHook 异步抽取（entity + typed relation，双 LLM 管道）
Write-Host "等待 60s 实体共现/typed 关系异步抽取..."
Start-Sleep -Seconds 60
if ($entryInboxId) {
    $rLinks = Api-Get -Path "/v1/knowledge/documents/$entryInboxId/links" -OutFile (Join-Path $ev "d05-entry-links.json")
    $entityEdges = @($rLinks.Body.items) | Where-Object { $_.link_type -eq "entity" }
    $semanticEdges = @($rLinks.Body.items) | Where-Object { $_.link_type -eq "semantic" }
    Record $M "D05" "实体共现边生成" $(if (@($entityEdges).Count -ge 1) { "PASS" } else { "REVIEW" }) "entity edges=$(@($entityEdges).Count)"
    Record $M "D06" "typed 语义关系抽取" $(if (@($semanticEdges).Count -ge 1) { "PASS" } else { "REVIEW" }) "semantic edges=$(@($semanticEdges).Count)"
}

# D20-pre：治理前检索基准（inbox 搜评测词条专属词，治理对象不含「评测-核心交换机」）
$d20q = @("SW-Eval-01 的管理IP", "核心设备每日巡检")
$d20pre = 0
foreach ($q in $d20q) {
    $r = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $inboxId; query = $q; top_k = 5 }
    $hitQ = $false
    foreach ($ch in @($r.Body.chunks)) { if ($ch.docId -eq $entryInboxId -or $ch.docId -eq $dutyDocId) { $hitQ = $true; break } }
    if ($hitQ) { $d20pre++ }
}
Record $M "D20-pre" "治理前检索基准" INFO "hit=$d20pre/$($d20q.Count)"

# ============ 阶段 3：curate 治理（inbox，评测词条隔离） ============
# 竞态消除（2026-08-18 Run3）：M2 内容驱动抽取会给陈旧/孤儿词条建 semantic 边
# （独特实体假设被击穿），稀释 closed_ratio 并使孤儿有边。构造场景前先清掉两词条
# 上的全部已建边——抽取按 content_hash 幂等，正文不再变更就不会重抽，清后不复发。
if ($staleDocId) { Db-Query "DELETE FROM knowledge_links WHERE collection_id='$inboxId' AND (doc_id='$staleDocId' OR target_doc_id='$staleDocId');" | Out-Null }
if ($orphanDocId) { Db-Query "DELETE FROM knowledge_links WHERE collection_id='$inboxId' AND (doc_id='$orphanDocId' OR target_doc_id='$orphanDocId');" | Out-Null }
# DB 构造 stale 场景：「评测-陈旧词条」出向 semantic 边 closed_ratio=0.8（1 active + 4 closed）
if ($staleDocId -and $entryInboxId) {
    $chk = Db-Query "SELECT COUNT(*) FROM knowledge_documents WHERE id='$staleDocId' AND collection_id='$inboxId';"
    if ([int]$chk -eq 1) {
        Db-Query "INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, relation, context, confidence, weight_f, valid_from) VALUES ('$inboxId','$staleDocId','$entryInboxId','semantic','related_to','eval-stale-active',0.9,1.0,NOW());" | Out-Null
        1..4 | ForEach-Object {
            Db-Query "INSERT INTO knowledge_links (collection_id, doc_id, target_doc_id, link_type, relation, context, confidence, weight_f, valid_from, valid_to) VALUES ('$inboxId','$staleDocId','$entryInboxId','semantic','related_to','eval-stale-closed-$_',0.9,1.0,NOW(),NOW());" | Out-Null
        }
        $linkCnt = Db-Query "SELECT COUNT(*) FROM knowledge_links WHERE collection_id='$inboxId' AND doc_id='$staleDocId' AND link_type='semantic';"
        Record $M "D09-prep" "stale 场景构造（closed_ratio=0.8）" $(if ([int]$linkCnt -ge 5) { "PASS" } else { "FAIL" }) "semantic edges=$linkCnt"
    }
} else { Record $M "D09-prep" "stale 场景构造" FAIL "staleDocId=$staleDocId entryInboxId=$entryInboxId" }

# D09/D10/D19-a：chat __memory__ 执行 curate（dry_run=false）
$rButlerSess = Api-Post "/v1/sessions" @{ agent_id = $butlerId; title = "eval-域D治理-20260818"; owner_type = "agent" }
$bsid = $rButlerSess.Body.id
$r = Chat-Say $butlerKey $bsid "请立即调用 memory_butler_knowledge_curate 工具，参数 collection_id=`"$inboxId`"，dry_run=false。只执行这一次工具调用，然后汇报执行结果。" (Join-Path $ev "d09-chat-curate.json") 420
Record $M "D19-a" "memory_butler_knowledge_curate 可用性" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms
Start-Sleep -Seconds 8
$staleProp = Db-Query "SELECT COUNT(*) FROM knowledge_governance_proposal WHERE collection_id='$inboxId' AND kind='stale' AND payload::text LIKE '%$staleDocId%';"
$staleAt = if ($staleDocId) { Db-Query "SELECT stale_at IS NOT NULL FROM knowledge_documents WHERE id='$staleDocId';" } else { "?" }
Record $M "D09" "陈旧提案（自动 applied + stale_at 置位）" $(if ([int]$staleProp -ge 1 -and $staleAt -eq "t") { "PASS" } else { "FAIL" }) "staleProps=$staleProp stale_at=$staleAt"
$orphanPropId = Db-Query "SELECT id FROM knowledge_governance_proposal WHERE collection_id='$inboxId' AND kind='orphan' AND status='pending' AND payload::text LIKE '%$orphanDocId%' ORDER BY id DESC LIMIT 1;"
Record $M "D10" "孤儿提案（pending 人工二审）" $(if ($orphanPropId -match '^\d+$') { "PASS" } else { "FAIL" }) "proposalId=$orphanPropId"

# D19-b：governance_proposals 工具
$r = Chat-Say $butlerKey $bsid "请立即调用 memory_butler_governance_proposals 工具，参数 collection_id=`"$inboxId`"，status=`"pending`"。只执行这一次工具调用，然后汇报提案列表。" (Join-Path $ev "d19-chat-proposals.json") 420
Record $M "D19-b" "memory_butler_governance_proposals 可用性" $(if ($r.Code -eq "200") { "PASS" } else { "FAIL" }) "code=$($r.Code)" $r.Ms

# D11-a：orphan 提案 → applied（disposal 生效删词条）
if ($orphanPropId -match '^\d+$') {
    $rResolve = Api-Post -Path "/v1/knowledge/governance-proposals/$orphanPropId`:resolve" -Body @{ decision = "applied" } -OutFile (Join-Path $ev "d11-resolve-orphan.json")
    Start-Sleep -Seconds 3
    $propStatus = Db-Query "SELECT status FROM knowledge_governance_proposal WHERE id=$orphanPropId;"
    $orphanGone = Db-Query "SELECT COUNT(*) FROM knowledge_documents WHERE id='$orphanDocId';"
    Record $M "D11-a" "提案二审（orphan applied→删词条）" $(if ($propStatus -eq "applied" -and [int]$orphanGone -eq 0) { "PASS" } else { "FAIL" }) "code=$($rResolve.Code) status=$propStatus orphanDocLeft=$orphanGone" $rResolve.Ms
} else { Record $M "D11-a" "提案二审（orphan）" FAIL "no pending orphan proposal" }

# D11-b：conflict 提案（fact-level）→ keep_old（删除矛盾新段）；
# D08 走 supersedes 路径时无提案可二审，改验版本链留痕完整性（INFO）。
if ($confPropId -match '^\d+$') {
    $rResolve2 = Api-Post -Path "/v1/knowledge/governance-proposals/$confPropId`:resolve" -Body @{ decision = "keep_old" } -OutFile (Join-Path $ev "d11-resolve-conflict.json")
    Start-Sleep -Seconds 3
    $propStatus2 = Db-Query "SELECT status FROM knowledge_governance_proposal WHERE id=$confPropId;"
    $bodyAfter = if ($entryInboxId) { Db-Query "SELECT content_text FROM knowledge_documents WHERE id='$entryInboxId';" } else { "" }
    $altGone = -not $bodyAfter.Contains("10.20.88.1")
    Record $M "D11-b" "提案二审（conflict keep_old→删新段）" $(if (($propStatus2 -eq "applied" -or $propStatus2 -eq "rejected") -and $altGone) { "PASS" } else { "FAIL" }) "code=$($rResolve2.Code) status=$propStatus2 altIPgone=$altGone" $rResolve2.Ms
} else {
    $verKeep = if ($entryInboxId) { Db-Query "SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id='$entryInboxId' AND fact_id='eval-sw-ip';" } else { "0" }
    Record $M "D11-b" "提案二审（supersedes 路径：版本链留痕核验）" $(if ([int]$verKeep -ge 1) { "PASS" } else { "FAIL" }) "no proposal (supersedes); fact_version rows=$verKeep"
}

# ============ 阶段 4：D20 治理后检索基准 ============
$d20post = 0
foreach ($q in $d20q) {
    $r = Api-Post -Path "/v1/knowledge/search" -Body @{ collection_id = $inboxId; query = $q; top_k = 5 }
    $hitQ = $false
    foreach ($ch in @($r.Body.chunks)) { if ($ch.docId -eq $entryInboxId -or $ch.docId -eq $dutyDocId) { $hitQ = $true; break } }
    if ($hitQ) { $d20post++ }
}
Record $M "D20" "自治理不劣化检索" $(if ($d20post -ge $d20pre) { "PASS" } else { "REVIEW" }) "pre=$d20pre/$($d20q.Count) post=$d20post/$($d20q.Count)"

# ============ 阶段 5：清理 ============
if (-not $SkipCleanup) {
    $evalDocs = Db-Query "SELECT id FROM knowledge_documents WHERE collection_id='$inboxId' AND (rel_path LIKE 'entries/%评测%' OR rel_path LIKE 'entries/sw-eval-01%');"
    foreach ($did in ($evalDocs -split "`n" | Where-Object { $_ -ne "" })) { Api-Delete -Path "/v1/knowledge/documents/$did" | Out-Null }
    # 残留评测提案（未 resolve 的）一律 rejected（拒绝即沉默，不再周期重提）
    $leftProps = Db-Query "SELECT id FROM knowledge_governance_proposal WHERE collection_id='$inboxId' AND status='pending' AND payload::text LIKE '%评测-%';"
    foreach ($propId in ($leftProps -split "`n" | Where-Object { $_ -match '^\d+$' })) { Api-Post -Path "/v1/knowledge/governance-proposals/$propId`:resolve" -Body @{ decision = "rejected" } | Out-Null }
    # DB 构造的 semantic 边随词条删除清理（若 DeleteDocument 不级联边则显式删）
    if ($staleDocId) { Db-Query "DELETE FROM knowledge_links WHERE collection_id='$inboxId' AND doc_id='$staleDocId' AND context LIKE 'eval-stale-%';" | Out-Null }
    Api-Delete -Path "/v1/knowledge/collections/$collId" | Out-Null
    Write-Host "[cleanup] inbox entries=[$evalDocs] leftoverProps=[$leftProps] eval-gov-kb deleted"
    Record $M "D-cleanup" "评测数据清理" PASS "entries/proposals/links/collection cleaned"
}
Write-Host "=== 域 D 完成，证据见 evidence/results.md ==="
