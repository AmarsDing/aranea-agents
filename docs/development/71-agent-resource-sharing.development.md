# M71: Agent 资源共享 — 开发计划

> 需求：[71-agent-resource-sharing.md](./71-agent-resource-sharing.md)
> 设计：[71-agent-resource-sharing.design.md](./71-agent-resource-sharing.design.md)

## 一、模块定位

受控资源访问层：memberfs（主管读员工目录）+ deptmail（主管信箱 + Turn 唤醒）+ sessionaccess（精灵只读检索会话）。统一 权限校验 → 范围解析 → 审计落库。

## 二、实施要点（探查结论，2026-07-22）

设计落地时的关键现实约束（已逐条核实）：

| # | 设计假设 | 代码现实 | 落地决策 |
|---|---------|---------|---------|
| 1 | messages_fts FTS5 检索 | messages 子系统已于迁移 20260902 整体 drop；现有 `MessageSearchReader.SearchMessages` 仅支持单 session（ActivityMessageReader → steps_v2 内存过滤） | 新增 data 层 `GlobalMessageSearcher`：steps_v2 `content LIKE` 全局检索（kind IN task/reply），按 started_at 倒序 |
| 2 | `internal/biz/resourceaccess/` 独立包 | biz 已 import biz/session；resourceaccess 重度依赖 biz 类型（Agent/Team/OrganizationReader/SessionReader 别名） | 三个 Usecase 放 **package biz**（与 dept_lead.go 同风格），避免包循环 |
| 3 | ToolsetConfig 新增旗标 | 现有自定义工具走 `CustomToolFunc` 闭包装配（cliAdminTools/spiritCustomTools/deliverableReaderTools 模式） | 沿用 CustomToolFunc：新增 `memberFSDeptMailTools(ag)` + `sessionAccessTools(ag)`，**不改 ToolsetConfig** |
| 4 | 索引经 DDL Migration Registry | Ent Schema.Create 会创建 Indexes() 声明的复合索引（见 20261001 注释） | 两表索引直接声明在 Ent Schema Indexes()，**不新增 DDL 迁移** |
| 5 | NativeTurnGateway 唤醒 | `NativeTurnGateway` 已 Deprecated（= ChannelTurnGateway 别名）；`TurnExecutorGateway.ExecuteTurn(ctx, TurnInput)` 为现行接口；EntryConfig 零值即允许 pending queue 排队 | service 层 `MailboxWaker` 实现注入 `biz.TurnExecutorGateway` + `biz.SessionReader/Writer` + `biz.AgentReader` |
| 6 | 主管身份反查 | DeptLeadManager 创建时 `PositionID = deptNode.ID`、`AgentVariant = "dept_lead"`、AgentKey 前缀 `__dept_lead_`（biz.DeptLeadAgentKeyPrefix） | usecase 内校验 variant/prefix + PositionID→department 节点；员工部门经 position→parent dept（复用 agentDepartment 逻辑） |
| 7 | 借调关系 | `Team.CrossDeptMemberIDs`（JSON string，Agent ID 列表）+ `DeptTeamLister.ListTeamsByDepartmentID` | team_owner 判定：callerDept 的非 archived Team 的 CrossDeptMemberIDs 含 target agent ID |
| 8 | 工作目录布局 | `{base}/workspace/{workspaceID}/{agentKey}`（tool_assembly.go resolveAgentFilesystemDir）；base = SystemSetting.RootDirectory 或 ARANEA_WORKSPACE_ROOT/WORKSPACE_ROOT env | service 层 `MemberDirResolver` 实现复刻同一解析链 |

## 三、改动文件清单

### 新建（14）

