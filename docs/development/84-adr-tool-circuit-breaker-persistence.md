# ADR-84：工具熔断器持久化与共享语义

> 状态：Accepted ｜ 日期：2026-10-19 ｜ 来源：`docs/reports/2026-10-18-audit-tools-endtoend.md` §4-A1 ｜ 标准：AS-ADR-01（跨模块决策记录）

## 0. 背景

系统内现有两套熔断器，共用同一个基础实现 `internal/biz/tool.CircuitBreakerRegistry`，但语义不对称：

| 维度 | Node 熔断（图节点） | Tool 熔断（Agent 工具调用） |
|------|--------------------|-----------------------------|
| 入口 | `ProvideNodeCircuitBreakerRegistry`（[node_circuit_breaker.go:52](../internal/biz/node_circuit_breaker.go)） | `buildCircuitBreakerRegistry`（[tool_circuit_breaker.go:18](../internal/agent/tool_circuit_breaker.go)） |
| 生命周期 | wire 单例（进程级） | **每次 Agent 构建时新建**（[callback_chain.go:262](../internal/agent/callback_chain.go#L262)） |
| 持久化 | `WithStateRepo` + 启动 `RestoreStates` | **无**（不传 StateRepo） |
| Key 命名 | `graph_node:<scope>:node:<nodeID>` 全限定 | 裸工具名（如 `httpfetch`） |
| 隔离边界 | 全局（scope 内） | 单个 Agent 实例 |

由此产生两类运维事故面：

1. **静默复位**：Agent 设置变更触发构建缓存失效 → 熔断注册表整体重建 → 所有 Open 状态静默回到 Closed，对外部故障服务的锤击立即恢复；进程重启同理。熔断器"记住故障"的核心价值在最关键的窗口期失效。
2. **跨 Agent 不认账**：同一工作空间内 20 个 Agent 各自统计同一外部服务（如 SMTP、MCP server）的失败，任何单个 Agent 都很难到阈值；保护半径被人为切碎。

**现存基础设施**（本 ADR 复用，不新增）：

- `CircuitBreakerStateRepo`（[circuit_breaker_config.go:24](../internal/biz/tool/circuit_breaker_config.go)，Stability:evolving）：`SaveState`/`LoadState`/`LoadAllStates`；stateRepo 调用 3s 硬超时（`stateIOTimeout`），失败 Warn 不阻塞热路径。
- data 层实现已 wire 注册：[circuit_breaker_state_repo.go](../internal/data/circuit_breaker_state_repo.go)（`data.go:173`），`circuit_breaker_states` 表按 `key` upsert。
- 注册表内建恢复钩子：`Get()` 创建 breaker 时在写锁内单体 `LoadState` 恢复；`onStateChange` 自动 `persistState`。即"接上 repo 即得持久化"。

## 1. 决策

**工具熔断改为「进程级单例注册表 + 全限定 breaker 名 + StateRepo 持久化」，与 Node 熔断对齐。**

1. **单例**：新增 `ProvideToolCircuitBreakerRegistry(repo, lg)`（wire 进程级单例），构造时 `WithStateRepo`（repo 为 nil 则降级为易失 + Warn，保持 TPM-P2-02 降级传统），启动时 `RestoreStates` 一次。
2. **命名空间**（`circuit_breaker_states.key`）：
   - 默认（无 per-agent override）：`tool:<workspaceID>:<toolName>` —— 工作空间内跨 Agent 共享、跨工作空间天然隔离。
   - Agent 配置了 `ToolsCircuitBreakerOverridesJSON`：`tool_agent:<agentID>:<toolName>` —— 保留既有 per-agent 隔离语义（override 是显式定制，共享会破坏其配置意图）。
   - override 应用方式不变：`SetOverride(name, cfg)` 本就按 breaker 名索引，全限定名不产生跨 Agent 串扰。
3. **持久化过滤**：仅 `tool:` 前缀的 breaker 持久化 + 恢复；`tool_agent:` 前缀保持易失（Agent 增删会产生孤儿行，且 per-agent 状态本就按易失设计）。注册表新增选项 `WithPersistFilter(func(name string) bool)`，`persistState`/`RestoreStates` 统一走该过滤器。
4. **接入点**：`callback_chain.go` 的 BeforeTool/AfterTool 熔断 hook 改为构造时注入共享注册表 + 按 `breakerNameFor(ag, toolName)` 组合 key 调用 `Get(name, category)`；`buildCircuitBreakerSystemPrompt`（Open 熔断注入系统提示）同样按工作空间前缀过滤 OpenBreakers。
5. **观测**：状态变更沿用 `onStateChange` 结构化日志（`tool.circuit_breaker.state_change`），breaker 名全限定后工作空间/Agent 归属在日志中直接可读。按 AS-EVT-01 分级：熔断状态持久化属 Important（同步边界 + 失败 Warn 不阻断——与现有 persistState 语义一致）。

## 2. 后果

**正面**
- Open 状态跨"Agent 重建 / 设置变更 / 进程重启"存活——熔断器的核心承诺成立。
- 同工作空间全部 Agent 对同一故障外部服务共享失败计数，阈值可达性恢复设计预期。
- 与 Node 熔断同一套心智模型、同一张表、同一条恢复路径；AS-STA 长期子问题（双语义不对称）关闭。

**代价与对策**
- **一个 Agent 的坏习惯会熔断全空间同名校验**：这是共享的既定语义（保护对象是被调用方，不是调用方）。感知面由 `tool.circuit_breaker.state_change` 日志 + 系统提示注入覆盖。
- **kHz 级热路径新增成本**：零。StateRepo 只在状态变迁与首次 `Get` 创建时触达，均已被既有 `stateIOTimeout` 约束。
- **`circuit_breaker_states` 行增长**：`tool:<ws>:<name>` 受 工作空间数 × 工具数 约束（有界）；`tool_agent:` 不落表。无需清理作业。
- **配置冲突**：per-agent overrides JSON 语义保留（见决策 2），不移除、不改 settings 契约。

## 3. 替代方案（已评审并否决）

| 方案 | 否决理由 |
|------|---------|
| A. 维持 per-agent 易失 + 文档化 | 只解决"不对称未文档化"，不解决"静默复位"与"计数切碎"两个实质事故面；成本最低但收益也最低。 |
| B. 全局共享（无工作空间维度）`tool:<toolName>` | 多工作空间部署下，A 空间配错的私有 MCP/SMTP 会熔断 B 空间的同名工具——跨租户耦合不可接受。 |
| C. 每 Agent 持久化（`tool_agent:` 也落表） | 孤儿行随 Agent 生命周期无限增长，且 per-agent 状态本就按易失设计；需要额外保留期/清理作业，复杂度不成比例。 |
| D. 复用 Node 注册表实例 | Node 注册表是 `NodeCircuitBreakerRegistry`（scope/nodeID 二维 + 策略对象），接口不匹配；强行复用会污染其 key 空间。 |

## 4. 实施锚点

| 模块 | 改动 |
|------|------|
| `internal/biz/tool/circuit_breaker_config.go` | 新增 `WithPersistFilter`；`persistState`/`RestoreStates` 走过滤器 |
| `internal/biz/tool`（或 `internal/biz`） | 新增 `ProvideToolCircuitBreakerRegistry(repo, lg)` |
| `internal/agent/tool_circuit_breaker.go` | `buildCircuitBreakerRegistry` 改为注入共享注册表；hook/`SystemPrompt` 按 `breakerNameFor` 组合全限定名 |
| `internal/agent/callback_chain.go` | 熔断 hook 从 per-build 改为依赖注入 |
| wire（`cmd/admin`） | 注册 `ProvideToolCircuitBreakerRegistry`，复用 `NewCircuitBreakerStateRepo` |
