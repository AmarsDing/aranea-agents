# 76 编程 Agent 桥接（Coding Agent Bridge）开发计划

> 开发计划 ｜ 2026-08-12 ｜ 状态：M1 主体完成；M2 审批中继已落地（adapter 仍待）
> 需求见 [76-coding-agent-bridge.md](./76-coding-agent-bridge.md) ｜ 设计见 [76-coding-agent-bridge.design.md](./76-coding-agent-bridge.design.md)

---

## 1. 模块定位

精灵助手 ↔ 外部编程 CLI Agent（Claude Code / Codex / CodeBuddy）的统一桥接层：ACP 协议驱动子进程、任务状态机、审批中继（复用工具确认门 + clarify 语音链路）、结果回收播报。Trae 仅作拉起入口。

## 2. 代码锚点（现状，已核实）

| 复用点 | 文件 | 复用方式 |
|--------|------|---------|
| 工具确认门事件 | `internal/agent/tool_confirm_gate.go`、`internal/agent/tool_confirmation.go` | 审批中继出站事件同构构造 |
| 确认回传 | `internal/service/chat_confirm.go`（ConfirmActivity/ConfirmToolGateForCard） | 回传通道语义参照，新增 ConfirmBridgePermission |
| 澄清语音链路 | `internal/service/chat_clarify.go` | 审批 TTS 播报 + 语音作答路由参照 |
| 确认卡片 | `web/src/components/companion/HoloConfirmCard.vue`、`web/src/features/companion/useCompanionConfirms.ts`（已核实存在） | source=external_coding 扩展 |
| 事件总线/WS | `internal/event/bus_v2.go`、`internal/server/ws_v2_wire_convert.go` | 新事件类型注册 |
| 工具注册 | `internal/tools/toolset.go`（Registry/ToolRegistration/ToolSetFactory，已核实） | codingbridge ToolSet 注册 |
| 精灵编排入口 | `internal/service/spirit_team.go` | 零改动（工具注入即可用） |

## 3. 现状评估与差距

| 维度 | 现状 | 差距 |
|------|------|------|
| 外部编程工具集成 | 无 | 全新模块 |
| ACP 协议 | 无 | 需自实现 Go 客户端子集（无官方 Go SDK） |
| 确认门/语音澄清 | 完整可用 | 仅需扩展事件 source 与路由 |
| RemoteAgent（A2A） | URL 模型，语义不符 | 不复用，新建 `biz/agentbridge` 域 |
| 子进程管理 | 无 | 新建 ProcessManager（spawn/监视/进程组 kill） |

## 4. Phase 划分

| Phase | 内容 | 状态 |
|-------|------|------|
| **M1** | ACP Go 客户端 + 数据模型 + 派发/进度/结果回收 + CodeBuddy 冒烟（审批走 agent 默认策略，无中继） | 🟡 M1-1~12 ✅；冒烟/E2E 📋 |
| **M2** | 审批中继（确认卡片 + 语音作答 + 超时）+ claude-code-acp / codex-acp adapter 接入 | 🟡 中继 ✅；adapter 📋 |
| **M3** | Trae 拉起 + 管理界面（AgentBridgePage）+ 排队并发控制 | 📋 |

## 5. M1 任务清单（TDD：每任务先失败测试后实现）

| # | 任务 | 验证 | 状态 |
|---|------|------|------|
| M1-1 | ACP 类型与 NDJSON 帧编解码（`acp/types.go` + `acp/conn.go`） | 单测：帧读写、JSON-RPC 请求/响应/通知路由、挂起请求表、畸形行容错 | ✅ |
| M1-2 | 子进程管理（`acp/process.go`）：spawn/stdout pipe/Wait 监视/进程组 kill | 单测（fake 子进程脚本）+ Windows 进程组终止验证 | ✅ |
| M1-3 | ACP Client（`acp/client.go`）：initialize/session/new/prompt/cancel | 集成测：fake ACP server（Go 实现的 stdin/stdout mock agent）全方法覆盖 | ✅ |
| M1-4 | Ent Schema：coding_agents / coding_projects / coding_tasks + `go generate` + DDL 迁移注册 | Schema 编译 + 迁移幂等性测试（testhelper.SetupTestPG） | ✅ |
| M1-5 | data 层三个 Repo + `entErrToBizErr` 翻译 | data 层测试（独立 PG schema，8/8 全绿） | ✅ |
| M1-6 | biz 任务状态机（`task_state_machine.go`） | 单测：合法/非法转换全枚举 | ✅ |
| M1-7 | biz AgentBridgeUsecase：派发/取消/项目消歧（mock ACPSession 端口） | 单测：消歧三分支、并发上限、错误路径（15/15 全绿） | ✅ |
| M1-8 | service AgentBridgeService：事件聚合限流（5s 窗口）+ 进度事件发射 | 单测：限流窗口、事件负载（6/6 全绿） | ✅ |
| M1-9 | codingbridge 三工具（dispatch/check/cancel）注册进 tools registry | 单测：工具入参校验、返回结构（11/11 全绿） | ✅ |
| M1-10 | proto + service 管理 API（agent/project CRUD、task 查询/取消） | `make api` 编译 + service 层测试 | ✅ |
| M1-11 | Wire 装配 + 启动恢复钩子（RecoverActiveTasks） | `make wire && go build ./cmd/admin` | ✅ |
| M1-12 | 流程日志 step 登记（flow_log.go stepTitleRegistry + 52 文档 §5.1 同步） | 登记检查 | ✅ |
| M1-13 | fake ACP server 端到端：dispatch → 进度 → done 全链路 | 集成测试绿 | 📋 |
| M1-14 | CodeBuddy 真实冒烟（手动）：`codebuddy --acp` 跑真实任务 | 语音派发→完成播报人工验收 | 📋 |
| M1-15 | 工具运行时挂载链收尾（M1-14 前置，M1-12 审查发现）：catalog seed（`builtin_tools_seed.go` + coding_dispatch_task/check_task/cancel_task）+ `effective_config.go` key 映射 + `ToolsetConfig.CodingBridge` flag + `TRPCToolAssemblyDeps.CodingBridgeSvc` + `tool_assembly.go` 传递 + `RuntimeTooling` 注入 | agent 启用 coding_* 后 `BuildToolsets` 产出三工具 | ✅ |

