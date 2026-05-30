# RalphLoop 自我反思闭环

## 一、需求文档

### 1.1 背景

RalphLoop 是 trpc-agent-go 框架提供的"外循环"机制。不同于 LLM 自身决定何时停止，RalphLoop 在 Runner 层面持续迭代，直到可验证的完成条件被满足（或达到最大迭代次数）。当前项目前端已有 RalphLoop 配置入口（`AgentRalphLoopSection.vue`），后端 `internal/agent/` 尚未集成框架的 `WithRalphLoop` 能力，导致前端配置无法生效。

### 1.2 目标

- 将框架 `runner.WithRalphLoop` 集成到项目的 Agent 构建和 Runner 装配流程
- 前端 RalphLoop 配置项能正确传递到后端并生效
- 支持三种停止条件：CompletionPromise、VerifyCommand、自定义 Verifier
- 迭代过程中的反馈消息正确注入 Session

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | Runner 装配时读取 Agent 的 RalphLoop 配置 | P0 | `internal/service` 中 Runner 构建时应用 `WithRalphLoop` |
| F2 | 支持 CompletionPromise 停止条件 | P0 | Agent 输出包含 `<promise>xxx</promise>` 时停止 |
| F3 | 支持 VerifyCommand 停止条件 | P1 | 每轮迭代后执行验证命令，退出码 0 则停止 |
| F4 | 支持自定义 Verifier | P1 | 实现 `runner.Verifier` 接口的可插拔验证器 |
| F5 | 前端配置项与后端字段对齐 | P0 | max_iterations / completion_promise / verify_command 等 |
| F6 | 迭代反馈事件正确转发到 WebSocket | P1 | RalphLoop 注入的 user feedback 消息在前端可见 |
| F7 | 达到最大迭代次数时发出错误事件 | P0 | 前端展示"达到最大迭代次数"提示 |

### 1.4 非功能需求

- RalphLoop 迭代不得阻塞其他并发 Session
- 单次迭代超时由 Agent 的 ctx 控制，RalphLoop 本身不额外增加超时
- VerifyCommand 执行必须在 `pkg/safego.Go` 中运行
- 日志走 `internal/event` 的 `FlowLog`

### 1.5 验收标准

- 前端开启 RalphLoop 配置后，Agent 在不满足条件时自动重试
- CompletionPromise 检测正确触发停止
- VerifyCommand 执行结果正确影响循环
- 最大迭代次数到达后正确终止并通知前端
- 不开启 RalphLoop 时行为与现有完全一致

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：`pkg/trpc-agent-go/runner/ralph_loop.go`

**核心类型和函数**：

```go
// RalphLoopConfig 控制 Runner 级别的 RalphLoop 模式
type RalphLoopConfig struct {
    MaxIterations    int
    CompletionPromise string
    PromiseTagOpen   string
    PromiseTagClose  string
    VerifyCommand    string
    VerifyWorkDir    string
    VerifyTimeout    time.Duration
    VerifyEnv        map[string]string
    VerifyRunner     RalphLoopCommandRunner
    Verifiers        []Verifier
}

// Verifier 检查任务是否完成
type Verifier interface {
    Verify(ctx context.Context, invocation *agent.Invocation, lastEvent *event.Event) (VerifyResult, error)
}

// VerifyResult 描述验证结果
type VerifyResult struct {
    Passed   bool
    Feedback string
}

// RalphLoopCommandRunner 执行 VerifyCommand
type RalphLoopCommandRunner interface {
    Run(ctx context.Context, spec RalphLoopCommandSpec) (RalphLoopCommandResult, error)
}

// WithRalphLoop 启用 RalphLoop 模式的 Runner Option
func WithRalphLoop(cfg RalphLoopConfig) Option

// 内部实现：ralphLoopAgent 包装原始 Agent
type ralphLoopAgent struct {
    inner agent.Agent
    cfg   RalphLoopConfig
}
```

**关键行为**：
- `ralphLoopAgent` 实现了 `agent.Agent` 接口的 5 个方法（Run/Tools/Info/SubAgents/FindSubAgent）
- `Run()` 内部循环调用 `agent.RunWithPlugins`，每轮验证后通过 `appender.Invoke` 注入反馈
- 停止条件：CompletionPromise 检测 AND VerifyCommand 成功 AND 所有 Verifiers 通过
- 至少需要一种停止条件，否则 `validateRalphLoopConfig` 返回错误
- 默认 `MaxIterations = 10`，默认 Promise 标签 `<promise>...</promise>`

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/agent/trpc_build.go` | `BuildTRPCLLMAgent` 构建 LLMAgent，无 RalphLoop 集成 |
| `internal/agent/trpc_build_router.go` | `BuildTRPCAgent` 路由到 LLMAgent 或 A2A Agent |
| `internal/service/` | Runner 装配入口，当前无 `WithRalphLoop` Option |
| `internal/biz/agent.go` | `biz.Agent` struct 有 `Settings` 字段，`AgentRuntimeSettings` 中无 RalphLoop 字段 |
| 前端 `AgentRalphLoopSection.vue` | 已有 UI 配置入口，但后端未对接 |

### 2.3 架构设计

**模块在四层架构中的位置**：

```
api/**/*.proto          ← 新增 ralph_loop 配置字段
        ↓
internal/service        ← Runner 装配时应用 WithRalphLoop
        ↓
internal/biz            ← Agent 模型扩展 RalphLoop 配置
        ↓
