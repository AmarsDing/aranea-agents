# 进程日志（Process Log）覆盖审计与关键节点补齐清单

> **类型**：audit | **日期**：2026-07-29
> **关联**：[2026-07-29-audit-flow-log-coverage.md](./2026-07-29-audit-flow-log-coverage.md) · [project_rules.md 日志架构约束](../../.trae/rules/project_rules.md)
> **结论**：进程日志链路（`loggateway.Logger` → Pipeline → FileSink/StdoutSink/EventBusSink → WS `log`）运转正常，全工程 436 处结构化调用分布于 120 个文件，但**分布严重不均**：data/agent/knowledge/memory/skill/team 较密，而 **WS 泵、channel 平台层连接、service 管理面 CRUD、artifact、scenario 等近乎为零**，导致「进程日志」Tab 在日常运行中只有少量模块的输出。

---

## 1. 机制回顾

```
业务代码 lg.Info/Warn/Error(msg, loggateway.StepID/SessionID/Err/Str...)
  → logpipeline.Pipeline（异步分发，4096 缓冲，stepID 前缀限流）
      ├─ FileSink     → logs/*.log（JSON + lumberjack 轮转）
      ├─ StdoutSink   → stdout（级别过滤）
      └─ EventBusSink → MonitorBus → WS /v1/ws `log` → 监控 Logs 页「进程日志」Tab
```

- 注入纪律：构造函数注入 `lg loggateway.Logger` + `lg.With()` 预设字段；禁止 `log/slog`、禁止 `loggateway.Global()`（红线 #16/CS-B1）
- 开关：`server.monitor.process_log_enabled`（默认 true）；trpc 运行时日志经 `RuntimeLogAdapter` 桥接已覆盖
- 级别语义：Debug=开发调试｜Info=正常业务关键节点｜Warn=可恢复异常/降级｜Error=需关注错误

## 2. 现状密度矩阵（2026-07-29 实测）

> 统计口径：`loggateway.*` 结构化字段调用次数 / 涉及文件数。

