# ADR-85：tool_invocations 工作区化与 KPI 24h 租户口径

> 状态：已采纳（2026-10-19） ｜ 决策源：tools 端到端审计 A2（`docs/reports/2026-10-18-audit-tools-endtoend.md` §4-A2，初判 defer，用户指令启动） ｜ 关联：ADR-84（工具熔断器持久化）、`internal/data/ent/schema/tool_invocation.go`、迁移 `20261279`

## 背景

`computeToolSummary` 的 24h KPI（调用量/失败率）对 `tool_invocations` 无条件聚合，而同一接口的工具总数已按 `toolWhereClause(q)` 租户过滤——两块口径不一致：单租户部署无碍，多工作空间隔离下租户会在摘要卡看到全平台调用量。

`tool_invocations` schema 原无 `workspace_id` 列，写入链（agent recorder / 确认门禁自录 / testexec 测试执行）均无租户概念。

## 决策

1. **加列 + 索引**：`tool_invocations.workspace_id TEXT NOT NULL DEFAULT ''` + `(workspace_id, started_at)` 索引（24h 租户聚合走该索引）。SQLite 侧由 Ent schema 建列（CLI/测试路径），生产走迁移 `20261279`（Postgres-only Func，幂等）。
2. **backfill**：迁移内 `UPDATE ... FROM agents` 按 `agent_id` 派生存量行；宿主已删除的行保持 `''`（仅系统视图 KPI 可见，不丢数据）。
3. **写入派生**：`RecordToolInvocation` 在 `ToolInvocationWrite.WorkspaceID` 为空时自 agents 表派生（同 `unified_evolution_suggestions` P0-1a 空值派生惯例）。constraint fallback-update（同 ToolCallID chunk 合并）**不改写** workspace —— 它随首次写入的身份行确定。
4. **KPI 过滤语义**：`q.WorkspaceID != ''`（tenant）→ 仅 `workspace_id = ?`；`== ''`（系统调用方）→ 不过滤（全局视图）。租户**不**共享 `''` 下行（与 `tools` 表 `'' OR =` 的 shared+own 语义不同——调用没有「平台共享」概念，强行纳入会泄漏历史计数）。
5. **不加 RLS**：本表租户边界一贯由 service 层断言（`service/tool.go` runs 列表 workspace 过滤），未纳入 `20261011` phase1 RLS 范围，A2 不扩大爆炸半径。
6. **scope 限定**：`tool_invocation_params`（参数旁车）与 `tool_invocation_audit` 不加列——KPI 只读主表；旁车/审计表的租户可见性继续由 service 层经主表/join 控制。

## 后果

- 正面：租户 KPI 与列表/明细口径一致；演进指标（`tool_invocations` 聚合查询方，如 evolution 模块）可按需复用该列。
- 代价：每次调用写入多一次 agents PK 查询（后台 goroutine 内，10s 超时，PK 命中开销可忽略）；写方显式提供 WorkspaceID 时跳过派生。
- 边界：宿主删除后的存量 `''` 行只进系统视图；演进建议同口径（20261212 先例）。
- 兼容：写入结构体仅新增字段，biz port 接口签名不变（内部 struct 扩展，符合 AS-STA-01 对 Internal 结构的演进约束）。

## 替代方案（已否决）

| 方案 | 否决理由 |
|------|---------|
| 只在 service 层对 KPI 文档化「全局口径」（审计原判 defer） | 用户指令明确拉通；口径不一致在多工作空间下是实质信息泄漏 |
| 写入方各自传 WorkspaceID（recorder/确认/testexec 各打各的） | 三处写入源各自演进易再分裂；data 层单点派生是既有先例且对遗漏写入源天然兜底 |
| 对 `tool_invocations` 加 RLS（`workspace_id = '' OR = GUC` 模板） | 超出 A2 目标；本表从未走 RLS，需单独一轮评估 GUC 接线下的行为差异 |
| `metadata_json` 塞 workspace 免加列 | JSON 子查询聚合无法走索引，24h 聚合是热路径 |
