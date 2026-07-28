# 特性开关登记表（Feature Flags Registry）

> 全局参考文档（非模块文档）。登记所有运行时行为开关：环境变量、conf 访问器、配置项。
> **红线：新增开关必须先登记本表，再写代码。** 未登记的开关视为技术债务。

## 治理规则

| # | 规则 | 说明 |
|---|------|------|
| F1 | 新开关必须登记 | 提交代码时同步更新本表（名称/默认值/用途/状态） |
| F2 | 统一读取入口 | 优先在 `internal/conf/features_*.go` 加访问器函数，禁止业务代码散落 `os.Getenv` |
| F3 | 布尔解析统一 | 使用 `conf.parseBoolFlag` 语义（`1/true/yes` 为真），禁止自定义真值表 |
| F4 | 默认值必须文档化 | 默认开/关直接影响生产行为，必须写明 |
| F5 | 临时开关必须有下线计划 | `状态=deprecated` 的开关在下个迭代移除代码 + 从本表删除 |
| F6 | 生产禁开必须标注 | 仅限 dev/staging 的开关在「备注」列写明 |

状态取值：`stable`（长期保留）｜`transitional`（迁移期，完成后下线）｜`deprecated`（待移除）｜`debug`（仅诊断）

---

## 1. conf 集中访问器（internal/conf/features_*.go）

### M58 Prompt Governance（features_pgo.go）

| 开关 | 访问器 | 默认值 | 用途 | 状态 | 备注 |
|------|--------|--------|------|------|------|
| `PGO_DEFAULT_FILES_V2` | `PGODefaultFilesV2()` | **true** | 新 Agent 默认 5 文件提示词集（旧为 9 文件） | transitional | 置 `0` 回退旧桩文件 |
| `PGO_CATEGORY_RESPONSIBILITY_INJECT` | `PGOCategoryResponsibilityInject()` | false | system prompt 注入 `<role_responsibility>` 岗位职责块 | transitional | 灰度中 |
| `PGO_AI_REFINE_V2` | `PGOAIRefineV2()` | false | 统一 `/v1/ai/refine` 端点 + 前端 AIRefineButton | transitional | false 走旧 ai-edit 路径 |
| `PGO_CLI_IMPORT_ENABLED` | `PGOCLIImportEnabled()` | **true** | CLI 注册 `aranea import` 子命令 | stable | 置 `0` 裁剪 CLI |

### DAO 会话表拆分（features_dao.go）

| 开关 | 访问器 | 默认值 | 用途 | 状态 | 备注 |
|------|--------|--------|------|------|------|
| `DAO_SESSION_METRICS_TABLE` | `DAOSessionMetricsTable()` | false | 指标写 `session_metrics` 表而非 `sessions` | transitional | 表拆分迁移期 |
| `DAO_SESSION_RUNTIME_TABLE` | `DAOSessionRuntimeTable()` | false | 运行时状态写 `session_runtime` 表 | transitional | 同上 |
| `DAO_SESSION_DUAL_WRITE` | `DAOSessionDualWrite()` | false | 新旧表双写（迁移对齐期） | transitional | 迁移完成后必须下线 |
| `DAO_VECTOR_PGVECTOR` | `DAOVectorPgVector()` | false | 向量存储用 PgVectorStore 替代 SQLiteVectorStore | transitional | 依赖 Postgres |

### M56 Business Logic Optimization（features_blo.go）

| 开关 | 访问器 | 默认值 | 用途 | 状态 | 备注 |
|------|--------|--------|------|------|------|
| `BLO_UNIFIED_JOB_ENABLED` | `BLOUnifiedJobEnabled()` | false | 统一 BackgroundJob 子系统（BLO-5） | transitional | **Sprint A3 门禁通过前禁止生产开启** |
| `BLO_PENDING_TASK_V2` | `BLOPendingTaskV2()` | false | 非阻塞 HITL（PendingTask 异步，BLO-4） | transitional | |
| `BLO_ESCALATION_V2` | `BLOEscalationV2()` | false | 多信号升级（BLO-2） | transitional | |
| `BLO_INTENT_CLASSIFIER` | `BLOIntentClassifier()` | false | 意图感知准入（BLO-1） | transitional | |
| `BLO_TRIGGER_RULES` | `BLOTriggerRules()` | false | 渠道触发规则（BLO-3） | transitional | |

### M27 Artifact（features_artifact.go）

| 开关 | 访问器 | 默认值 | 用途 | 状态 | 备注 |
|------|--------|--------|------|------|------|
| `FEATURES_LOCAL_REVEAL_ENABLED` | `LocalRevealEnabled()` | false | `POST /v1/system/reveal` 打开本地文件夹 | stable | **仅限本地单机部署，生产禁开** |