| 模块 | 密度 | 证据（热点文件） | 评价 |
|------|------|------------------|------|
| internal/data（启动/迁移/种子/Repo） | ★★★ | data.go:27、tx.go:14、seed_system_admin:13、turn_index_migrate:10 | 启动链路覆盖好 |
| internal/agent | ★★★ | model_selector:10、trpc_build:7、tool_assembly:7、cache:5 | 构建链路覆盖好 |
| internal/knowledge | ★★★ | embedder:7、query_rewriter:6、vault 系列 2-3/文件 | 检索/同步覆盖好 |
| internal/memory | ★★★ | link_evolution:10、sleep_time:8、episode_consolidator:7 | 维护任务覆盖好 |
| internal/skill | ★★★ | importer/engine:23、watch/runner:21 | 导入/热重载覆盖好 |
| internal/team | ★★☆ | team_graph_run_coordinator:20、finisher:7、mediator:7 | 良好 |
| internal/modelregistry | ★★★ | store:15、sync:13 | 目录同步覆盖好 |
| internal/cronrunner | ★★☆ | execute:9、runner:4、jobs/* 各 1-6 | 执行有，调度判断稀疏 |
| internal/a2a | ★★☆ | remote_invoke:8、invoker:6、remote_client:5 | 调用链有，治理链在 biz 层 |
| internal/graph | ★★☆ | runtime_adapter:51、event_bridge:12、builder:8 | adapter 密，service 面无 |
| internal/plugin/trpc | ★★☆ | cost_guard_budget:7、hook_retry_worker:6 | 中等 |
| internal/evaluation | ★★☆ | runner:18、callbacks:9 | 中等 |
| internal/provider（LLM） | ★★☆ | trpc_llm:15、retry_transport:2 | HA/预检有，常规调用无 |
| internal/session/compress | ★★☆ | compressor:21、runtime:8 | 中等 |
| internal/service（管理面 CRUD） | ★☆☆ | background_job_worker:11、admin:6、agent:4、a2a:4 | **大量 CRUD 入口无日志** |
| internal/mcp | ★☆☆ | health/runner:2、alert:1 | 健康检查稀疏 |
| internal/tools | ★☆☆ | deptmail:4、worktree_isolator:4、sessionaccess:3 | **Assemble 装配无摘要日志** |
| internal/provider/media | ★☆☆ | persist:3 | 生成调用无 |
| internal/channel（13 平台子包） | ★☆☆ | 仅 wechat/telegram/slack outbound 各 1-2 | **连接生命周期/入站近乎为零** |
| internal/server（WS pump） | ☆☆☆ | 仅 grpc/http/login_ratelimit 有 Logger 字段 | **WS 读写/丢弃无日志**（注册表 `system.ws.*` 无发射） |
| internal/artifact | ☆☆☆ | 无（仅 data/artifactfs/repo:8） | 零 |
| internal/scenario（pack 安装） | ☆☆☆ | 无 | 零 |
| internal/event（总线） | ★★☆ | generic_bus/monitor_bus drop 计数日志 | drop 有，订阅异常无 |

## 3. 关键节点模型（补齐标准）

> 进程日志的目标是**抓住系统是否稳定运行、业务逻辑是否正确**。以下 7 类节点为必打点；高频热路径（stream delta、token 级、list 查询读路径）**禁止**打 Info，避免日志风暴。

| # | 节点类型 | 级别 | 内容要求 |
|---|----------|------|----------|
| K1 | 流程入口/出口 | Info | 关键参数摘要（ID、数量、开关），出口含耗时/结果计数 |
| K2 | 错误路径 | Error/Warn | error 返回前必打，含 `loggateway.Err(err)` + 上下文 ID |
| K3 | 降级/回退 | Warn | 降级原因 + 影响面（如"语义层不可用，降级 BM25"） |
| K4 | 重试 | Warn→Error | 每次重试 Warn（第 N 次/退避），耗尽后 Error |
| K5 | 状态变更 | Info | 状态机转换、claim/release、终态（含 from→to） |
| K6 | 外部调用 | Info/Warn | LLM/HTTP/MCP/子进程：目标、耗时、失败原因 |
| K7 | 后台任务生命周期 | Info/Error | start/stop/tick 异常、panic recover、死信 |

## 4. 分模块缺口与补齐清单

### 4.1 P0（稳定性关键，6 组）

| 模块 | 缺口 | 补齐点（文件 → 节点） |
|------|------|----------------------|
| **internal/server WS 泵** | 连接建立/关闭、读写错误、发送缓冲满丢弃、解析失败全部无日志 | `ws.go` 连接建立/关闭（K7 Info，含 session_id/mode）；`ws_io_pump.go` 读错误/写错误（K2 Error）、`isLogEnabled` 丢弃计数（K3 Warn 节流）；`ws_message_handler.go` 消息解析失败（K2 Warn）。stepID 对齐注册表 `system.ws.*` |
| **internal/channel 平台连接层** | 13 平台接入近乎无日志，连接断开/重连无法追踪 | `lark/ws.go`+`ws_inbound.go`（K7 连接建立/断开/重连 Info，K2 入站解密/解析失败 Warn）；`discord/gateway.go`、`slack/socketmode.go`、`mattermost/gateway.go`、`telegram/polling.go` 同模式；`runtime/supervisor.go`/`manager.go` Reload 与凭据失败（K2，stepID 对齐 `channel.runtime.credentials_fail` ★已注册） |
| **internal/service 管理面 CRUD** | agent/session/skill/team/graph/knowledge 的创建/更新/删除入口与错误无日志 | 各 Service 写方法：入口 Info（K1，含对象 ID/key）+ 错误 Error（K2）。读/列表方法不打 |
| **internal/service graph 控制面** | Run/Resume/HITL 确认/取消无日志（adapter 层有但入口无） | `service/graph.go` StartRun/Resume/ConfirmHITL/Cancel：K1 入口 + K5 状态转换 + K2 错误（stepID 对齐 `system.graph.*` ★已注册） |
| **internal/cronrunner 调度** | tick/claim/skip/死信判断稀疏 | `runner.go` tick 异常（K7 Error）；`execute.go` 分发跳过（会话忙，stepID `system.cron.dispatch_skipped` ★已注册）、重试（K4 Warn，`system.cron.retry` ★）、死信（K2 Error，`system.cron.job_dead` ★）、panic（K7，`system.cron.panic` ★） |
| **internal/event 总线** | 订阅异常、死信缓冲满无日志 | `generic_bus.go` drop 已有；补 `event_bus_async.go` 队列满降级（K3 Warn，stepID `event_bus.queue_full_*` 已有进程日志→确认级别）、dead-letter 缓冲写入（K2） |

### 4.2 P1（业务正确性关键，7 组）

| 模块 | 缺口 | 补齐点 |
|------|------|--------|
| **internal/tools 装配** | Assemble 无摘要，工具缺失时无法定位 | `toolset.go` Assemble 出口 Info（K1：装配 toolset 数/工具 key 列表/耗时）；单工具装配失败 Warn（K3 跳过原因） |
| **internal/mcp 健康检查** | 探活失败/状态翻转无感知 | `mcp/health/runner.go` 检查结果翻转 Info（K5）、连续失败 Warn（K2，stepID 对齐 `system.mcp.*` ★已注册）；probe 超时 Warn |
| **internal/provider LLM 调用** | 常规调用失败只在 Turn 流程日志，进程侧无 | `trpc_llm.go` 模型构建完成 Info（K1 provider/model/HA 候选数）；重试/熔断触发 Warn（K4） |
| **internal/biz 关键写路径** | session 压缩 CAS 失败、agent 创建校验失败等无日志 | `biz/session/usecase.go` 压缩版本冲突 Warn（K5）；`biz/agent_*.go` 创建/更新冲突 Warn（K2） |
| **internal/a2a 治理链** | 信任/策略/配额拒绝在 biz 层无日志 | `biz/a2a/federation_trust.go`/`federation_policy.go`/`federation_quota.go` 拒绝分支 Warn（K5，含 org_id/规则）；审计落库失败 Error（K2） |
| **internal/artifact** | 存取/版本冲突无日志 | `data/artifactfs/repo.go` 已有 8 处→补 `internal/artifact/trpc` 写入失败 Error（K2） |
| **internal/provider/media** | 生成调用无日志 | `provider/media` 生成入口 Info（K6：provider/模型/尺寸）、失败 Error、产物落库 Info |

### 4.3 P2（低频/补强，5 组）

| 模块 | 缺口 | 补齐点 |
|------|------|--------|
| internal/scenario | pack 安装无日志 | 安装入口/完成 Info（K1）、失败 Error（K2） |
| internal/evaluation | judge 调用细节 | `runner.go` 每个 case 完成 Info（K1 得分）、judge 失败 Warn（K6） |
| internal/monitor | 告警状态翻转 | `biz/monitor/monitor.go` 告警触发 Info（K5 rule/severity）、通知失败已有 ★ |
| internal/telemetry | init 结果已有 3 处 ✓ | 无需补 |
| pkg/auth / server 中间件 | 认证失败 | 登录失败 Warn（K2 节流，防爆破刷屏）；认证绕过启用 Warn（stepID `system.auth.*` ★已注册） |

### 4.4 明确不补（防止日志风暴）

- chat/team Turn 热路径（已由流程日志覆盖，进程侧由 FlowTracker 内部 `ft.lg.Info` 顺带输出）
- stream delta/token 级事件、WS 逐帧转发
- 纯 list/get 读路径（除非 error）
- data 层常规 CRUD（ent 错误经 `entErrToBizErr` 翻译，由上层 K2 记录）

## 5. 实施约束

1. 全部走构造函数注入 `lg loggateway.Logger`；struct 已有 lg 的直接用，没有的补构造参数 + Wire（`make wire && go build ./cmd/admin`）。
2. 字段用结构化构造器（`loggateway.StepID/SessionID/RunID/Err/Str/Int`），禁止拼接到 msg；stepID 优先复用 `stepTitleRegistry` 已有条目（★ 标注处）。
3. 高频丢弃/失败类日志（WS 丢弃、认证失败）必须节流或归入既有 Pipeline 限流前缀，防止刷屏。
4. 验证：`go build ./... && go test ./internal/... -count=1`；运行时触发对应场景，在 Logs 页「进程日志」Tab + `logs/` 文件双侧确认。