internal/data           ← Ent Schema 扩展（如需持久化）
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/biz/agent.go` | 修改 | `AgentRuntimeSettings` 新增 `RalphLoop` 字段 |
| `internal/biz/ralph_loop.go` | 新增 | `RalphLoopConfig` biz 模型 + 转换函数 |
| `internal/agent/ralph_loop.go` | 新增 | `BuildRalphLoopOption` 桥接函数 |
| `internal/service/chat.go` | 修改 | Runner 装配时调用 `WithRalphLoop` |
| `api/admin/v1/agent.proto` | 修改 | 新增 RalphLoop 配置字段 |
| `internal/data/ent/schema/` | 修改 | Agent 表扩展 ralph_loop JSON 字段 |

**接口设计**：

```go
// internal/biz/ralph_loop.go

type RalphLoopConfig struct {
    Enabled            bool
    MaxIterations      int
    CompletionPromise  string
    PromiseTagOpen     string
    PromiseTagClose    string
    VerifyCommand      string
    VerifyWorkDir      string
    VerifyTimeoutSec   int
    VerifyEnv          map[string]string
}

func (c RalphLoopConfig) ToRunnerConfig() runner.RalphLoopConfig

// internal/agent/ralph_loop.go

func BuildRalphLoopOption(cfg biz.RalphLoopConfig) runner.Option
```

**数据流图**：

```
前端 AgentRalphLoopSection.vue
  → API UpdateAgent (ralph_loop JSON)
    → biz.AgentRuntimeSettings.RalphLoop
      → service/chat.go Runner 装配
        → agent.BuildRalphLoopOption(cfg)
          → runner.WithRalphLoop(runnerConfig)
            → ralphLoopAgent.Run() 循环迭代
              → 每轮验证 → 反馈注入 Session → 事件转发 WebSocket
```

### 2.4 与框架的集成方式

1. **桥接层**：`internal/agent/ralph_loop.go` 负责将 `biz.RalphLoopConfig` 转换为 `runner.RalphLoopConfig`
2. **装配点**：`internal/service/chat.go` 中 Runner 构建时，检查 Agent 的 `Settings.RalphLoop.Enabled`，若为 true 则追加 `runner.WithRalphLoop(cfg)`
3. **Verifier 扩展**：项目可自定义 `runner.Verifier` 实现（如 LLM 质量评估 Verifier），通过 `RalphLoopConfig.Verifiers` 注入
4. **VerifyCommand 安全**：框架默认使用 `hostRalphLoopRunner`（直接 `exec.CommandContext`），项目可注入自定义 `RalphLoopCommandRunner` 实现沙箱执行

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| 无停止条件配置 | `validateRalphLoopConfig` 返回 `errRalphLoopMissingStopCondition`，Runner 构建失败 |
| VerifyCommand 执行失败 | `commandSatisfied` 返回 `(false, report)`，循环继续 |
| VerifyCommand 超时 | `RalphLoopCommandResult.TimedOut = true`，视为失败 |
| 达到最大迭代次数 | `emitStopError` 发出 `ErrorTypeStopAgentError` 事件 |
| Session appender 未挂载 | `appendFeedback` 返回错误，循环终止 |
| ctx 取消 | `agent.CheckContextCancelled` 检测后正常退出 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| RL-01 | `internal/biz/ralph_loop.go`：定义 `RalphLoopConfig` biz 模型 + `ToRunnerConfig()` 转换 | 无 | S |
| RL-02 | `internal/biz/agent.go`：`AgentRuntimeSettings` 新增 `RalphLoop` 字段 | RL-01 | S |
| RL-03 | `internal/agent/ralph_loop.go`：`BuildRalphLoopOption` 桥接函数 | RL-01 | S |
| RL-04 | `api/admin/v1/agent.proto`：新增 RalphLoop 配置消息类型和字段 | RL-01 | M |
| RL-05 | `make api` 重新生成 proto 代码 | RL-04 | S |
| RL-06 | `internal/service/chat.go`：Runner 装配时应用 `WithRalphLoop` | RL-03 | M |
| RL-07 | `internal/data/ent/schema/`：Agent 表扩展 ralph_loop JSON 字段 | RL-02 | M |
| RL-08 | `go generate` 重新生成 Ent 代码 | RL-07 | S |
| RL-09 | Service 层 proto↔biz 映射函数 | RL-05, RL-02 | M |
| RL-10 | 前端配置项与后端字段对齐验证 | RL-09 | S |
| RL-11 | 单元测试：biz 模型转换、桥接函数 | RL-03 | S |
| RL-12 | 集成测试：Runner 装配 + RalphLoop 迭代 | RL-06 | M |
| RL-13 | `make wire` 更新 Wire 注入 | RL-06 | S |

### 3.2 开发顺序

```
RL-01 → RL-02 → RL-03 → RL-04 → RL-05 → RL-07 → RL-08
                                              ↓
                                    RL-06 → RL-09 → RL-10
                                              ↓
                                    RL-11 → RL-12 → RL-13
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| biz 模型转换正确性 | `go test ./internal/biz/... -run TestRalphLoop -count=1` |
| 桥接函数输出 | `go test ./internal/agent/... -run TestBuildRalphLoop -count=1` |
| Runner 装配集成 | `go test ./internal/service/... -run TestRalphLoop -count=1` |
| Proto 生成 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 全量验证 | `make api && make wire && make build && make test` |