## 6. M1 验收标准

- [ ] AC-1：语音派发 CodeBuddy 任务，60 秒内进入 running，流程日志完整
- [ ] AC-4：完成后 5 秒内 TTS 播报摘要
- [ ] AC-5：未安装工具派发时明确报错，无僵死任务
- [ ] AC-7：服务重启后活跃任务标记 failed，无残留子进程
- [ ] `go test ./internal/agentbridge/... ./internal/biz/agentbridge/... ./internal/data/... ./internal/service/... -count=1` 全绿
- [ ] `make api && make wire && make build && make lint` 通过

## 7. M1 改动文件清单

**新增**：
- `internal/agentbridge/acp/{client,conn,process,types}.go` + 测试
- `internal/biz/agentbridge/{types,repo,task_state_machine,usecase}.go` + 测试
- `internal/data/{coding_agent_repo,coding_project_repo,coding_task_repo}.go` + 测试
- `internal/data/ent/schema/{coding_agent,coding_project,coding_task}.go`
- `internal/service/agentbridge.go` + 测试
- `internal/tools/codingbridge/` 工具集 + 测试
- `api/kratos/agentbridge/v1/agentbridge.proto`

**修改**：
- `internal/data/ent`（generate 产物）
- `internal/data/sql/migrations/` + `ddl_migration_registry.go`（唯一索引/约束）
- `internal/event/flow_log.go`（step 登记）
- `internal/event/bus_v2.go`、`internal/server/ws_v2_wire_convert.go`（事件类型）
- `cmd/admin/wire.go` + `wire_gen.go`（装配）
- `docs/development/52-flow-logger.design.md` §5.1（step 注册表同步）
- `docs/development/65-module-cross-reference-full.md`（新增模块卡片）

## 7.1 M2 审批中继（2026-08-22）

| # | 任务 | 验证 | 状态 |
|---|------|------|------|
| M2-1 | ACP `OnPermission` 不再自动放行；任务 `running → awaiting_approval`；发射 `coding_task_approval` + confirm step（`source=external_coding`） | `agentbridge_approval_test.go` | ✅ |
| M2-2 | `ConfirmActivity` 路由 `external_coding` → `ConfirmBridgePermission`；`allow_always` 仅本任务内存缓存 | 同测：二次 permission 不发卡 | ✅ |
| M2-3 | 审批超时 5 分钟 → 任务 `cancelled` + `agentbridge.approval.timeout` | 同测缩短超时 | ✅ |
| M2-4 | `HoloConfirmCard` 标题 `{agent} · {project}` | `useCompanionConfirms.spec.ts` | ✅ |
| M2-5 | claude-code-acp / codex-acp adapter | 专项评估 | 📋 |

**代码锚点**：`internal/service/agentbridge_approval.go`、`internal/biz/agentbridge/approval.go`、`internal/service/chat_confirm.go`、`cmd/admin/app.go` `BindAgentBridge`。

## 8. 风险与缓解

| 风险 | 缓解 |
|------|------|
| Go 无 ACP SDK，协议理解偏差 | M1-3 fake server 按官方 schema 实现，集成测先行；CodeBuddy 官方文档对照 |
| Windows 子进程组管理差异 | M1-2 专项验证 taskkill /T 行为 |
| adapter（claude/codex）滞后 | M1 只做 CodeBuddy 原生，adapter 评估留到 M2 入口 |
| 并行会话 GOCACHE 幻影 | 验证以独立 GOCACHE 干净缓存复跑为准 |
