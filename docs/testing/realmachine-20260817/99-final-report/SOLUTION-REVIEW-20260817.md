# 真机测试缺陷深度分析与方案评审（2026-08-17）

> 上游文档：[REALMACHINE-REPORT-20260817.md](REALMACHINE-REPORT-20260817.md)（缺陷清单与优先级）。
> 本文档对清单中 P1/P2 全部缺陷与关键 P3 项做**系统性根因深挖**，每个缺陷给出：
> **根因证据链 → 解决方案 → 副作用评审（会不会带来其他问题）→ 有效性评审（是否真正解决）→ 验证方法**。
> 所有 DB 证据来自真机 `aranea-postgres` 容器实况；所有代码位置附 file:line。

---

## 〇、评审结论矩阵

| # | 缺陷 | 根因定性 | 方案核心 | 副作用风险 | 有效性 | 待裁定 |
|---|------|---------|---------|-----------|--------|--------|
| P1-1 | BUG-01 无岗位 Agent 创建必败 | 唯一索引设计过宽 | 部分唯一索引 `WHERE position_key<>'' AND deleted_at=''` | 低（已验证无读路径依赖、无历史重复） | 三模式全覆盖 | 无 |
| P1-2 | BUG-02 会话删除恒 500 | 表重构后级联未同步 | 删 messages 行 + 补 9 张活跃表清理 + PG 集成回归 | 低（审计表保留已裁定） | F1 直接消除；F2 止漏 | ✅已裁定：审计表保留；RestoreSession 维持现状语义 |
| P2-3 | BUG-MON-A 澄清被自动代答为取消 | 意图 prompt 缺 destructive 示例 + autoResolve 无否决守卫 | 三层防御：prompt 补例 + 确定性强制打标 + 推荐否决守卫 | 低（误报方向安全） | 事故路径三重拦截 | 无 |
| P2-4 | BUG-MON-C 端口级故障无告警 | 监控模型粒度缺失 | 嗅探拓扑自动生成钉端口 SNMP 探测（discovered_links 端口持久化 + 自动建线） | 中（GNS3 设备需 SNMP+CDP 可达） | 补上断链环节，拓扑变化自跟随 | ✅已裁定：(b) 拓扑自动生成 |
| P2-5 | BUG-G1 残缺图 visualize 500 | 错误码被压平为 Internal | `apierror.Wrap` 透传 BadRequest | 极低 | 400 语义恢复 | 无 |
| P2-6 | BUG-CLI-01 非法命令静默退出 | 错误打印双缺位 | main() 退出前打印 stderr | 极低（排查过无双打路径） | 直接消除 | 无 |
| P3-7 | ISSUE-G2 无检查点执行 404 | 两种"不存在"共用 ErrNotFound | 先查执行存在性，无 lineage 返回 200 空集 | 低 | 语义可分 | 无 |
| P3-8 | ISSUE-G3 step_index=0 被拒 | proto3 REQUIRED 零值陷阱 | 去 REQUIRED，业务层校验 `>=0` | 低（需 buf generate） | 0 步可回溯 | 无 |
| P3-9 | BUG-MON-B 持久化授权无 TTL | schema 缺 expires_at | 加 expires_at + 默认 TTL 72h + 读径过滤 | 低（存量行兼容策略已定） | 授权有时界 | ✅已裁定：默认 TTL=72h |
| P3-11 | PERF-S1 Spirit 24k token/轮 | 工具声明全量注入高度疑似 | 工具分级装载 + prompt cache | 中（需抓包确认构成） | 预期降 50%+ | 先取证后实施 |
| P3-12 | PERF-F1 providers 510ms | 2.88MB JSON 每请求全量读盘反序列化 | mtime+size 失效缓存 | 低 | 预期 <50ms | 无 |

---

## 一、BUG-01（P1）：无岗位 Agent 创建必败

### 1.1 根因证据链