### 配置文件开关（非环境变量）

| 配置项 | 访问器 | 默认值 | 用途 | 状态 |
|--------|--------|--------|------|------|
| `server.monitor.process_log_enabled` | `Server.ProcessLogEnabled()` | true | Gateway 过程日志（WS EnvelopeTypeLog） | stable |

---

## 2. 直读环境变量（未收口 conf，技术债务）

> 债务说明：以下开关直接 `os.Getenv` 散落在业务代码中，违反 F2。
> 整改方向：逐步迁入 `internal/conf/features_runtime.go`（见各「读取位置」）。

### 运行时行为开关

| 开关 | 默认值 | 用途 | 读取位置 | 状态 |
|------|--------|------|----------|------|
| `ARANEA_PARALLEL_AUTO` | **true** | LLM 返回 parallel_candidates 自动并行调度 | `internal/agent/intent/parallel.go` | stable |
| `ARANEA_TEAM_GRAPH_RUNTIME` | **true** | 团队 Graph 运行时总闸 | `internal/team/graph_runtime.go` | stable |
| `ARANEA_OBS_PERSIST` | **true** | 编排观测步骤落库（ActivityStepFlusher） | `internal/team/activity_step_flusher.go` | stable |
| `ARANEA_PROMPT_SNAPSHOT` | **true** | BeforeModel 记录 Prompt 组成快照日志 | `internal/agent/prompt_snapshot.go` | debug |
| `ARANEA_L0_SNAPSHOT` | false | L0 快照强制调试（`1/true/always/force`） | `internal/agent/l0_snapshot_persist.go` | debug |
| `ARANEA_INTENT_PASS` | false | 意图分类旁路直通 | `internal/agent/intent/pass.go` | debug |
| `ARANEA_INTENT_PASS_MODEL` / `ARANEA_INTENT_PASS_PROVIDER` | 空 | 直通模式下指定模型/Provider | `internal/agent/intent/pass.go` | debug |
| `ARANEA_TOOL_AUTO_APPROVE` | false | 工具确认自动通过（配合 DEV_MODE） | `internal/agent/tool_confirmation.go` | debug |
| `ARANEA_DEV_MODE` | 空 | 开发模式（影响工具确认策略） | `internal/agent/tool_confirmation.go` | debug |
| `ARANEA_KANBAN_TOOLS` | false | 强制启用看板工具（无 TASK_ID 时） | `internal/tools/kanban/tools.go` | debug |
| `ARANEA_ENT_SQL_DEBUG` | false | Ent SQL 逐行打印到 stdout | `internal/data/data.go` | debug |

### 部署/环境配置

| 开关 | 默认值 | 用途 | 读取位置 | 状态 |
|------|--------|------|----------|------|
| `ARANEA_ENV` | 空 | 环境标识（codeexecutor / spirit_synthesis 分支） | `internal/agent/codeexecutor/factory.go`、`internal/biz/spirit_synthesis.go` | stable |
| `ARANEA_DATA_DIR` | 空 | 数据目录（CriticalJournal 落盘根） | `internal/event/critical_journal.go` | stable |
| `ARANEA_WORKSPACE_ROOT` | 空 | 工作区根目录（工具装配/member_fs） | `internal/agent/tool_assembly.go`、`internal/service/member_fs.go` | stable |
| `ARANEA_WEB_HTTP_PROXY` | 空 | webresearch 工具 HTTP 代理 | `internal/tools/webresearch/config.go` | stable |
| `ARANEA_ALLOWED_PRIVATE_HOSTS` | 空 | 内网主机白名单（URL 安装等） | `internal/service/cli_admin_tools.go` | stable |

### CLI / 测试辅助（不影响服务运行时）

| 开关 | 默认值 | 用途 | 读取位置 | 状态 |
|------|--------|------|----------|------|
| `ARANEA_BASE_URL` / `ARANEA_TOKEN` / `ARANEA_OUTPUT` | 空 | CLI 连接配置覆盖 | `internal/cli/config/config.go` | stable |
| `ARANEA_NO_COLOR` | false | CLI 禁用彩色输出 | `internal/cli/ui/tty.go`、`internal/cli/config/config.go` | stable |
| `ARANEA_TASK_ID` | 空 | 看板工具任务上下文注入 | `internal/tools/kanban/tools.go` | stable |
| `ARANEA_TEST_PG_DSN` | 空 | 集成测试 Postgres DSN | `internal/data/testhelper/pg.go` 等 | stable |

---

## 3. 变更记录

| 日期 | 变更 |
|------|------|
| 2026-07-28 | 初版登记（P1-Y6）：盘点 conf 访问器 14 个 + 直读 env 24 处 |