| 文件 | 职责 |
|------|------|
| `internal/data/ent/schema/dept_lead_message.go` | 信箱表 Schema（含 Indexes） |
| `internal/data/ent/schema/resource_access_audit.go` | 审计表 Schema（含 Indexes） |
| `internal/biz/resource_access.go` | ResourceAccessUsecase + 端口（MemberFileReader/MemberDirResolver/AccessAuditor）+ 权限策略 |
| `internal/biz/dept_mailbox.go` | DeptMailboxUsecase + MailboxRepo/MailboxWaker 端口 + 5min 防抖 |
| `internal/biz/session_search.go` | SessionSearchUsecase + GlobalMessageSearcher 端口 + 令牌桶限流（20/min） |
| `internal/data/dept_lead_message_repo.go` | 信箱 Repo（Ent） |
| `internal/data/resource_access_audit_repo.go` | 审计 Repo（Ent，只增） |
| `internal/data/global_message_search_repo.go` | steps_v2 全局 LIKE 检索（Ent predicate） |
| `internal/service/member_fs.go` | MemberFileReader + MemberDirResolver 实现（secureJoin + 二进制嗅探 + 截断） |
| `internal/service/mailbox_waker.go` | MailboxWaker 实现（找/建主管 session → ExecuteTurn） |
| `internal/tools/memberfs/memberfs.go` | 3 工具薄壳（list/read/search） |
| `internal/tools/deptmail/deptmail.go` | 4 工具薄壳（send/list_inbox/read/reply） |
| `internal/tools/sessionaccess/sessionaccess.go` | 3 工具薄壳（search_messages/list_agent_sessions/read_session_history） |
| `internal/biz/resource_access_test.go` | 权限矩阵 + 防抖 + 限流 + fail-closed 单测 |
| `internal/service/m71_service_test.go` | secureJoin 路径穿越/符号链接/二进制/截断 + MailboxWaker 单测 |

### 修改（6）

| 文件 | 改动 |
|------|------|
| `internal/data/data.go` | ProviderSet 注册 3 个新 Repo |
| `internal/service/chat_orchestrator.go` | RuntimeTooling +3 字段；CustomToolFunc +2 装配分支 |
| `cmd/admin/wire.go` | provideRuntimeTooling +3 参数；3 个 biz usecase provider + 2 个 service 实现 provider + wire.Bind |
| `cmd/admin/wire_gen.go` | `make wire` 重新生成 |
| `internal/service/service.go` | 补 `synthesisResultService` 绑定（P-REPORT 遗留，修复 `make wire` 不可复现） |
| `internal/data/team_repo.go` | 修复 `ListActiveRunTeamIDs` Ent API 误用（`Unique(true).Select.Strings` 不存在 → `GroupBy.Scan`） |

### Ent 生成物

`go generate ./internal/data/ent`（--feature sql/execquery,sql/upsert），生成物整体提交（DB-R2）。

## 四、任务清单

- [x] T1 Ent Schema ×2 + `go generate` 通过
- [x] T2 data 层 Repo ×3（信箱/审计/全局检索）+ ProviderSet 注册
- [x] T3 biz 层 resource_access.go（权限策略 + 审计编排，fail-closed）
- [x] T4 biz 层 dept_mailbox.go（收发读回 + 唤醒防抖）
- [x] T5 biz 层 session_search.go（spirit 校验 + 限流）
- [x] T6 service 层 member_fs.go（路径安全 + 文件读取）
- [x] T7 service 层 mailbox_waker.go（session 定位 + Turn 提交；SetTurnGateway setter 注入破 Wire 环）
- [x] T8 tools ×3 包（薄壳，装配期身份闭包）
- [x] T9 chat_orchestrator 装配 + wire 绑定
- [x] T10 单测（biz 策略矩阵/防抖/限流/fail-closed + service 路径安全）— 2026-07-23 全绿（biz 21 用例 + service 23 用例）
- [x] T11 全量验证 `go build ./... && go test ./internal/...` + 文档状态同步 — 2026-07-23 build exit 0；service 7.7s / biz 18.6s / agent 45.7s / data 21.1s 全绿；`make wire` 恢复可复现生成

## 五、验收标准

1. `go build ./...` exit 0；`go test ./internal/biz/... ./internal/service/... ./internal/data/... ./internal/tools/...` 全绿
2. 权限矩阵单测覆盖：dept_lead/spirit/员工 × org_home/team_owner/无关系 × allow/deny
3. 路径安全单测覆盖：`..` 逃逸、绝对路径、符号链接逃逸、二进制拒绝、200KB 截断
4. 防抖单测：同发送方 5min 窗口合并、跨发送方独立
5. fail-closed 单测：审计写失败 → 访问被拒
6. 文档同步：本文件任务状态 ✅；`65-module-cross-reference-full.md` 新增 M71 卡片
