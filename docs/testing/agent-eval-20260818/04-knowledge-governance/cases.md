# 域 D 知识库自治理图谱评测用例（agent-eval-20260818）

## 测试拓扑

| 隔离单元 | 用途 | 清理 |
|---------|------|------|
| 评测库 `eval-gov-kb`（team backend，bge-m3，dim 修正 1024） | D18/D01/D03/D09/D10/D20 | 测后删库 |
| 团队收件箱 inbox（`a7310ebb25e82766f6e6`）中「评测-」前缀词条 | D15/D16/D07/D08/D17/D05/D06 | 测后删词条文档 + resolve 提案 |

## 关键机理（决定测试路径）

1. **写回链路（knowledge_write 工具）只落团队收件箱**（`LookupWriteBackHome`），无 collection 选择参数 → 词条类用例只能落在 inbox，用「评测-」前缀 tag 隔离。
2. **team 库无 vault 同步循环**：普通 IngestDocument 不触发实体/typed 关系抽取；只有**写回路径**（WriteBackGraphHook，wire.go L1452-1493）对 touched 词条页异步触发 entity.ProcessDoc + relation.ExtractDoc → D05/D06 必须走写回链路。
3. **arbiter 同步在写回路径内**（upsertEntryDoc L286）：追加新事实段时对同页既有段 LLM 仲裁，contradicts → recordConflictProposalLater 落 conflict/pending 提案；supersedes → 版本链替换 + knowledge_fact_version 快照。
4. **stale/orphan 候选判定**（data/knowledge_curate.go）：entries/* 且（无 access_log 或超 30 天未访问）；stale 另需出向 semantic 边 closed_ratio≥0.5；orphan 需无任何 active 边。评测库新建词条无 access_log → `last_access_days=999999` 天然满足时间条件。
5. **curate 触发**：memory_butler_knowledge_curate 工具（仅 collection_id + dry_run 两参）经 __memory__ agent chat 调用；无 HTTP 直调。对 eval-gov-kb 隔离库执行，不触碰 inbox 生产数据。
6. **无独立 distill 工具**：distill 内嵌 knowledge_curate（CurateReport.DistilledFacts），D19 按实际装配验证 3 个治理工具。

## 用例清单

| 编号 | 功能点 | 步骤 | 判定 |
|------|--------|------|------|
| D-00 | 建评测库 | CreateCollection(team) + DB 修正 dim=1024（BUG-C-01 workaround） | 200 + dim=1024 |
| D18-a | chunk 重放（vault 写入口） | CreateVaultDocument entries/ → UpdateDocumentContent → 立即 Search | **写入→可检索 <5s**（P0 回归点） |
| D18-b | chunk 重放（ingest 对照） | IngestDocument notes/ → 轮询 indexed → Search | indexed 且可检索 |
| D01 | 访问日志 | 对 eval 库 Search x3 → DB 查 knowledge_access_log | eval 库 doc 有访问记录 |
| D03 | Hebbian 共激活边（P2 INFO） | 同批检索命中 ≥2 文档 → DB 查 co_activated 边 | 边生成（INFO 观察） |
| D15 | knowledge_write 工具 | chat eval_memory_probe 写「评测-核心交换机」事实 | inbox 出现 entries/评测-核心交换机.md |
| D16/D07 | 词条 upsert + supersedes 版本链 | 同 fact_id 二次写入更新 IP → 查词条 + fact_version | 整段替换不重复；旧段快照落 knowledge_fact_version |
| D05/D06 | 实体共现边 + typed 关系 | 写回后等异步 hook → ListDocumentLinks | 词条文档出现 entity/semantic 出边 |
| D08 | 冲突检测提案（P0） | 写入矛盾事实（新 fact_id，不同 IP）→ 等 arbiter | conflict/pending 提案，payload 含评测词条 |
| D17 | 别名解析合并 | tag 用既有词条 alias 写入 → 查词条数 | 合并进同页，不新建词条 |
| D09 | 陈旧提案 | DB 构造 closed semantic 边 → chat curate(dry_run=false, eval 库) | stale 提案 applied + documents.stale_at 置位 |
| D10 | 孤儿提案 | 同一次 curate（无 active 边词条天然候选） | orphan 提案 pending |
| D11 | 提案人工二审 | ResolveGovernanceProposal：orphan→approved（生效删词条）；conflict→rejected | 状态流转 + orphan 词条被删 |
| D19 | 治理工具可用性 | chat __memory__：knowledge_curate（D09/D10 已覆盖）+ governance_proposals | 工具返回正确结构 |
| D20 | 治理不劣化检索 | 治理前后对 eval 库跑同一 3 条查询基准 | 治理后命中率不降 |

## 矛盾对设计（D08 arbiter 触发）

- 原事实（D16 后词条既有段）：`评测-核心交换机SW-Eval-01的管理IP为10.20.99.2`
- 矛盾事实：`评测-核心交换机SW-Eval-01的管理IP为10.20.88.1`（新 fact_id=eval-sw-ip-conflict）
- 同词条页（tag 命中「评测-核心交换机」）→ 追加前 arbiter 对既有段仲裁 → 预期 contradicts

## chat 指令模板

eval_memory_probe（tools_enabled=t，profile=coding + allow=["knowledge_write","knowledge_search"]；knowledge_write 不属于任何 profile/group，必须 allow 显式点名，run.ps1 D-00c 步可重入授权）：
> 请立即调用 knowledge_write 工具写入以下事实，不要其他操作：statement="..."，tags=["评测-核心交换机"]，fact_id="eval-sw-ip"，confidence=0.95

__memory__（记忆管家，butler 工具组）：
> 请立即调用 memory_butler_knowledge_curate 工具，参数 collection_id="<eval-gov-kb-id>"，dry_run=false。只执行这一次调用。
