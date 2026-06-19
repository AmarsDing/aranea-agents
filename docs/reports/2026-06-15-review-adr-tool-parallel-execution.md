# ADR-02: 工具并行执行默认开启与项目层装饰器保护

## 状态：已接受

## 背景

性能瓶颈评审报告（`2026-06-15-review-agent-tool-execution-performance.md`）的 P2-B1 方案建议将 `AgentRuntimeSettings.ToolsParallelEnabled` 默认值从 `false` 改为 `true`，以利用 trpc-agent-go 框架的 `WithEnableParallelTools` 能力提升工具执行吞吐。

核心担忧：**开启并行执行后，Exclusive（状态修改型）工具可能被框架同时调度，导致资源竞争或状态不一致**。框架本身不区分工具的安全等级，仅提供并行开关。

## 决策

### D1: 默认开启并行执行

将 `internal/biz/agent_defaults.go` 中 `ToolsParallelEnabled` 默认值改为 `true`。框架在 `internal/agent/trpc_build.go` 中已通过 `WithEnableParallelTools(true)` 读取此设置。

### D2: 项目层 ToolDecorator 提供执行保护

在 `internal/tools/decorator.go` 中实现 `ToolDecorator`，包装所有 `CallableTool`，提供三层保护：

1. **P0-G3 执行超时**：每次工具调用默认 60s 超时（`DefaultToolTimeout`），防止慢工具阻塞整个并行批次
2. **P0-D 结果预算**：工具结果超过 10KB（`DefaultResultBudget.MaxBytes`）时自动截断，防止单个工具结果撑爆 LLM 上下文窗口
3. **P2-E 确定性缓存**：`ConcurrentSafe` 工具（如 `file`、`read_document`）的相同参数调用结果被缓存，减少重复计算

### D3: 工具安全分类（P1-C）

`internal/tools/safety.go` 的 `ClassifyTool(name)` 基于 `Registry` 的 `SupportsConcurrency` 字段将工具分类为 `SafetyConcurrentSafe` 或 `SafetyExclusive`。未知工具默认 `Exclusive`（安全默认）。

### D4: 装饰器不触碰框架内部

`ToolDecorator` 在项目层包装 `CallableTool` 接口，不修改 `pkg/trpc-agent-go/` 内部代码。框架升级时装饰器自动兼容（只要 `CallableTool` 接口不变）。

## 后果

### 正面影响

- 工具执行吞吐提升：多个独立工具可同时执行，减少端到端延迟
- 执行安全兜底：即使 Exclusive 工具被并行调度，60s 超时和 10KB 预算仍生效
- 缓存减少重复计算：ConcurrentSafe 工具的相同调用直接返回缓存结果
- 框架解耦：装饰器在项目层实现，框架升级零风险

### 负面影响

- Exclusive 工具并行风险仍存在：装饰器不阻止并行调度，仅提供超时/预算保护。如果 Exclusive 工具（如 `hostexec`）被同时调用，仍可能产生资源竞争。**缓解**：`Registry.SupportsConcurrency=false` 的工具在分类时标记为 Exclusive，未来可在装饰器层加互斥锁（当前未实现，因框架的并行调度策略不暴露给项目层控制）
- DeferredManager 工具不受保护：`ApplyDecorators` 仅装饰构建时存在的工具，延迟加载的工具不经过装饰器。**缓解**：延迟工具通常是 MCP/agent 工具，框架 runner 层已有自己的治理
- 缓存内存占用：每个 ConcurrentSafe 工具的缓存无上限。**缓解**：当前工具集规模有限，未来可加 LRU 淘汰

## 替代方案

### A1: 保持 ToolsParallelEnabled=false（不实施 P2-B1）

- **优点**：零风险，Exclusive 工具串行执行
- **缺点**：性能无提升，独立工具（如多个 file 读取）仍串行
- **未选原因**：性能评审报告明确要求提升工具执行并行度

### A2: 修改框架内部添加工具安全分级（被否决）

- **方案**：在 `pkg/trpc-agent-go/internal/flow/llmflow/` 中添加工具安全分级逻辑，框架只并行调度 ConcurrentSafe 工具
- **优点**：从源头解决 Exclusive 工具并行风险
- **缺点**：**修改第三方框架内部代码，框架更新时会被覆盖**
- **未选原因**：项目铁律禁止修改 `pkg/trpc-agent-go/` 内部代码

### A3: 在装饰器层为 Exclusive 工具加全局互斥锁

- **方案**：`ToolDecorator` 对 Exclusive 工具加进程级互斥锁，确保串行执行
- **优点**：完全消除 Exclusive 工具并行风险
- **缺点**：过度工程化；当前 Exclusive 工具（hostexec 等）已有自己的并发控制；全局锁可能成为瓶颈
- **未选原因**：YAGNI——当前无证据表明 Exclusive 工具并行导致了实际问题，可未来按需添加