**索引定义**（[ent/schema/agent.go:26](file:///f:/myproject/aranea-agents/internal/data/ent/schema/agent.go#L26)）：

```go
index.Fields("position_key", "agent_variant").Unique(),
```

DB 实况确认该索引为**全量唯一索引**（无部分谓词、不含 deleted_at）：

```
agent_position_key_agent_variant | CREATE UNIQUE INDEX ... ON agents USING btree (position_key, agent_variant)
```

设计意图是"一个岗位槽位一个在任 agent（按变体区分）"，但实现把两类不应受限的行也纳入唯一键：

- **无岗位 agent**：`position_key` 默认 `''`（schema agent.go:57 `Default("")`），biz 层 `create`（[agent_usecase.go:264](file:///f:/myproject/aranea-agents/internal/biz/agent_usecase.go#L264)）不做默认改写，直传空值 → 全部无岗位 agent 共享同一个键空间；
- **软删除 agent**：删除只是 `SetDeletedAt(now)`，索引不排除 `deleted_at<>''` 的行 → 墓碑行永久占槽。

**复现（真机事务内验证后回滚，未污染数据）**：

```sql
-- 模式 A：被软删墓碑行永久阻塞
INSERT INTO agents (..., position_key, agent_variant, ...) VALUES (..., '', '', ...);
-- ERROR: duplicate key value violates unique constraint "agent_position_key_agent_variant"
-- DETAIL: Key (position_key, agent_variant)=(, ) already exists.   ← 来自 1 条 deleted_at<>'' 的遗留行

-- 模式 B：('', 'general') 全库仅可存在一个
INSERT ... ('', 'general') -- 第 1 次成功
INSERT ... ('', 'general') -- 第 2 次 23505
```

**模式 C（推导成立）**：岗位 agent 软删后，同 `(position_key, agent_variant)` 无法重建——删除行继续占槽。

**消费方排查（决定修复安全边界）**：

- 全仓 grep 无任何生产读路径按 `(position_key, agent_variant)` 复合查询（`PositionKeyEQ`/`AgentVariantEQ` 仅存在于 ent 生成代码）；dept_lead 查找走 `agent_variant = 'dept_lead' AND deleted_at = ''` 单列（[seed_system_admin.go:623](file:///f:/myproject/aranea-agents/internal/data/seed_system_admin.go#L623)）；
- **无 RestoreAgent 路径**（grep 无命中）——排除软删行不会产生"恢复撞键"场景；
- 全代码库其实已经在绕这个索引：copy 路径 `AgentVariant = AgentKey`（[agent_duplicate.go:53](file:///f:/myproject/aranea-agents/internal/biz/agent_duplicate.go#L53)）、pack 导入 `PositionKey = spec.Key` 注释明言"避免唯一约束冲突"（[importer.go:377](file:///f:/myproject/aranea-agents/internal/biz/pack/importer.go#L377)）、seed 全部使用非空 position_key。**既有规避模式佐证索引设计过宽**。

**历史数据安全验证**：

```sql
SELECT position_key, agent_variant, COUNT(*) FROM agents
WHERE position_key<>'' AND deleted_at='' GROUP BY 1,2 HAVING COUNT(*)>1;
-- 0 rows → 在候选索引范围内无重复，可直接建部分唯一索引
```

### 1.2 解决方案

**方案 A（推荐）：部分唯一索引 + Ent schema 同步 + 版本化 DDL 迁移**

1. DDL 迁移（新增版本，幂等）：

```sql
DROP INDEX IF EXISTS agent_position_key_agent_variant;
CREATE UNIQUE INDEX IF NOT EXISTS agent_position_key_agent_variant
    ON agents (position_key, agent_variant)
    WHERE position_key <> '' AND deleted_at = '';
```

2. Ent schema 同步（[agent.go:26](file:///f:/myproject/aranea-agents/internal/data/ent/schema/agent.go#L26)），保证 fresh install 直接建出部分索引：

```go
index.Fields("position_key", "agent_variant").Unique().
    Annotations(entsql.IndexWhere("position_key <> '' AND deleted_at = ''")),
```

3. 按项目惯例在 `ddl_migration_registry.go` 末尾注册递增版本迁移（受 `TestMigrationVersionsGloballyUnique` 守卫）。

**为什么不选报告中的原方案**（仅 `WHERE position_key <> ''`）：不解决模式 C——岗位 agent 删除后同岗同变体仍无法重建，而"删旧换新"正是岗位 agent 的正常生命周期。

**为什么不选应用层方案**（无岗位时 variant 默认 agent_key）：治标；不解决模式 C；且把"规避模式"从 2 处扩散到 3 处，与"从根本解决"原则相悖。

### 1.3 副作用评审

| 风险 | 评估 | 结论 |
|------|------|------|
| Ent auto-migrate 与 DDL 双轨冲突 | 已确认执行顺序：Ent `Schema.Create` 在前（[data.go:821/828](file:///f:/myproject/aranea-agents/internal/data/data.go#L821)），DDL 迁移在后（`ensureSchemaDDL`→`runDDLMigrationsWithDialect`）。同名索引下 Ent 只做存在性检查，不会回改谓词；DDL 后跑，最终态=部分索引。dev 环境 `WithDropIndex(true)` 也不会删同名索引 | 安全 |
| 部分索引的唯一性推断 | `ON CONFLICT (position_key, agent_variant)` 推断点需匹配谓词——全仓 seed 的 `ON CONFLICT` 目标均为 `agent_key`（[seed_system_admin.go:36](file:///f:/myproject/aranea-agents/internal/data/seed_system_admin.go#L36) 等 6 处），无一引用该索引 | 安全 |
| 软删行与活跃行同键共存 | 无读路径受影响（已排查）；唯一会撞键的场景是"恢复删除的 agent"，但该路径不存在 | 安全 |
| 未来新增 RestoreAgent | 恢复 UPDATE 会触发 23505（与活跃行冲突），表现为显式报错而非静默错乱 | 可接受，届时按需处理 |
| 历史数据建索引失败 | 已实测候选范围无重复（0 rows） | 安全 |

### 1.4 有效性评审

- 模式 A（''+'' 被墓碑阻塞）：`position_key=''` 行整体出键 → 消除；
- 模式 B（''+general 全库唯一）：同上 → 消除；
- 模式 C（岗位删后重建）：`deleted_at<>''` 行出键 → 消除；
- 设计意图（一岗一变体一在任）：`position_key<>'' AND deleted_at=''` 范围内仍唯一 → **保留**。

结论：真正解决，且语义比原状更精确。

### 1.5 验证方法

1. `ARANEA_TEST_PG_DSN` 指向测试库，迁移幂等性测试（重跑 2 次）；
2. 集成用例：创建无岗位 agent ×2（variant 相同）→ 均成功；创建岗位 agent → 删除 → 同岗同变体重建 → 成功；创建同岗同变体活跃 agent ×2 → 第二次 400；
3. 真机复测 01-agent-mgmt 模块 agt 创建用例。

---

## 二、BUG-02（P1）：会话删除恒 500

### 2.1 根因证据链

**F1（致命，直接原因）**：[cascade_delete.go:112](file:///f:/myproject/aranea-agents/internal/data/cascade_delete.go#L112)

```go
// Hard-delete messages
if _, err := execer.ExecContext(txCtx, ...(`DELETE FROM messages WHERE session_id = ?`), sessionID); err != nil {
```

- `messages` 表已被迁移 `20260902_drop_messages_subsystem` 删除（schema_migrations 已应用；information_schema 实况无此表）；
- 真机复现：`BEGIN; DELETE FROM session_turns ...; DELETE FROM messages ...` → `ERROR: relation "messages" does not exist`（42P01）；
- 调用链：`DeleteSession`（[session_repo.go:436](file:///f:/myproject/aranea-agents/internal/data/session_repo.go#L436)）与 `DeleteSessionsByIDs`（[session_repo_batch.go:102](file:///f:/myproject/aranea-agents/internal/data/session_repo_batch.go#L102)）均进 `cascadeDeleteBySession` → 事务整体回滚 → **单删/批删 100% 失败 500**；
- 演进时间线：744e49209（06-25）清理 event_store 时删了级联里的 event_store 行，但**漏删 messages 行**；后续 v2 表成为聊天真相后，级联再未同步。

**F2（隐性，数据泄漏）**：级联清单停留在 v1 时代，以下**活跃** session 域表永不清理（真机行数/体积实况）：

| 表 | 实况 | 性质 |
|----|------|------|
| `steps_v2` | 15,885 行 / 23MB | v2 聊天真相 |
| `trpc_session_events` | 22,026 行 / 58MB | 框架事件持久化 |
| `event_delivery_outbox` | 13,264 行 | B-06 关键事件 outbox |
| `tool_invocation_audit` | 18,584 行 | 工具调用审计 |
| `turns_v2` | 752 行 | v2 轮次 |
| `tasks_v2` | 524 行 | v2 任务 |
| `member_sessions_v2` | 376 行 | 团队成员会话 |
| `model_token_usage_events` | 403 行 | token 计量 |
| `trpc_session_states` / `trpc_session_summaries` / `trpc_session_track_events` / `session_summaries` / `memory_event_marks` | 少量/0 | 框架/摘要/水位 |

（以上 15 张表均确认含 `session_id` 列；`sessions_v2`/`event_store`/`activities` 为无写入方的死表，**不得**进级联——fresh install 已 drop，引用即 42P01 重蹈 F1。）

**F3（设计冲突，评审发现）**：`RestoreSession`（[session_repo.go:348](file:///f:/myproject/aranea-agents/internal/data/session_repo.go#L348)）可恢复软删会话，但级联在删除时已硬删全部历史 → 恢复回来的是空壳会话。该冲突是存量行为（messages 时代即如此），非本次回归引入。

**F4（旁证）**：`activities` 表被 20261012 drop 后仍以 272 行存活——data.go:419 启用 Ent `Schema.Create(WithDropIndex)`，任何含 Activity schema 的**旧二进制**启动即重建退表。部署回滚会静默复活已退役表结构。

### 2.2 解决方案

1. **F1**：删除 cascade_delete.go:111-114 的 `messages` 段（注释与代码）；
2. **F2**：级联补删以下表（同事务内，顺序无关）：`turns_v2`、`steps_v2`、`tasks_v2`、`trpc_session_events`、`trpc_session_states`、`trpc_session_summaries`、`trpc_session_track_events`、`session_summaries`、`member_sessions_v2`、`event_delivery_outbox`、`memory_event_marks`；
3. **回归测试**：新增 PG 集成测试——建会话→写入 v2 全链路数据→删除→逐表 `COUNT=0` 断言 + 对不存在表名做 information_schema 正向校验（防再次不同步）；
4. **防复发规则**：把"表结构变更（drop/rename）必须全局搜索 `cascade_delete.go` 并同步"写入 `.trae/rules/project_rules.md`；
5. **F4 处置（附带）**：新建迁移 drop `activities`/`event_store`/`sessions_v2` 死表（IF EXISTS 幂等），并在部署纪律中明确"禁止旧二进制连接现网库"。

**已裁定 ①（审计/计量表去留，2026-08-17）**：**A 保留**——`tool_invocation_audit` 与 `model_token_usage_events` **不进级联**；审计记录比会话长寿，会话删除后仍可按 session_id 追溯（sessions 永不硬删，孤儿行可关联回墓碑行）。

**已裁定 ②（RestoreSession 语义，2026-08-17）**：**A 维持现状**——删除即硬删历史，Restore 仅恢复会话壳；本次仅修 F1/F2，并在 RestoreSession 接口文档/注释中明示"恢复≠恢复历史"。纯软删+GC 如未来需要另立需求单独立项。

### 2.3 副作用评审

| 风险 | 评估 | 结论 |
|------|------|------|
| 删 `trpc_session_events` 影响 checkpoint 恢复 | checkpoint 元数据在 `session_run_checkpoints`（级联已删）；events 的消费者是会话回放/历史装配，会话删除后无消费者 | 安全 |
| 删 `event_delivery_outbox` 未发布事件 | outbox 事件以 session 为投递边界，会话删除后事件无处可投 | 删除正确 |
| 删 `member_sessions_v2` 影响 team 恢复 | member session 是 team run 的成员态，会话删除即团队会话终止；team_runs 侧不依赖 member_sessions_v2 做跨会话恢复（v2_recovery_repo 按 run 域查） | 安全 |
| 大表单事务删除锁时长 | 最坏 case：trpc_session_events 单会话量级为千~万行，PG 索引删除毫秒~百毫秒级；DeleteSession 本就单事务硬删 13 张表，量级同阶 | 可接受；若后续出现超长会话可改分批 |
| `ON DELETE CASCADE` 外键 | 仅 `event_store`、`session_run_checkpoints` 有 FK CASCADE 且指向 sessions 硬删（永不发生），不冲突 | 安全 |
| 死表误加级联 | 方案明确排除 `activities`/`event_store`/`sessions_v2`（fresh install 不存在） | 已规避 |
| 审计表保留（若选 A） | 孤儿审计行按 session_id 可关联回墓碑会话行（sessions 永不硬删），可追溯 | 安全 |

### 2.4 有效性评审

- F1：删除出错语句 → 删除链路恢复 200。真机复现已证明 42P01 是唯一失败点（其前 8 条 DELETE 均成功，其后语句同模式无表名风险——已逐一对照 information_schema 验证 13 张被引用表全部存在）；
- F2：级联覆盖全部活跃 session 域表（以 information_schema 全量 `session_id` 列扫描为准绳，而非记忆）→ 无遗漏；
- 回归测试把"表存在性"变成 CI 断言 → 同类不同步不再漏网。

结论：真正解决。

### 2.5 验证方法

1. 修复后真机复测：创建会话→发消息→调用工具→删除会话→期望 200；
2. SQL 抽查 15 张表 `WHERE session_id=<已删id>` 全为 0（审计表若选 A 则豁免）；
3. 批量删除用例复测（session_repo_batch.go 路径）；
4. `go build ./cmd/... ./internal/...` + PG 集成测试（`ARANEA_TEST_PG_DSN` 姿势见项目记忆）。

---

## 三、BUG-MON-A（P2）：破坏性操作的澄清被自动代答为「取消」

### 3.1 根因证据链

**事故链路**（真机 10-monitor-scenario run 实测）：用户请求"注入故障"→ 意图识别产出澄清问题（推荐答案 = "Cancel the injection"）且 **risk_flags 无 destructive** → [chat_clarify_gate.go:118](file:///f:/myproject/aranea-agents/internal/service/chat_clarify_gate.go#L118) `!HasHighRiskFlag() && ClarificationAllRecommended(questions)` 成立 → `autoResolveClarification` 按推荐自动作答 → 模型收到"用户已取消"的上下文 → 顺从取消。HITL 确认门禁（`requires_confirmation=true`）甚至未到达——**用户的破坏性请求被系统静默否决，且全程无人工参与**。

**双重根因**：

1. **意图 prompt 缺陷**：[pass.go:125/137](file:///f:/myproject/aranea-agents/internal/agent/intent/pass.go#L125) 的 risk_flags 示例仅列 `touches_auth/migrations`（coding）与 `sensitive_data/compliance`（general）——`destructive`/`irreversible` 两个高危标记（定义于 [pass.go:61-68](file:///f:/myproject/aranea-agents/internal/agent/intent/pass.go#L61)）**从未在 prompt 中出现**，LLM 不知道它们存在；
2. **autoResolve 无否决守卫**：当推荐答案本身是对用户请求的**取消/否定**时，"按推荐假设前进"的语义从"代用户选一个推进路径"退化为"代用户否决自己的请求"——这超出了假设式前进的合法边界，但代码无任何识别手段。

### 3.2 解决方案（三层防御，纵深互补）

**L1 prompt 补全**（[pass.go](file:///f:/myproject/aranea-agents/internal/agent/intent/pass.go) 两个 system prompt 同步改）：
risk_flags 示例改为：`e.g. touches_auth, migrations, destructive, irreversible, or []`，并追加规则："当请求会导致数据删除、服务中断、故障注入、配置覆盖等破坏性或不可逆后果时，必须包含 `destructive`；后果无法撤销时同时包含 `irreversible`。"

**L2 确定性强制打标**（`runWithSystem` 解析产物后，[pass.go:347](file:///f:/myproject/aranea-agents/internal/agent/intent/pass.go#L347) 之后）：
对用户原文做破坏性模式匹配（注入故障/fault inject/删除/清空/重置/重启/shutdown/kill/drop/wipe 等窄表），命中则向 `art.RiskFlags` 补 `destructive`（幂等去重）。该层是 LLM 漏标的兜底，纯函数、可单测。放在 intent 包（而非 gate）的原因：产物会被 `WrapUserMessage` 注入主模型上下文，主模型看到 destructive 标记也会更谨慎——不止服务澄清门。

**L3 推荐否决守卫**（[chat_clarify_gate.go:118](file:///f:/myproject/aranea-agents/internal/service/chat_clarify_gate.go#L118) 前）：
`ClarificationAllRecommended` 之外增加 `!recommendedLooksLikeCancellation(questions)`——任一问题的推荐答案命中取消语义模式（cancel/取消/放弃/不执行/不要/否/stop/abort 等，大小写不敏感）时，放弃自动代答、走挂起弹卡。即使 L1+L2 全失效，本层也能独立拦截本次事故形态（推荐="Cancel the injection"）。

### 3.3 副作用评审

| 风险 | 评估 | 结论 |
|------|------|------|
| L2 关键词误伤 | "删除"出现在非破坏性语境（如"帮我删除注释"——这本来就是 destructive，标了也对；真正误伤如"删除线怎么加"）→ 后果是澄清挂起弹卡变多 | 方向安全（fail-closed），词表保持窄表，误报成本=用户多点一次确认 |
| L1 使更多请求挂起 | 运维 agent 的破坏性请求本应走 HITL——这正是产品语义 | 符合预期 |
| L3 误伤正常"推荐取消"场景 | 极少见：LLM 推荐取消通常仅当它判断请求有风险——此时交还人工恰是正确行为 | 可接受 |
| 三层叠加导致"完全不自动代答" | 仅命中词表/取消推荐的请求受影响；普通澄清（选环境、选参数）仍自动前进 | 影响面受控 |

### 3.4 有效性评审

本次事故的完整路径被三层分别独立拦截：
- L1：LLM 正确打 destructive → 不 autoResolve；
- L2（假设 L1 失效）："注入故障"命中词表 → 强制打标 → 不 autoResolve；
- L3（假设 L1、L2 全失效）：推荐="Cancel the injection" 命中取消模式 → 不 autoResolve。

任一单层生效即消除事故；三层同时失效的概率积极低。结论：真正解决，且对未知变体（其他高危操作）有泛化能力（L2/L3 不依赖具体工具名）。

### 3.5 验证方法

1. 单测：L2 词表命中/未命中/幂等；L3 各语言取消模式；
2. 真机复测 10-monitor-scenario 原 prompt（"注入 sw1 eth1 故障"不明指节点）→ 期望澄清挂起弹卡而非自动取消；
3. 回归：普通澄清场景（推荐为推进选项）仍自动前进。

---

## 四、BUG-MON-C（P2）：端口级故障不产生告警

### 4.1 根因证据链（twinmonitor 侧）

**监控模型现状**：

- linemonitor 的"线路"探测 = 对线路 `target_ip` 做 ICMP ping（默认）/HTTP/TCP（[prober.go:74-83](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/prober.go#L74)）；真机演练中线路目标为**设备管理面地址**——sw1 eth1（数据面端口）down 不影响管理面可达性 → 无探测失败 → 无事件 → 无告警（链路：probe fail → NATS line.events → monitoralarm → alarm_events）；
- SNMP 探测路径存在但语义不符：[prober.go:425](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/prober.go#L425) 是**带宽探测**，`ifOperStatus` 仅用于"选第一个 up 的接口"（`pickUpInterface`，prober.go:525）——**目标端口 down 后它会自动换到别的 up 接口继续测带宽，恰好掩盖端口故障**；
- 端口级数据其实已采集：collector-snmp-ssh 的 profile 定义了 `if_oper_status`（[snmp_profile_repo.go:680](file:///f:/myproject/twinmonitor/TwinServer/app/collector-snmp-ssh/internal/data/snmp_profile_repo.go#L680)），visualization 也消费它做展示（datasource_providers_instance.go:438）——**采集在、展示在，唯独无告警规则消费**。

结论：不是采集缺口，是**告警模型粒度缺口**——告警只挂在"线路可达性"上，没有"指定设备指定端口状态"这一告警源。

### 4.2 解决方案（已裁定：(b) 拓扑自动生成；2026-08-17 补充代码实证设计）

**拓扑数据源选型（三选一，已逐一核实代码）**：

| 候选源 | 实况 | 结论 |
|--------|------|------|
| admin `topo_projects.doc` | 画板式自由 JSON（`{devices:[],links:[]}`，[topo_repo.go:44-45](file:///f:/myproject/twinmonitor/TwinServer/app/admin/internal/data/topo_repo.go#L44)），links 无强类型端口字段 | 不可靠，排除 |
| visualization `topology_links` | 仅节点间连接关系，**无任何端口语义**（[topology_link.go:32-43](file:///f:/myproject/twinmonitor/TwinServer/app/visualization/internal/data/ent/schema/topology_link.go#L32)） | 无端口信息，排除 |
| **linemonitor 嗅探拓扑** | LLDP/CDP 邻居表读取，本地端口名经 ifName 解析（`NeighborEntry.LocalPort`，[discovery.go:284](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/discovery.go#L284)；ifName walk 见 [topology_snmp.go:65-76](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/topology_snmp.go#L65)）；设备 SNMP 凭据经 `device_protocol_bindings` 现成解析（[discovery_topology.go:92-99](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/discovery_topology.go#L92)） | **唯一含真实端口语义且凭据现成的源，选用** |

**已存在可直接复用的基础设施（代码实证）**：

1. **钉端口探测能力已就绪**：[prober.go:407](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/prober.go#L407) `probe_params.snmp.if_index` 指定后，`probeSNMP` 定点 GET 该端口 ifOperStatus，`operStatus != up(1)` 即判探测失败（prober.go:477-483）——**不需要新增探测类型**，事故场景要的"eth1 down → 探测失败"语义现成；
2. **配置模型现成**：`monitor_configs(line_id, probe_protocol, protocol_binding_id, probe_params JSONB)`（[main_db.go:174](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/data/main_db.go#L174)），SNMP 凭据走 protocol_binding_id → snmp_configs；
3. **热加载现成**：ProbeEngine 实现 `RefreshLine/RemoveLine/Reload`（[probe_engine.go:36](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/biz/probe_engine.go#L36)），自动建线无需重启；
4. **告警链现成**：探测失败 → 状态机 → line.events（NATS）→ monitoralarm，零新管道。

**需新建的部件（4 个）**：

1. **端口名持久化**：`discovered_links` 目前只存单端 `switch_port`（[discovery_repo.go:148](file:///f:/myproject/twinmonitor/TwinServer/app/linemonitor/internal/data/discovery_repo.go#L148)）；inter_switch 链路的对端端口（`NeighborEntry.PortDesc`）在构建期有、落库时丢。加列 `local_port`/`peer_port`（IF EXISTS 幂等迁移），TopologyBuilder 构建 inter_switch 链路时写入两端端口名；
2. **PortProbeGenerator（新 biz 组件）**：每轮 `RebuildSegment` 完成后触发（拓扑每轮全量重建，天然自刷新）：扫描 inter_switch 链路两端 (device, port) → ifName→ifIndex 解析（复用 topology_snmp reader 的 ifName walk，**生成时解析**——设备重启 ifIndex 漂移后下轮发现自愈）→ 按端点 upsert：
   - line：`name=auto:{device}:{port}`（auto: 前缀隔离命名空间，避免与手工线路冲突），`target_ip=设备 IP`，标记 source=auto_discovery；
   - monitor_config：`probe_protocol=SNMP` + 设备现有 binding + `probe_params={"snmp":{"if_index":N,"host":ip}}`，周期/重试按模板；**带宽阈值置空**（纯状态线路不产生 bandwidth_exceeded 噪音）；
   - 调 `RefreshLine` 热加载；
3. **生命周期管理**：链路连续 N 轮（建议 3 轮）未再发现 → 对应自动线路 disable（**不删除**——保留事件历史与告警可追溯）；重现则 re-enable；
4. **生成上限护栏**：自动线路总数上限（如 500）防爆量，超限记 warn 并截断。

**部署前提（非代码障碍，写入演练 SOP）**：被监控设备 SNMP 可达且 community 已配（GNS3 IOS 镜像需 `snmp-server community` + CDP 开启）；设备已录入 monitor_assets/device_ext_switch 并绑定 SNMP 协议；网段发现（lan_segments）已配置。

### 4.3 副作用评审

| 风险 | 评估 | 结论 |
|------|------|------|
| 自动建线数量膨胀 → 探测压力 | 仅 inter_switch 链路端点（每链路 ≤2 端），中小拓扑数十条；每线每周期 1 个 SNMP GET；探测池上限 100（probe_engine.go:21）+ 生成护栏 500 | 可控 |
| 发现抖动 → 线路 enable/disable 抖动 | LLDP 邻居短暂消失会致链路重建波动；以"连续 3 轮缺失才 disable"抑制 | 已有抑制手段 |
| ifIndex 漂移（设备重启重排） | 生成时按 ifName 解析，发现每轮重建 → 下轮自动重解析自愈 | 架构性免疫 |
| 与手工线路冲突 | `auto:` 命名前缀 + 线路 name 唯一约束（ErrNameExists，ports.go:23）隔离 | 已规避 |
| 带宽指标噪音 | 自动线路带宽阈值置空，probeSNMP 差分指标不评估、不产生 bandwidth_exceeded | 已规避 |
| 误删除用户数据 | 生命周期只 disable 不 delete；自动/手工命名空间隔离 | 安全 |
| 多租户 | 自动线路继承网段/设备 tenant_id | 一致 |

### 4.4 有效性评审

- 直接覆盖事故场景：sw1/sw2 录入 + 网段发现跑通 → 自动生成 `auto:sw1:Ethernet1` 线路 → fault_inject eth1 down → 下一探测周期 operStatus=down(2) → outage 事件 → monitoralarm 告警 → Aranea 侧 twin_alarm 可见 → **监控闭环补全，且全程零人工配置**；
- 拓扑变化自跟随：新增链路/设备下轮发现即自动纳管，优于手工配置（会过时）；
- **如实说明的局限**：仅覆盖 SNMP 可达且被发现的端口；CDP/LLDP 关闭或 SNMP 未配的设备端口仍无覆盖——这是监控覆盖度问题（SOP 保证），非方案缺陷；
- 与 BUG-MON-A 修复叠加后，完整闭环"注入→告警→清除→恢复确认"每一环都有真实信号。

结论：真正解决断链环节。

### 4.5 验证方法

1. GNS3 sw1/sw2 开 SNMP+CDP → 录入资产并绑定 SNMP → 配网段发现 → 跑一轮扫描；
2. 断言：`discovered_links` 出现 inter_switch 链路且 `local_port`/`peer_port` 非空；自动生成 `auto:*` 线路与 monitor_config；
3. gns3_fault_inject eth1 down → 1-2 探测周期内出现端口告警；
4. gns3_fault_clear → 告警自动 recovered；
5. 抖动用例：停 CDP 3 轮内线路不 disable，第 4 轮 disable；
6. 复跑 10-monitor-scenario 端到端。

---

## 五、BUG-G1（P2）：残缺图 visualize 500

### 5.1 根因证据链

- 失败图 `f76d092c`（DB 实况）：5 个 `type:"function"` 节点 **`func_ref` 全空**，有 entry/edges，非空图；
- 构建守卫本身存在：空 nodes/缺 entry 在 [builder.go:201-206](file:///f:/myproject/aranea-agents/internal/graph/trpc/builder.go#L201) 返回 `apierror.BadRequest`；func_ref 空在 [node_wiring.go:182](file:///f:/myproject/aranea-agents/internal/graph/trpc/node_wiring.go#L182) 返回 `apierror.BadRequest("node %q type function requires Func or ... FuncRef")`；
- **压平点**：[runtime_adapter.go:696](file:///f:/myproject/aranea-agents/internal/graph/adapter/runtime_adapter.go#L696) `Visualize` 把**一切** Build 错误重写为 `apierror.Internal(...)` → BadRequest(400) 被吞成 Internal(500)；
- 报告归因"panic 上抛"不完全准确——实测路径是 error 被错误码压平，不是 panic（亦未见 recover 日志）。修复不变。

### 5.2 解决方案

```go
// runtime_adapter.go Visualize
g, _, err := graphtrpc.BuildStateGraphWithRegistryAndLogger(ctx, cfg, f.registry, &f.resolvers, f.lg)
if err != nil {
    // apierror.Wrap 不重复包装：已是 *apierror.Error（如 BadRequest）则原样透传，
    // 非 apierror（框架原生 error）才按 Internal 包装。
    return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainGraph)
}
```

`apierror.Wrap` 的防双包语义见 [apierror.go:136-146](file:///f:/myproject/aranea-agents/pkg/apierror/apierror.go#L136)。同时排查 `Visualize` 之外 `runtime_adapter.go` 内是否还有其他 `apierror.Internal` 压平点（BuildRuntime/ResumeRuntime 等）一并改 Wrap。

### 5.3 副作用评审

- BadRequest 消息（如"node \"dispatch\" type function requires Func..."）将原样返回客户端——这些消息本就是为调用方设计的契约文案（BadRequest 语义），且 `ToKratos` 只对 CodeInternal 做消息脱敏（[apierror.go:176-178](file:///f:/myproject/aranea-agents/pkg/apierror/apierror.go#L176)），无内部信息泄漏新增面；
- 前端画布收到 400 + 明确消息后可提示"图定义不完整"而非通用 500——体验改善即修复目标；
- 不改框架 vendored 代码（遵守 FW-R1~R3），改动全部在自有 adapter 层。

### 5.4 有效性评审

三类残缺图（空 nodes / 缺 entry / func_ref 空）在 builder 层均有 BadRequest 守卫（已逐行确认），透传后全部恢复 400 语义。对真正非预期的框架 error 仍按 Internal 包装，500 保留给真异常。结论：真正解决。

### 5.5 验证方法

单测：构造 func_ref 空图调 Visualize → 断言 apierror code=bad_request；真机复测 GRAPH-04 用例（f76d092c）→ 期望 400。

---

## 六、BUG-CLI-01（P2）：非法命令静默退出

### 6.1 根因

[cmd/aranea/main.go:79](file:///f:/myproject/aranea-agents/cmd/aranea/main.go#L79) `SilenceErrors: true`（cobra 不打印）+ [main.go:50-52](file:///f:/myproject/aranea-agents/cmd/aranea/main.go#L50) `if err := execute(...); err != nil { os.Exit(cli.ExitCodeOf(err)) }`（main 也不打印）→ 双缺位 → exit 3 零输出。

### 6.2 方案

```go
if err := execute(ctx, bi); err != nil {
    fmt.Fprintln(os.Stderr, "aranea:", err.Error())
    os.Exit(cli.ExitCodeOf(err))
}
```

### 6.3 副作用评审

排查了子命令错误处理模式（`internal/cli/cmd/*`）：错误均通过 `return err` 上抛，未发现"先自行打印再 return"的双打路径（cobra 层 SilenceErrors 保证中间不打印）。唯一已打印场景是 REPL 内部交互错误（不进该退出路径）。结论：无重复打印风险。

### 6.4 有效性

非法命令 → stderr 输出 `aranea: unknown command "xxx" for "aranea"` + exit 3。直接消除。验证：`aranea foobar; echo $LASTEXITCODE` → 有输出且为 3。

---

## 七、ISSUE-G2 / ISSUE-G3（P3）

### 7.1 G2 无检查点执行返回 404

- 根因：[graph_execution_usecase.go:222-224](file:///f:/myproject/aranea-agents/internal/biz/graph_execution_usecase.go#L222) `LineageID=="" → ErrNotFound`，与"执行不存在"共用 404；
- 方案：checkpoints/state-snapshot 两个端点的用例层先取执行（不存在才 404），存在但 `LineageID==""` 时返回 200 空集（`items: []`）；
- 副作用：前端原依赖 404 判"无检查点"的逻辑需改判空集——查前端消费点后同步；语义上"存在但无数据"用空集表达本就比 404 准确；
- 有效性：两种状态（无执行/无检查点）从此可分。

### 7.2 G3 time-travel step_index=0 被拒

- 根因：[graph.proto:269](file:///f:/myproject/aranea-agents/api/kratos/graph/v1/graph.proto#L269) `int32 step_index = 2 [(google.api.field_behavior) = REQUIRED]`——proto3 标量零值=未设置，protovalidate 拒 0；
- 方案：去掉 step_index 的 REQUIRED（保留 execution_id 的），biz 层校验 `step_index >= 0`；改后 `cd api && buf generate` 重生成（项目惯例）；
- 副作用：不传 step_index 时零值 0 语义="第 0 步"——与显式传 0 不可区分；由于 0 本就是合法目标步，该歧义无实际危害；
- 有效性：time-travel 到首个业务步骤可用。

---

## 八、BUG-MON-B（P3）：持久化授权无 TTL

### 8.1 根因证据链

- schema（[ent/schema/tool_grant.go](file:///f:/myproject/aranea-agents/internal/data/ent/schema/tool_grant.go)）：`id/agent_id/tool_key/granted_by/created_at`——**无 expires_at**；
- 写入链：用户选"始终允许"→ `applyGrantOutcome` 的 `ApproveAlways` 分支（[tool_confirmation.go:259-275](file:///f:/myproject/aranea-agents/internal/agent/tool_confirmation.go#L259)）→ `ToolUC.GrantTool` → DB 永存；
- 读取链：[tool_grant.go:36-43](file:///f:/myproject/aranea-agents/internal/data/tool_grant.go#L36) 纯存在性查询，无时效过滤；
- 对比：session 级授权（内存层）**已有 24h TTL**（[tool_grant_store.go:14](file:///f:/myproject/aranea-agents/internal/agent/tool_grant_store.go#L14)）——持久层反而裸奔；
- 真机实证：`tool_grants` 表 4 行，最老 `shell_exec`（default_user，2026-08-05）残留 12 天，含 `hostexec_exec_command`/`playwright_browser_navigate` 高危工具，全部对 spirit agent 永久生效。

### 8.2 解决方案

1. schema 加 `expires_at`（string，RFC3339，`''`= 不过期——仅保留给未来显式"永不过期"选项）；
2. `GrantTool` 写入时按默认 TTL 计算 expires_at（**已裁定：默认 72h**，2026-08-17——覆盖一次演练周期又不跨周）；
3. `HasToolGrant` 增加 `expires_at = '' OR expires_at > now` 过滤；过期行由定时任务或读径惰性删除（推荐读径惰性删 + cron 日扫，双保险）；
4. `ListToolGrants` 返回 expires_at，前端授权管理页显示剩余时效；
5. 存量行迁移：版本化迁移把 `created_at` 早于 N 天前的行按 `created_at + 默认TTL` 回填 expires_at（多数已立即过期，惰性删除自然清理）；
6. SOP 补充：演练后清理授权的操作手册条目（文档化 `DELETE FROM tool_grants WHERE ...` 三步校验姿势）。

### 8.3 副作用评审

| 风险 | 评估 | 结论 |
|------|------|------|
| 存量"始终允许"用户预期被打破 | 72h/7d 后重新弹确认——这正是安全语义修复的目的；前端显示剩余时效降低困惑 | 可接受 |
| 过期判断时钟源 | 统一用 DB 当前时间或应用 UTC now（项目 nowRFC3339 惯例），字符串比较需同格式 | 按项目惯例实现 |
| 高频工具到期打断长任务 | 到期只影响"下一次调用"的确认弹窗，不打断进行中的 invocation | 安全 |
| 字符串时间比较正确性 | RFC3339 固定宽度 UTC（项目全域惯例）字典序=时间序 | 安全 |

### 8.4 有效性

授权获得确定时界；过期后 `HasToolGrant=false` → HITL 确认重新生效 → 演练残留场景（8-15 授权 8-17 仍生效）不再可能。结论：真正解决。

---

## 九、PERF-F1（P3）：/v1/model-catalog/providers ~510ms

### 9.1 根因

[model_registry.go:120](file:///f:/myproject/aranea-agents/internal/biz/model_registry.go#L120) 每请求 `st.LoadDirectory()` → [store.go:164-179](file:///f:/myproject/aranea-agents/internal/modelregistry/store.go#L164) `os.ReadFile` + `json.Unmarshal` **全量**反序列化目录文件——真机实测该文件 **2.88MB**（容器内 `/app/data/model-catalog/current.json`）。同模式影响 `ListModels`/`SearchRaw` 等全部目录读接口。

### 9.2 方案

`Store` 增加目录缓存：`{dir, meta, modTime, size}` + RWMutex；`LoadDirectory` 先 `os.Stat`，mtime+size 与缓存一致直接返回缓存；`SaveDirectory`/Sync 成功后主动失效（双保险，不依赖 stat 分辨率）。

### 9.3 副作用评审

- 外部手改文件：mtime 变化即被捕获（stat 每请求一次，µs 级）；
- 并发：RWMutex 读写锁，读多写少场景无争用问题；
- 内存：常驻一份 Directory（2.88MB JSON → 数十 MB Go 对象）——单实例可接受；
- 时钟回拨/mtime 精度：SaveDirectory 主动失效覆盖自身写入；外部写入靠 stat，容器内文件系统 mtime 可靠。

### 9.4 有效性

预期 providers 接口降至 <50ms（内存索引+分页）；量级估算：当前 510ms 的 90%+ 是重复 IO+反序列化。结论：真正解决。验证：修复后连测 10 次取 p95，对比基线 510ms。

---

## 十、PERF-S1（P3）：Spirit 系统提示词 24k token/轮

### 10.1 现状证据

- Spirit 静态 prompt 文件实测仅 ~8.4KB（DECISION 3290B / CAPABILITIES 2560B / IDENTITY 1236B / orchestrator 999B / dept_lead 327B ≈ 2-3k token）——**静态部分不是主因**；
- `agent_runtime_settings.tools_enabled=true` + 平台 108 个工具——每个工具声明（name+description+JSON schema）约 100-300 token，**全量注入即 10-30k token，与实测 24199 高度吻合**；
- 高度疑似主因：工具声明全量注入 + 会话历史累积。

### 10.2 方案方向（先取证后实施）

1. **取证**（前置，一次抓包）：用 `test/ts10-gns3/llm_relay.py`（:8899 中继）抓一次 Spirit 平凡问答的完整 request，分解 system prompt 构成（工具声明/静态 prompt/记忆注入各占多少）——用数据决定优化顺序，不猜；
2. **工具分级装载**：Spirit 的工具按场景分组（只读查询类常驻；高危/低频类按需经 `read_upstream_deliverable` 式发现机制或两轮制——先小集合规划，命中再扩载）；
3. **provider prompt cache**：deepseek/openai 均支持前缀缓存，确保静态部分（工具 schema+静态 prompt）置于消息前缀且内容稳定（避免每轮注入变化因子如时间戳到前缀区）。

### 10.3 副作用与有效性

分级装载的风险是"该用的工具没被加载"——需配套工具发现机制与回归用例（监控闭环全链路必须通过）；prompt cache 无副作用（纯收益）。有效性待取证数据量化后确认目标（预期 token_in 降 50%+、首 token 延迟显著改善）。

---

## 十一、其余 P3 简评

| 项 | 简评与处置 |
|----|-----------|
| ISSUE-K1 vault 宿主路径容器不可达 | 配置问题非代码缺陷：`F:\...` 宿主路径未挂载进容器。改容器卷映射或停用该 vault 同步，属部署配置修正 |
| /v1/tools/test 平台限制 | 400 语义已清晰；低优：tools 元数据加 `testable=false` 让前端隐藏测试入口 |
| 2 个阿里云 MCP server 配置失效 | 宿主 exe 路径容器不可达（同 K1 性质）；清理或改容器内可达配置。可复用 2026-08-17 方案 A 的 MCPVersionHash 机制热更新 |
| knowledge relation extract 超时 | provider 侧稳定性问题（context deadline）；非代码缺陷，观察项 |
| ListTools `limit` 不生效 | 仅 `page_size` 生效；低优：service 层加 limit→page_size 别名映射 |

---

## 十二、落地顺序与验证计划

**批次 1（P1，先行）**：BUG-01（索引迁移+ent 同步）、BUG-02（cascade 修复+回归测试）→ 真机复测 01/02 模块；
**批次 2（P2）**：BUG-MON-A（三层防御）、BUG-G1（Wrap 透传）、BUG-CLI-01（打印）、BUG-MON-C（TwinMonitor 跨仓：discovered_links 端口持久化 + PortProbeGenerator 自动建线 + 生命周期管理）→ 复测 10/04/19 模块；
**批次 3（P3 择要）**：BUG-MON-B（TTL）、PERF-F1（缓存）、G2/G3 → 回归；
**观察项**：PERF-S1 先抓包取证再立项；K1/MCP 配置修正随下次部署窗口。

**全局防复发规则（建议写入 project_rules.md）**：
1. 表结构变更（drop/rename/唯一约束改动）→ 必须全局搜索 `cascade_delete.go`、`ON CONFLICT`、raw SQL 引用点三处并同步；
2. PG 集成回归纳入 CI（`ARANEA_TEST_PG_DSN` 姿势已有项目记忆）；
3. 禁止旧二进制连接现网库（Ent auto-migrate 会复活退役表——F4 教训）。

**用户裁定清单（2026-08-17 全部裁定完毕）**：
1. BUG-02 审计/计量表（tool_invocation_audit、model_token_usage_events）去留：**A 保留**（不进级联）；
2. BUG-02 RestoreSession 语义：**A 维持"恢复≠恢复历史"**（本次仅修 F1/F2；软删+GC 另立项）；
3. BUG-MON-B 默认 TTL：**72h**；
4. BUG-MON-C 端口探测项配置来源：**(b) 拓扑自动生成**（linemonitor 嗅探拓扑为数据源，设计见 §4.2）。
