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

1. **P0-G3 执行超时**：普通工具不再由装饰器默认加 60s。`ToolDecoratorConfig.Timeout=0` 表示尊重外层截止；产品层 `toolExecutionTimeoutHooks` 按 `ToolsExecutionTimeoutSec`（0→10min）注入。`plan_and_execute` 仍由装饰器覆盖为 3min，避免子阶段预算之和撑破外层。
2. **P0-D 结果预算**：工具结果超过 10KB（`DefaultResultBudget.MaxBytes`）时自动截断，防止单个工具结果撑爆 LLM 上下文窗口
3. **P2-E 确定性缓存**：仅对**非工作区** ConcurrentSafe 工具（`web_fetch` / search 等）缓存相同参数结果，默认 TTL 60s。`read_file` 等 file 族不缓存。
4. **P1-C2 Exclusive 互斥**：`ToolDecorator.Call` 对 Exclusive 工具加进程级 family 锁。`exec_command`/`write_stdin`/`kill_session` 共享 `hostexec`；文件写工具按目标路径分锁（`file_write:<path>`，缺路径退回 `file_write`）；只读文件工具不锁。

### D3: 工具安全分类（P1-C）

`internal/tools/safety.go` 的 `ClassifyTool(name)` 基于 `Registry` 的 `SupportsConcurrency` 字段将工具分类为 `SafetyConcurrentSafe` 或 `SafetyExclusive`。未知工具默认 `Exclusive`（安全默认）。

### D4: 装饰器不触碰框架内部

`ToolDecorator` 在项目层包装 `CallableTool` 接口，不修改 `pkg/trpc-agent-go/` 内部代码。框架升级时装饰器自动兼容（只要 `CallableTool` 接口不变）。

## 后果

### 正面影响

- 工具执行吞吐提升：多个独立工具可同时执行，减少端到端延迟
- 执行安全兜底：Exclusive 工具被并行调度时由 family 锁串行；普通工具超时由 Agent `ToolsExecutionTimeoutSec` 生效，不再被装饰器 60s 覆盖
- 缓存减少重复计算：网络 ConcurrentSafe 工具的相同调用直接返回缓存结果；file 族不走装饰器缓存（会话内文件缓存由 file 工具自己按 mtime 管理）
- 框架解耦：装饰器在项目层实现，框架升级零风险

### 负面影响

- Exclusive 工具并行风险已由装饰器 family 锁缓解：hostexec 会话工具进程内串行；文件写按目标路径互斥（不同文件可并行）；`list_file`/`search_*` 对目录子树共享覆盖锁，与子路径写互斥（无 path 时用 glob/`file_pattern` 收窄，避免整仓互斥）。未知 Exclusive 按名称串行。工作区是 git 仓时，LLM 文件写走 worktree 提交合并（内层 filenorm）；否则写活树。`ParallelToolExecutor` 在 `ARANEA_WORKSPACE_ROOT` 为 git 仓时挂 `WorktreeIsolator`。
- DeferredManager 工具不受装饰器互斥保护：`ApplyDecorators` 仅装饰构建时存在的工具。**缓解**：延迟工具的超时仍走回调链；互斥若需要可在 materialize 时套装饰器
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

- **方案**：`ToolDecorator` 对 Exclusive 工具加进程级 family 互斥锁，确保同族串行执行
- **优点**：完全消除 hostexec / 文件写在框架并行下的资源竞争
- **缺点**：同 family 吞吐量下降；锁非可重入
- **状态**：已实施（2026-08-17）。锁按 family 而非全局一把：`hostexec`、文件写/读/`list_file`/`search_*` 走分层路径表（单 mutex + cond，无父子锁序死锁）；其余 Exclusive 按 Registry 名。`StreamableCall` 持锁至流结束。
